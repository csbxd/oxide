//! Bind an already linked serde_json implementation through real compiler instances.
use rustc_hir::attrs::lang_items::LangItem;
use rustc_hir::def::{DefKind, Res};
use rustc_hir::def_id::{CrateNum, DefId};
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
    pub results: Vec<Ty>,
}

#[derive(Clone, Copy, PartialEq)]
enum Operation {
    Serialize,
    Deserialize,
    Value,
}

fn child(tcx: TyCtxt<'_>, krate: CrateNum, name: &str, kind: DefKind) -> Option<DefId> {
    tcx.module_children(krate.as_def_id())
        .iter()
        .find_map(|child| match child.res {
            Res::Def(actual, def)
                if actual == kind && child.vis.is_public() && child.ident.name.as_str() == name =>
            {
                Some(def)
            }
            _ => None,
        })
}

fn resolve<'tcx>(
    tcx: TyCtxt<'tcx>,
    value: ty::Ty<'tcx>,
    def: DefId,
    error: DefId,
    json_value: DefId,
    operation: Operation,
) -> Result<Option<(Instance, Ty)>> {
    let generics = tcx.generics_of(def);
    if generics.parent_count != 0
        || generics
            .own_params
            .iter()
            .filter(|param| matches!(param.kind, ty::GenericParamDefKind::Type { .. }))
            .count()
            != 1
        || generics
            .own_params
            .iter()
            .any(|param| matches!(param.kind, ty::GenericParamDefKind::Const { .. }))
    {
        return Err(format!(
            "unsupported serde_json generics: {}",
            tcx.def_path_str(def)
        ));
    }
    // to_value consumes its generic T. Instantiate T = &Value so this API
    // borrows the original Rust value just like to_string does.
    let argument = if operation == Operation::Value {
        ty::Ty::new_imm_ref(tcx, tcx.lifetimes.re_erased, value)
    } else {
        value
    };
    let args = ty::GenericArgs::for_item(tcx, def, |param, _| match param.kind {
        ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
        ty::GenericParamDefKind::Type { .. } => argument.into(),
        _ => unreachable!(),
    });
    // Serialize and Deserialize are distinct capabilities. Do not manufacture
    // a binding when the concrete type fails the function's actual predicates.
    if tcx.instantiate_and_check_impossible_clauses((def, args)) {
        return Ok(None);
    }
    let Some(instance) =
        ty::Instance::try_resolve(tcx, TypingEnv::fully_monomorphized(), def, args)
            .map_err(|_| format!("cannot resolve JSON operation for {value}"))?
    else {
        return Ok(None);
    };
    let signature = tcx.normalize_erasing_regions(
        TypingEnv::fully_monomorphized(),
        tcx.fn_sig(def).instantiate(tcx, args),
    );
    let signature = tcx.instantiate_bound_regions_with_erased(signature);
    if !signature.is_fn_trait_compatible() || signature.inputs().len() != 1 {
        return Err(format!(
            "unsupported serde_json signature: {}",
            tcx.def_path_str(def)
        ));
    }
    let ty::Ref(_, pointee, rustc_hir::Mutability::Not) = signature.inputs()[0].kind() else {
        return Err("serde_json operation does not take a shared reference".into());
    };
    if (operation != Operation::Deserialize && tcx.erase_and_anonymize_regions(*pointee) != value)
        || (operation == Operation::Deserialize && !matches!(pointee.kind(), ty::Str))
    {
        return Err("serde_json operation input type mismatch".into());
    }
    let result = signature.output();
    let ty::Adt(adt, result_args) = result.kind() else {
        return Err("serde_json operation does not return Result".into());
    };
    if tcx.get_diagnostic_item(Symbol::intern("Result")) != Some(adt.did())
        || result_args.len() != 2
        || !matches!(result_args.type_at(1).kind(), ty::Adt(adt, _) if adt.did() == error)
    {
        return Err("serde_json Result/error identity mismatch".into());
    }
    let output = result_args.type_at(0);
    match operation {
        Operation::Serialize if !matches!(output.kind(), ty::Adt(adt, _) if tcx.is_lang_item(adt.did(), LangItem::String)) =>
        {
            return Err("serde_json serializer does not return the compiler String type".into());
        }
        Operation::Deserialize if tcx.erase_and_anonymize_regions(output) != value => {
            return Err("serde_json deserializer output type mismatch".into());
        }
        Operation::Value if !matches!(output.kind(), ty::Adt(adt, _) if adt.did() == json_value) => {
            return Err("serde_json to_value output identity mismatch".into());
        }
        _ => (),
    }
    Ok(Some((
        rustc_internal::stable(instance),
        rustc_internal::stable(result),
    )))
}

// Invoked only for concrete types in the public API closure. This neither adds
// a dependency nor enumerates every type appearing in the complete MIR graph.
pub fn plan(tcx: TyCtxt<'_>, value: Ty) -> Result<Option<Plan>> {
    let value = tcx.erase_and_anonymize_regions(rustc_internal::internal(tcx, value));
    let mut plan = Plan {
        metadata: json!({}),
        instances: vec![],
        results: vec![],
    };
    let mut serialize_bound = false;
    let mut deserialize_bound = false;
    let mut value_bound = false;
    for &krate in tcx.crates(()) {
        if tcx.crate_name(krate).as_str() != "serde_json" {
            continue;
        }
        let error = child(tcx, krate, "Error", DefKind::Struct)
            .ok_or("serde_json public Error definition missing")?;
        let json_value = child(tcx, krate, "Value", DefKind::Enum)
            .ok_or("serde_json public Value definition missing")?;
        for (name, key, result_key, operation, bound) in [
            (
                "to_string",
                "serialize_symbol",
                "serialize_result_type",
                Operation::Serialize,
                &mut serialize_bound,
            ),
            (
                "from_str",
                "deserialize_symbol",
                "deserialize_result_type",
                Operation::Deserialize,
                &mut deserialize_bound,
            ),
            (
                "to_value",
                "value_symbol",
                "value_result_type",
                Operation::Value,
                &mut value_bound,
            ),
        ] {
            let def = child(tcx, krate, name, DefKind::Fn)
                .ok_or_else(|| format!("serde_json public function {name} missing"))?;
            if let Some((instance, result)) =
                resolve(tcx, value, def, error, json_value, operation)?
            {
                if *bound {
                    return Err(format!(
                        "ambiguous serde_json {name} implementations for {value}"
                    ));
                }
                *bound = true;
                plan.instances.push((key, instance));
                plan.results.push(result);
                plan.metadata[result_key] = json!(result);
            }
        }
    }
    Ok((!plan.instances.is_empty()).then_some(plan))
}
