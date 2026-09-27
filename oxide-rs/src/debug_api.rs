//! Bind Rust's own Debug formatter without synthesizing a project-specific shim.
use rustc_hir::def::{DefKind, Res};
use rustc_hir::def_id::DefId;
use rustc_middle::ty::{self, TyCtxt, TypingEnv};
use rustc_public::mir::mono::Instance;
use rustc_public::rustc_internal;
use rustc_public::ty::Ty;
use rustc_span::Symbol;
use serde_json::{Value, json};

type Result<T> = std::result::Result<T, String>;

pub struct Plan {
    pub metadata: Value,
    pub instances: Vec<(&'static str, Instance)>,
    pub layouts: Vec<Ty>,
    pub result: Ty,
    pub value: Ty,
}

fn method(tcx: TyCtxt<'_>, owner: DefId, name: &str) -> Option<DefId> {
    tcx.inherent_impls(owner).iter().find_map(|implementation| {
        tcx.associated_items(*implementation)
            .in_definition_order()
            .find(|item| item.is_fn() && item.name().as_str() == name)
            .map(|item| item.def_id)
    })
}

fn child(tcx: TyCtxt<'_>, module: DefId, name: &str, kind: DefKind) -> Option<DefId> {
    let children = if let Some(local) = module.as_local() {
        tcx.module_children_local(local)
    } else {
        tcx.module_children(module)
    };
    children.iter().find_map(|child| match child.res {
        Res::Def(actual, def) if actual == kind && child.ident.name.as_str() == name => Some(def),
        _ => None,
    })
}

fn result_type<'tcx>(
    tcx: TyCtxt<'tcx>,
    def: DefId,
    args: ty::GenericArgsRef<'tcx>,
) -> ty::Ty<'tcx> {
    let signature = tcx.fn_sig(def).instantiate(tcx, args).skip_norm_wip();
    tcx.instantiate_bound_regions_with_erased(signature)
        .output()
}

pub fn plan(tcx: TyCtxt<'_>, value: Ty) -> Result<Option<Plan>> {
    let (Some(argument), Some(string), Some(arguments_impl)) = (
        tcx.lang_items().format_argument(),
        tcx.lang_items().string(),
        tcx.get_diagnostic_item(Symbol::intern("FmtArgumentsNew")),
    ) else {
        return Ok(None);
    };
    let new_debug =
        method(tcx, argument, "new_debug").ok_or("compiler Debug argument constructor missing")?;
    let internal = rustc_internal::internal(tcx, value);
    let sized = tcx
        .layout_of(TypingEnv::fully_monomorphized().as_query_input(internal))
        .map_err(|error| format!("Debug input layout {internal}: {error:?}"))?
        .is_sized();
    let debug_value = if sized {
        internal
    } else {
        ty::Ty::new_imm_ref(tcx, tcx.lifetimes.re_erased, internal)
    };
    let debug_args = ty::GenericArgs::for_item(tcx, new_debug, |param, _| match param.kind {
        ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
        ty::GenericParamDefKind::Type { .. } => debug_value.into(),
        _ => unreachable!("unexpected generics on fmt::Argument::new_debug"),
    });
    if tcx.instantiate_and_check_impossible_clauses((new_debug, debug_args)) {
        return Ok(None);
    }
    let debug =
        ty::Instance::try_resolve(tcx, TypingEnv::fully_monomorphized(), new_debug, debug_args)
            .map_err(|_| format!("cannot resolve Debug argument for {internal}"))?
            .ok_or_else(|| format!("missing Debug argument instance for {internal}"))?;
    let arguments_new = tcx
        .associated_items(arguments_impl)
        .in_definition_order()
        .find(|item| item.is_fn() && item.name().as_str() == "new")
        .ok_or("compiler fmt::Arguments::new missing")?
        .def_id;
    let new_args = ty::GenericArgs::for_item(tcx, arguments_new, |param, _| match param.kind {
        ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
        ty::GenericParamDefKind::Const { .. } => ty::Const::from_target_usize(
            tcx,
            match param.name.as_str() {
                "N" => 2,
                "M" => 1,
                _ => unreachable!("unexpected format template parameter"),
            },
        )
        .into(),
        _ => unreachable!("unexpected generics on fmt::Arguments::new"),
    });
    let arguments = ty::Instance::new_raw(arguments_new, new_args);
    let alloc_fmt = child(tcx, string.krate.as_def_id(), "fmt", DefKind::Mod)
        .ok_or("compiler alloc::fmt module missing")?;
    let format = child(tcx, alloc_fmt, "format", DefKind::Fn)
        .ok_or("compiler alloc::fmt::format missing")?;
    let format = ty::Instance::mono(tcx, format);
    let argument_ty = result_type(tcx, new_debug, debug_args);
    let arguments_ty = result_type(tcx, arguments_new, new_args);
    let string_ty = result_type(tcx, format.def_id(), format.args);
    let argument_array = ty::Ty::new_array(tcx, argument_ty, 1);
    let template = ty::Ty::new_array(tcx, tcx.types.u8, 2);
    let as_public = |value| -> Ty { rustc_internal::stable(value) };
    // This is the pinned compiler's documented format_args encoding: one
    // default placeholder followed by End. Debug lives in the argument's real
    // Rust formatter function; no formatting rules are reimplemented in Go.
    let metadata = json!({"value_type":as_public(debug_value),"by_reference":!sized,
        "argument_type":as_public(argument_ty),"argument_array":as_public(argument_array),
        "template_type":as_public(template),"arguments_type":as_public(arguments_ty),
        "string_type":as_public(string_ty),"template":[192,0]});
    Ok(Some(Plan {
        metadata,
        instances: vec![
            ("argument_symbol", rustc_internal::stable(debug)),
            ("arguments_symbol", rustc_internal::stable(arguments)),
            ("format_symbol", rustc_internal::stable(format)),
        ],
        layouts: vec![
            as_public(argument_ty),
            as_public(argument_array),
            as_public(template),
            as_public(arguments_ty),
        ],
        result: as_public(string_ty),
        value: as_public(debug_value),
    }))
}
