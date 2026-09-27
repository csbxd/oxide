//! Bind container iteration to the concrete Rust iterator implementation.
use rustc_middle::ty::{self, TyCtxt, TypingEnv};
use rustc_public::mir::mono::Instance;
use rustc_public::rustc_internal;
use rustc_public::ty::Ty;
use serde_json::{Value, json};

type Result<T> = std::result::Result<T, String>;

pub struct Plan {
    pub metadata: Value,
    pub instances: Vec<(&'static str, Instance)>,
    pub children: Vec<Ty>,
}

fn result_type<'tcx>(
    tcx: TyCtxt<'tcx>,
    def: rustc_hir::def_id::DefId,
    args: ty::GenericArgsRef<'tcx>,
) -> ty::Ty<'tcx> {
    let signature = tcx.normalize_erasing_regions(
        TypingEnv::fully_monomorphized(),
        tcx.fn_sig(def).instantiate(tcx, args),
    );
    tcx.instantiate_bound_regions_with_erased(signature)
        .output()
}

pub fn plan(tcx: TyCtxt<'_>, value: Ty, method_name: &str) -> Result<Plan> {
    let value = rustc_internal::internal(tcx, value);
    let ty::Adt(adt, actual) = value.kind() else {
        return Err("map iteration needs an ADT".into());
    };
    let mut candidates = Vec::new();
    for &implementation in tcx.inherent_impls(adt.did()) {
        let pattern = tcx
            .type_of(implementation)
            .instantiate_identity()
            .skip_norm_wip();
        let ty::Adt(pattern_def, pattern_args) = pattern.kind() else {
            continue;
        };
        if pattern_def != adt || pattern_args.len() != actual.len() {
            continue;
        }
        let generics = tcx.generics_of(implementation);
        let mut inferred = vec![None; generics.count()];
        let mut matches = true;
        for (pattern, argument) in pattern_args.iter().zip(actual.iter()) {
            match pattern.kind() {
                ty::GenericArgKind::Type(ty) if matches!(ty.kind(), ty::Param(_)) => {
                    let ty::Param(param) = ty.kind() else {
                        unreachable!()
                    };
                    let slot = &mut inferred[param.index as usize];
                    if slot.is_some_and(|old| old != argument) {
                        matches = false;
                        break;
                    }
                    *slot = Some(argument);
                }
                ty::GenericArgKind::Lifetime(_) => (),
                _ if tcx.erase_and_anonymize_regions(pattern)
                    == tcx.erase_and_anonymize_regions(argument) =>
                {
                    ()
                }
                _ => {
                    matches = false;
                    break;
                }
            }
        }
        if !matches
            || generics.own_params.iter().any(|param| {
                !matches!(param.kind, ty::GenericParamDefKind::Lifetime)
                    && inferred[param.index as usize].is_none()
            })
        {
            continue;
        }
        for method in tcx.associated_items(implementation).in_definition_order() {
            if !method.is_fn()
                || method.name().as_str() != method_name
                || tcx
                    .generics_of(method.def_id)
                    .own_requires_monomorphization()
            {
                continue;
            }
            let args = ty::GenericArgs::for_item(tcx, method.def_id, |param, _| match param.kind {
                ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
                _ => inferred[param.index as usize].expect("inferred map impl parameter"),
            });
            let receiver = tcx.normalize_erasing_regions(
                TypingEnv::fully_monomorphized(),
                tcx.type_of(implementation).instantiate(tcx, args),
            );
            if receiver != tcx.erase_and_anonymize_regions(value) {
                continue;
            }
            if tcx.instantiate_and_check_impossible_clauses((method.def_id, args)) {
                continue;
            }
            if let Some(instance) = ty::Instance::try_resolve(
                tcx,
                TypingEnv::fully_monomorphized(),
                method.def_id,
                args,
            )
            .map_err(|_| format!("cannot resolve {value}::{method_name}"))?
            {
                candidates.push((instance, result_type(tcx, method.def_id, args)));
            }
        }
    }
    if candidates.len() != 1 {
        return Err(format!(
            "expected one concrete {value}::{method_name}, got {}",
            candidates.len()
        ));
    }
    let (iterator, iterator_ty) = candidates[0];
    let next = tcx
        .lang_items()
        .next_fn()
        .ok_or("Iterator::next lang item missing")?;
    let next_args = ty::GenericArgs::for_item(tcx, next, |param, _| match param.kind {
        ty::GenericParamDefKind::Lifetime => tcx.lifetimes.re_erased.into(),
        ty::GenericParamDefKind::Type { .. } if param.index == 0 => iterator_ty.into(),
        _ => unreachable!("unexpected Iterator::next generics"),
    });
    if tcx.instantiate_and_check_impossible_clauses((next, next_args)) {
        return Err(format!("{iterator_ty} does not implement Iterator"));
    }
    let next = ty::Instance::try_resolve(tcx, TypingEnv::fully_monomorphized(), next, next_args)
        .map_err(|_| format!("cannot resolve {iterator_ty}::next"))?
        .ok_or("missing Iterator::next instance")?;
    let item_ty = result_type(tcx, next.def_id(), next.args);
    let iterator_ty: Ty = rustc_internal::stable(iterator_ty);
    let item_ty: Ty = rustc_internal::stable(item_ty);
    Ok(Plan {
        metadata: json!({"iterator_type":iterator_ty,"item_type":item_ty}),
        instances: vec![
            ("symbol", rustc_internal::stable(iterator)),
            ("next_symbol", rustc_internal::stable(next)),
        ],
        children: vec![iterator_ty, item_ty],
    })
}
