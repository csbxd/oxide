//! Compiler-owned metadata for operating on Rust values at the public boundary.
use rustc_hir::attrs::lang_items::LangItem;
use rustc_hir::def_id::DefId;
use rustc_middle::ty::print::{with_no_trimmed_paths, with_no_visible_paths};
use rustc_middle::ty::{self, TyCtxt, TypingEnv};
use rustc_public::mir::mono::Instance;
use rustc_public::rustc_internal;
use rustc_public::ty::Ty;
use rustc_span::Symbol;
use serde_json::{Value, json};

type Result<T> = std::result::Result<T, String>;

pub struct Description {
    pub metadata: Value,
    pub children: Vec<Ty>,
    pub operations: Vec<(&'static str, Instance, Ty)>,
    pub debug: Option<crate::debug_api::Plan>,
    pub iterations: Vec<(&'static str, crate::iteration_api::Plan)>,
    pub json: Option<crate::json_api::Plan>,
}

pub fn canonical(tcx: TyCtxt<'_>, value: Ty) -> Ty {
    let internal = rustc_internal::internal(tcx, value);
    rustc_internal::stable(tcx.erase_and_anonymize_regions(internal))
}

fn name(tcx: TyCtxt<'_>, value: Ty) -> String {
    with_no_visible_paths!(with_no_trimmed_paths!(
        rustc_internal::internal(tcx, value).to_string()
    ))
}

fn definition(tcx: TyCtxt<'_>, def: DefId) -> String {
    with_no_visible_paths!(with_no_trimmed_paths!(tcx.def_path_str(def)))
}

fn diagnostic(tcx: TyCtxt<'_>, def: DefId, item: &str) -> bool {
    tcx.get_diagnostic_item(Symbol::intern(item)) == Some(def)
}

fn layout<'tcx>(
    tcx: TyCtxt<'tcx>,
    value: ty::Ty<'tcx>,
) -> Result<rustc_middle::ty::layout::TyAndLayout<'tcx>> {
    tcx.layout_of(TypingEnv::fully_monomorphized().as_query_input(value))
        .map_err(|error| format!("public type layout {value}: {error:?}"))
}

fn field<'tcx>(
    tcx: TyCtxt<'tcx>,
    value: ty::Ty<'tcx>,
    field_name: &str,
) -> Result<(ty::Ty<'tcx>, u64)> {
    let ty::Adt(adt, args) = value.kind() else {
        return Err(format!("expected ADT at {value}.{field_name}"));
    };
    if !adt.is_struct() {
        return Err(format!("expected struct at {value}.{field_name}"));
    }
    let (index, field) = adt
        .non_enum_variant()
        .fields
        .iter()
        .enumerate()
        .find(|(_, field)| field.name.as_str() == field_name)
        .ok_or_else(|| format!("compiler container field missing: {value}.{field_name}"))?;
    let field_ty =
        tcx.normalize_erasing_regions(TypingEnv::fully_monomorphized(), field.ty(tcx, args));
    Ok((field_ty, layout(tcx, value)?.fields.offset(index).bytes()))
}

fn field_path<'tcx>(
    tcx: TyCtxt<'tcx>,
    mut value: ty::Ty<'tcx>,
    path: &[&str],
) -> Result<(ty::Ty<'tcx>, u64)> {
    let mut offset = 0u64;
    for field_name in path {
        let (child, displacement) = field(tcx, value, field_name)?;
        offset = offset
            .checked_add(displacement)
            .ok_or("container field offset overflow")?;
        value = child;
    }
    Ok((value, offset))
}

fn scalar_pointer<'tcx>(tcx: TyCtxt<'tcx>, value: ty::Ty<'tcx>) -> Result<()> {
    let l = layout(tcx, value)?;
    if !matches!(l.backend_repr, rustc_abi::BackendRepr::Scalar(scalar) if matches!(scalar.primitive(), rustc_abi::Primitive::Pointer(_)))
        || l.size != tcx.data_layout.pointer_size()
    {
        return Err(format!("expected compiler pointer scalar for {value}"));
    }
    Ok(())
}

fn scalar_usize<'tcx>(tcx: TyCtxt<'tcx>, value: ty::Ty<'tcx>) -> Result<()> {
    let l = layout(tcx, value)?;
    if !matches!(l.backend_repr, rustc_abi::BackendRepr::Scalar(scalar) if matches!(scalar.primitive(), rustc_abi::Primitive::Int(_, false)))
        || l.size != tcx.data_layout.pointer_size()
    {
        return Err(format!("expected compiler usize scalar for {value}"));
    }
    Ok(())
}

fn is_global(tcx: TyCtxt<'_>, value: ty::Ty<'_>) -> bool {
    matches!(value.kind(), ty::Adt(adt, _) if tcx.is_lang_item(adt.did(), LangItem::GlobalAlloc))
}

fn vec_container<'tcx>(
    tcx: TyCtxt<'tcx>,
    value: ty::Ty<'tcx>,
    kind: &str,
    prefix: &[&str],
) -> Result<(Value, Vec<Ty>)> {
    let (vector, base) = field_path(tcx, value, prefix)?;
    let ty::Adt(def, args) = vector.kind() else {
        return Err("String backing field is not Vec".into());
    };
    if !diagnostic(tcx, def.did(), "Vec") {
        return Err(format!("compiler Vec identity mismatch for {value}"));
    }
    let element = args.type_at(0);
    let allocator = args.type_at(1);
    let (data, data_offset) = field_path(tcx, vector, &["buf", "inner", "ptr"])?;
    let (length, len_offset) = field_path(tcx, vector, &["len"])?;
    let (capacity, capacity_offset) = field_path(tcx, vector, &["buf", "inner", "cap"])?;
    scalar_pointer(tcx, data)?;
    scalar_usize(tcx, length)?;
    scalar_usize(tcx, capacity)?;
    let element_layout = layout(tcx, element)?;
    let element: Ty = rustc_internal::stable(element);
    let allocator: Ty = rustc_internal::stable(allocator);
    Ok((
        json!({"kind":kind,"element":element,"allocator":allocator,
        "global_allocator":is_global(tcx, rustc_internal::internal(tcx, allocator)),
        "data_offset":base+data_offset,"len_offset":base+len_offset,"capacity_offset":base+capacity_offset,
        "zst":element_layout.size.bytes()==0,"dangling_align":element_layout.align.abi.bytes()}),
        vec![element, allocator],
    ))
}

// These are semantic identities, not matches against a user-provided type name.
fn byte_dst<'tcx>(tcx: TyCtxt<'tcx>, value: ty::Ty<'tcx>) -> Result<Option<&'static str>> {
    match value.kind() {
        ty::Str => Ok(Some("str")),
        ty::Slice(_) => Ok(Some("slice")),
        ty::Adt(def, _)
            if diagnostic(tcx, def.did(), "OsStr") || diagnostic(tcx, def.did(), "Path") =>
        {
            let path = diagnostic(tcx, def.did(), "Path");
            let fields: &[&str] = if path {
                &["inner", "inner", "inner"]
            } else {
                &["inner", "inner"]
            };
            let (bytes, offset) = field_path(tcx, value, fields)?;
            if offset != 0
                || !matches!(bytes.kind(), ty::Slice(element) if *element == tcx.types.u8)
            {
                return Err(format!("Unix byte DST layout mismatch for {value}"));
            }
            Ok(Some(if path { "path" } else { "os_str" }))
        }
        _ => Ok(None),
    }
}

fn fat_pointer<'tcx>(tcx: TyCtxt<'tcx>, value: ty::Ty<'tcx>, vtable: bool) -> Result<u64> {
    let l = layout(tcx, value)?;
    let rustc_abi::BackendRepr::ScalarPair { a, b, b_offset } = l.backend_repr else {
        return Err(format!("expected compiler fat pointer layout for {value}"));
    };
    if !matches!(a.primitive(), rustc_abi::Primitive::Pointer(_))
        || (if vtable {
            !matches!(b.primitive(), rustc_abi::Primitive::Pointer(_))
        } else {
            !matches!(b.primitive(), rustc_abi::Primitive::Int(_, false))
        })
        || b.primitive().size(&tcx.data_layout) != tcx.data_layout.pointer_size()
        || l.size.bytes() != tcx.data_layout.pointer_size().bytes() * 2
    {
        return Err(format!("invalid compiler slice pointer layout for {value}"));
    }
    Ok(b_offset.bytes())
}

fn resolve_method(
    tcx: TyCtxt<'_>,
    value: Ty,
    diagnostic_name: &str,
) -> Result<Option<(Instance, Ty)>> {
    let Some(def) = tcx.get_diagnostic_item(Symbol::intern(diagnostic_name)) else {
        return Ok(None);
    };
    let internal = rustc_internal::internal(tcx, canonical(tcx, value));
    let args = ty::GenericArgs::for_item(tcx, def, |param, _| match param.kind {
        ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
        ty::GenericParamDefKind::Type { .. } if param.index == 0 => internal.into(),
        _ => unreachable!("unexpected generics on compiler diagnostic method {diagnostic_name}"),
    });
    if tcx.instantiate_and_check_impossible_clauses((def, args)) {
        return Ok(None);
    }
    let Some(instance) =
        ty::Instance::try_resolve(tcx, TypingEnv::fully_monomorphized(), def, args)
            .map_err(|_| format!("cannot resolve {diagnostic_name} for {internal}"))?
    else {
        return Ok(None);
    };
    let signature = tcx.fn_sig(def).instantiate(tcx, args).skip_norm_wip();
    let result = tcx
        .instantiate_bound_regions_with_erased(signature)
        .output();
    Ok(Some((
        rustc_internal::stable(instance),
        rustc_internal::stable(result),
    )))
}

pub fn describe(tcx: TyCtxt<'_>, value: Ty) -> Result<Description> {
    let canonical = canonical(tcx, value);
    let internal = rustc_internal::internal(tcx, canonical);
    let mut metadata = json!({"name":name(tcx, canonical),"canonical":canonical,
        "needs_drop":internal.needs_drop(tcx, TypingEnv::fully_monomorphized())});
    let mut children = Vec::new();
    let mut iterations = Vec::new();
    if canonical != value {
        children.push(canonical);
    }
    if let Ok(l) = layout(tcx, internal) {
        metadata["inhabited"] = json!(!l.is_uninhabited());
    }
    match internal.kind() {
        ty::Ref(_, pointee, mutable) | ty::RawPtr(pointee, mutable) => {
            metadata["pointer_kind"] = json!(if matches!(internal.kind(), ty::Ref(..)) {
                "ref"
            } else {
                "raw"
            });
            metadata["mutable"] = json!(*mutable == rustc_hir::Mutability::Mut);
            children.push(rustc_internal::stable(*pointee));
            if let Some(kind) = byte_dst(tcx, *pointee)? {
                let element = match pointee.kind() {
                    ty::Slice(element) => *element,
                    _ => tcx.types.u8,
                };
                let element: Ty = rustc_internal::stable(element);
                children.push(element);
                metadata["container"] = json!({"kind":format!("{kind}_ref"),"element":element,
                    "data_offset":0,"len_offset":fat_pointer(tcx, internal, false)?});
            } else if matches!(
                layout(tcx, internal)?.backend_repr,
                rustc_abi::BackendRepr::ScalarPair { .. }
            ) {
                let tail = tcx.struct_tail_for_codegen(*pointee, TypingEnv::fully_monomorphized());
                let metadata_kind = match tail.kind() {
                    ty::Dynamic(..) => "vtable",
                    ty::Slice(_) | ty::Str => "length",
                    _ => return Err(format!("unsupported reference metadata tail {tail}")),
                };
                let offset = fat_pointer(tcx, internal, metadata_kind == "vtable")?;
                metadata["container"] = json!({"kind":"reference","element":rustc_internal::stable(*pointee),
                    "data_offset":0,"meta_offset":offset,"len_offset":offset,"metadata":metadata_kind});
            }
        }
        ty::Array(element, _) | ty::Slice(element) => {
            children.push(rustc_internal::stable(*element))
        }
        ty::Str => children.push(rustc_internal::stable(tcx.types.u8)),
        ty::Tuple(fields) => {
            let l = layout(tcx, internal)?;
            let mut members = Vec::new();
            for (index, field) in fields.iter().enumerate() {
                let field: Ty = rustc_internal::stable(field);
                children.push(field);
                members.push(json!({"name":index.to_string(),"type":field,"public":true,"offset":l.fields.offset(index).bytes()}));
            }
            metadata["members"] = json!([members]);
        }
        ty::Adt(def, args) => {
            metadata["definition"] = json!(definition(tcx, def.did()));
            metadata["non_exhaustive"] = json!(def.is_variant_list_non_exhaustive());
            let l = layout(tcx, internal)?;
            let mut variants = Vec::new();
            let mut members = Vec::new();
            for (variant_index, variant) in def.variants().iter_enumerated() {
                let inhabited = !l.is_variant_uninhabited(variant_index);
                variants.push(
                    json!({"name":variant.name.to_ident_string(),"inhabited":inhabited,
                    "non_exhaustive":variant.is_field_list_non_exhaustive()}),
                );
                let mut fields = Vec::new();
                for (index, field) in variant.fields.iter().enumerate() {
                    let field_ty = tcx.normalize_erasing_regions(
                        TypingEnv::fully_monomorphized(),
                        field.ty(tcx, args),
                    );
                    let field_ty: Ty = rustc_internal::stable(field_ty);
                    let public = field.vis.is_public();
                    // A DST's private tail still determines its dynamic size and
                    // alignment. Retain that descriptor without making the field
                    // accessible; ordinary private implementation fields stay out.
                    if public || (!l.is_sized() && index + 1 == variant.fields.len()) {
                        children.push(field_ty);
                    }
                    let offset = if inhabited {
                        Some(match &l.variants {
                            rustc_abi::Variants::Multiple { variants, .. } => variants
                                [variant_index]
                                .field_offsets
                                .iter()
                                .nth(index)
                                .unwrap()
                                .bytes(),
                            _ => l.fields.offset(index).bytes(),
                        })
                    } else {
                        None
                    };
                    fields.push(json!({"name":field.name.to_ident_string(),"type":field_ty,"public":public,"offset":offset}));
                }
                members.push(fields);
            }
            metadata["members"] = json!(members);
            metadata["variants_info"] = json!(variants);
            let container = if tcx.is_lang_item(def.did(), LangItem::String) {
                Some(vec_container(tcx, internal, "string", &["vec"])?)
            } else if diagnostic(tcx, def.did(), "Vec") {
                Some(vec_container(tcx, internal, "vec", &[])?)
            } else if diagnostic(tcx, def.did(), "OsString") {
                Some(vec_container(
                    tcx,
                    internal,
                    "os_string",
                    &["inner", "inner"],
                )?)
            } else if diagnostic(tcx, def.did(), "PathBuf") {
                Some(vec_container(
                    tcx,
                    internal,
                    "path_buf",
                    &["inner", "inner", "inner"],
                )?)
            } else {
                None
            };
            if let Some((container, nested)) = container {
                metadata["container"] = container;
                children.extend(nested);
            } else if diagnostic(tcx, def.did(), "HashMap")
                || diagnostic(tcx, def.did(), "BTreeMap")
            {
                let key: Ty = rustc_internal::stable(args.type_at(0));
                let value: Ty = rustc_internal::stable(args.type_at(1));
                metadata["container"] = json!({"kind":if diagnostic(tcx,def.did(),"HashMap") {"hash_map"} else {"btree_map"},"key":key,"value":value});
                children.extend([key, value]);
                for (field, method) in [("iteration", "iter"), ("mutable_iteration", "iter_mut")] {
                    let plan = crate::iteration_api::plan(tcx, canonical, method)?;
                    children.extend(plan.children.iter().copied());
                    iterations.push((field, plan));
                }
            } else if diagnostic(tcx, def.did(), "Result") || diagnostic(tcx, def.did(), "Option") {
                metadata["container"] = json!({"kind":if diagnostic(tcx, def.did(), "Result") {"result"} else {"option"}});
            } else if let Some(kind) = byte_dst(tcx, internal)? {
                metadata["container"] =
                    json!({"kind":kind,"element":rustc_internal::stable(tcx.types.u8)});
                children.push(rustc_internal::stable(tcx.types.u8));
            } else if tcx.is_lang_item(def.did(), LangItem::OwnedBox) {
                let element: Ty = rustc_internal::stable(args.type_at(0));
                let allocator: Ty = rustc_internal::stable(args.type_at(1));
                let (pointer, offset) = field(tcx, internal, "0")?;
                let pointer_layout = layout(tcx, pointer)?;
                let mut container = json!({"kind":"box","element":element,"allocator":allocator,
                    "global_allocator":is_global(tcx, args.type_at(1)),"data_offset":offset});
                match pointer_layout.backend_repr {
                    rustc_abi::BackendRepr::Scalar(_) => scalar_pointer(tcx, pointer)?,
                    rustc_abi::BackendRepr::ScalarPair { a, b, b_offset }
                        if matches!(a.primitive(), rustc_abi::Primitive::Pointer(_)) =>
                    {
                        container["meta_offset"] = json!(offset + b_offset.bytes());
                        container["metadata"] = json!(if matches!(
                            b.primitive(),
                            rustc_abi::Primitive::Pointer(_)
                        ) {
                            "vtable"
                        } else {
                            "length"
                        });
                    }
                    _ => {
                        return Err(format!(
                            "invalid compiler Box pointer representation for {internal}"
                        ));
                    }
                }
                metadata["container"] = container;
                children.extend([element, allocator]);
            }
        }
        ty::FnPtr(..) => {
            let signature = tcx.instantiate_bound_regions_with_erased(internal.fn_sig(tcx));
            children.extend(
                signature
                    .inputs()
                    .iter()
                    .map(|ty| rustc_internal::stable(*ty)),
            );
            children.push(rustc_internal::stable(signature.output()));
        }
        _ => (),
    }
    let mut operations = Vec::new();
    for (field, method) in [
        ("display_symbol", "to_string_method"),
        ("default_symbol", "default_fn"),
    ] {
        if let Some((instance, result)) = resolve_method(tcx, canonical, method)? {
            operations.push((field, instance, result));
            children.push(result);
        }
    }
    let debug = crate::debug_api::plan(tcx, canonical)?;
    if let Some(debug) = &debug {
        children.extend([debug.result, debug.value]);
    }
    let json = crate::json_api::plan(tcx, canonical)?;
    if let Some(json) = &json {
        children.extend(json.results.iter().copied());
    }
    Ok(Description {
        metadata,
        children,
        operations,
        debug,
        iterations,
        json,
    })
}
