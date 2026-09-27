//! Discover callable APIs through the compiler's public namespace, not item visibility alone.
use rustc_hir::def::{CtorKind, DefKind, Res};
use rustc_hir::def_id::{CRATE_DEF_ID, CrateNum, DefId, LOCAL_CRATE};
use rustc_middle::ty::print::{PrintTraitRefExt, with_no_trimmed_paths, with_no_visible_paths};
use rustc_middle::ty::{self, TyCtxt, TypeVisitableExt};
use rustc_public::mir::mono::Instance;
use rustc_public::ty::Ty;
use rustc_public::{CrateDef, ItemKind, all_local_items, rustc_internal};
use serde_json::{Value, json};
use std::collections::{BTreeMap, HashMap, HashSet, VecDeque};

pub struct Roots {
    pub selected: Vec<(String, Instance)>,
    pub public_api: Vec<Value>,
    pub public_types: Vec<(String, Ty)>,
}

fn requested_roots(filter: &str) -> Result<HashSet<&str>, String> {
    let mut names = HashSet::new();
    let mut depth = 0usize;
    let mut start = 0;
    for (offset, ch) in filter.char_indices() {
        match ch {
            '<' | '(' | '[' | '{' => depth += 1,
            '>' | ')' | ']' | '}' => {
                depth = depth
                    .checked_sub(1)
                    .ok_or_else(|| format!("unbalanced root filter {filter:?}"))?
            }
            ',' if depth == 0 => {
                names.insert(filter[start..offset].trim());
                start = offset + 1;
            }
            _ => (),
        }
    }
    names.insert(filter[start..].trim());
    if depth != 0 || names.contains("") {
        return Err(format!("invalid root filter {filter:?}"));
    }
    Ok(names)
}

struct Discovery<'tcx> {
    tcx: TyCtxt<'tcx>,
    candidates: BTreeMap<String, Instance>,
    inventory: Vec<Value>,
    types: Vec<(String, DefId, ty::Ty<'tcx>, bool)>,
    public_types: Vec<(String, Ty)>,
    traits: HashMap<CrateNum, HashMap<DefId, String>>,
}

fn sized_signature<'tcx>(tcx: TyCtxt<'tcx>, instance: ty::Instance<'tcx>) -> bool {
    let signature = tcx
        .fn_sig(instance.def_id())
        .instantiate(tcx, instance.args)
        .skip_norm_wip();
    let signature = tcx.instantiate_bound_regions_with_erased(signature);
    signature
        .inputs_and_output
        .iter()
        .all(|ty| ty.is_sized(tcx, ty::TypingEnv::fully_monomorphized()))
}

pub fn discover(tcx: TyCtxt<'_>, filter: &str) -> Result<Roots, String> {
    let mut discovery = Discovery {
        tcx,
        candidates: BTreeMap::new(),
        inventory: vec![],
        types: vec![],
        public_types: vec![],
        traits: HashMap::new(),
    };
    discovery.module(
        CRATE_DEF_ID.to_def_id(),
        &tcx.crate_name(LOCAL_CRATE).to_string(),
        &mut HashSet::new(),
    )?;
    discovery.trait_impls()?;
    if filter.is_empty() {
        if let Some(entry) = rustc_public::entry_fn() {
            discovery.add_root(
                entry.name(),
                Instance::try_from(entry).map_err(|e| e.to_string())?,
            )?;
        }
    } else {
        // Explicit selection may also name a private local function. This is useful for
        // compiler probes; it does not make the function part of the public API inventory.
        let requested = requested_roots(filter)?;
        for item in all_local_items() {
            if item.kind() == ItemKind::Fn
                && requested.contains(item.name().as_str())
                && !item.requires_monomorphization()
            {
                let def = rustc_internal::internal(tcx, item.def_id());
                let instance = ty::Instance::mono(tcx, def);
                if rustc_middle::mono::MonoItem::Fn(instance).is_instantiable(tcx)
                    && sized_signature(tcx, instance)
                {
                    discovery.add_root(item.name(), rustc_internal::stable(instance))?;
                }
            }
        }
        for name in &requested {
            if !discovery.candidates.contains_key(*name) {
                let reason = discovery
                    .inventory
                    .iter()
                    .find(|api| api["name"] == *name)
                    .and_then(|api| api["status"].as_str())
                    .unwrap_or("unknown or non-public API path");
                return Err(format!("cannot select root {name:?}: {reason}"));
            }
        }
        discovery
            .candidates
            .retain(|name, _| requested.contains(name.as_str()));
    }
    for api in &mut discovery.inventory {
        api["selected"] = json!(
            api["name"]
                .as_str()
                .is_some_and(|name| discovery.candidates.contains_key(name))
        );
    }
    discovery.inventory.sort_by(|a, b| {
        (a["name"].as_str(), a["definition"].as_str())
            .cmp(&(b["name"].as_str(), b["definition"].as_str()))
    });
    Ok(Roots {
        selected: discovery.candidates.into_iter().collect(),
        public_api: discovery.inventory,
        public_types: discovery.public_types,
    })
}

impl<'tcx> Discovery<'tcx> {
    fn add_root(&mut self, name: String, instance: Instance) -> Result<(), String> {
        if let Some(previous) = self.candidates.insert(name.clone(), instance)
            && previous != instance
        {
            return Err(format!(
                "ambiguous public root {name:?}: {} and {}",
                previous.def.name(),
                instance.def.name()
            ));
        }
        Ok(())
    }

    fn function(
        &mut self,
        name: String,
        def: DefId,
        kind: &str,
        generic_owner: bool,
    ) -> Result<(), String> {
        let generic = generic_owner
            || self
                .tcx
                .generics_of(def)
                .requires_monomorphization(self.tcx);
        let instance = (!generic).then(|| ty::Instance::mono(self.tcx, def));
        let impossible = instance.is_some_and(|instance| {
            !rustc_middle::mono::MonoItem::Fn(instance).is_instantiable(self.tcx)
        });
        let unsized_value = instance.is_some_and(|instance| !sized_signature(self.tcx, instance));
        self.inventory.push(json!({
            "name": name, "definition": self.tcx.def_path_str(def), "kind": kind,
            "status": if generic { "requires_monomorphization" } else if impossible { "unsatisfied_predicates" } else if unsized_value { "unsupported_unsized_value" } else { "monomorphic" },
        }));
        if let Some(instance) = instance
            && !impossible
            && !unsized_value
        {
            self.add_root(name, rustc_internal::stable(instance))?;
        }
        Ok(())
    }

    fn module(
        &mut self,
        def: DefId,
        path: &str,
        ancestors: &mut HashSet<DefId>,
    ) -> Result<(), String> {
        // Reexports can form cycles and therefore infinitely many valid path spellings.
        // Keep every acyclic alias path and explicitly record where recursion stops.
        if !ancestors.insert(def) {
            self.inventory.push(
                json!({"name": path, "definition": self.tcx.def_path_str(def),
                "kind": "module", "status": "recursive_reexport"}),
            );
            return Ok(());
        }
        let children = if let Some(local) = def.as_local() {
            self.tcx.module_children_local(local)
        } else {
            self.tcx.module_children(def)
        };
        for child in children {
            if !child.vis.is_public() {
                continue;
            }
            let Res::Def(kind, target) = child.res else {
                continue;
            };
            let name = format!("{path}::{}", child.ident.name.to_ident_string());
            match kind {
                DefKind::Mod => self.module(target, &name, ancestors)?,
                DefKind::Fn => self.function(name, target, "function", false)?,
                DefKind::Struct | DefKind::Enum | DefKind::Union | DefKind::TyAlias => {
                    self.inherent(name, target)?;
                }
                DefKind::Trait => {
                    for method in self.tcx.associated_items(target).in_definition_order() {
                        if method.is_fn() {
                            self.inventory.push(json!({
                                "name": format!("{name}::{}", method.name().to_ident_string()),
                                "definition": self.tcx.def_path_str(method.def_id),
                                "kind": "trait_declaration", "status": "requires_trait_resolution",
                            }));
                        }
                    }
                }
                _ => (),
            }
        }
        ancestors.remove(&def);
        Ok(())
    }

    fn inherent(&mut self, path: String, def: DefId) -> Result<(), String> {
        let generic = self
            .tcx
            .generics_of(def)
            .requires_monomorphization(self.tcx);
        let ty = self.tcx.type_of(def).instantiate_identity().skip_norm_wip();
        if !generic {
            let public_ty = self.tcx.normalize_erasing_regions(
                ty::TypingEnv::fully_monomorphized(),
                self.tcx.type_of(def).instantiate_identity(),
            );
            self.public_types
                .push((path.clone(), rustc_internal::stable(public_ty)));
        }
        let Some(adt) = ty.ty_adt_def() else {
            return Ok(());
        };
        self.types.push((path.clone(), adt.did(), ty, generic));
        for variant in adt.variants() {
            if variant.ctor_kind() != Some(CtorKind::Fn) {
                continue;
            }
            let constructor = variant.ctor_def_id().unwrap();
            if !self.tcx.visibility(constructor).is_public() {
                continue;
            }
            let name = if adt.is_enum() {
                format!("{path}::{}", variant.name.to_ident_string())
            } else if self.tcx.def_kind(def) != DefKind::TyAlias {
                path.clone()
            } else {
                // A tuple-struct alias is a type, not a constructor value in Rust.
                continue;
            };
            self.function(name, constructor, "constructor", generic)?;
        }
        for &implementation in self.tcx.inherent_impls(adt.did()) {
            let impl_ty = self
                .tcx
                .type_of(implementation)
                .instantiate_identity()
                .skip_norm_wip();
            // A concrete type alias must not inherit methods of a different specialization.
            if !generic
                && !impl_ty.has_non_region_param()
                && self.tcx.erase_and_anonymize_regions(ty)
                    != self.tcx.erase_and_anonymize_regions(impl_ty)
            {
                continue;
            }
            for method in self
                .tcx
                .associated_items(implementation)
                .in_definition_order()
            {
                if method.is_fn() && method.visibility(self.tcx).is_public() {
                    self.function(
                        format!("{path}::{}", method.name().to_ident_string()),
                        method.def_id,
                        "inherent_method",
                        generic,
                    )?;
                }
            }
        }
        Ok(())
    }

    fn public_traits(&mut self, krate: CrateNum) -> &HashMap<DefId, String> {
        self.traits.entry(krate).or_insert_with(|| {
            let mut traits = HashMap::new();
            let mut visited = HashSet::new();
            let mut pending =
                VecDeque::from([(krate.as_def_id(), self.tcx.crate_name(krate).to_string())]);
            while let Some((module, path)) = pending.pop_front() {
                if !visited.insert(module) {
                    continue;
                }
                let children = if let Some(local) = module.as_local() {
                    self.tcx.module_children_local(local)
                } else {
                    self.tcx.module_children(module)
                };
                let mut children: Vec<_> = children
                    .iter()
                    .filter(|child| child.vis.is_public())
                    .collect();
                children.sort_by_key(|child| child.ident.name.to_string());
                for child in children {
                    let Res::Def(kind, def) = child.res else {
                        continue;
                    };
                    let name = format!("{path}::{}", child.ident.name.to_ident_string());
                    match kind {
                        DefKind::Mod => pending.push_back((def, name)),
                        DefKind::Trait => {
                            traits.entry(def).or_insert(name);
                        }
                        _ => (),
                    }
                }
            }
            traits
        })
    }

    fn trait_impls(&mut self) -> Result<(), String> {
        // This is a finite inventory of explicit/derived impls from each public type's
        // defining crate and the facade. Blanket impl instantiations are not enumerable.
        let mut crates: HashSet<_> = self.types.iter().map(|(_, def, _, _)| def.krate).collect();
        crates.insert(LOCAL_CRATE);
        let implementations: Vec<_> = crates
            .into_iter()
            .flat_map(|krate| self.tcx.trait_impls_in_crate(krate).iter().copied())
            .collect();
        for implementation in implementations {
            if self.tcx.impl_polarity(implementation) != ty::ImplPolarity::Positive {
                continue;
            }
            let self_ty = self
                .tcx
                .type_of(implementation)
                .instantiate_identity()
                .skip_norm_wip();
            let Some(adt) = self_ty.ty_adt_def() else {
                continue;
            };
            let trait_ref = self
                .tcx
                .impl_trait_ref(implementation)
                .instantiate_identity()
                .skip_norm_wip();
            let Some(public_trait) = self
                .public_traits(trait_ref.def_id.krate)
                .get(&trait_ref.def_id)
                .cloned()
            else {
                continue;
            };
            let printed_trait = with_no_visible_paths!(with_no_trimmed_paths!(
                trait_ref.print_only_trait_path().to_string()
            ));
            let arguments = printed_trait
                .find('<')
                .map_or("", |offset| &printed_trait[offset..]);
            let trait_path = format!("{public_trait}{arguments}");
            let trait_definition = with_no_visible_paths!(with_no_trimmed_paths!(
                self.tcx.def_path_str(trait_ref.def_id)
            ));
            let owners: Vec<_> = self
                .types
                .iter()
                .filter(|(_, def, _, _)| *def == adt.did())
                .cloned()
                .collect();
            for (path, _, owner_ty, generic_owner) in owners {
                if !generic_owner
                    && !self_ty.has_non_region_param()
                    && self.tcx.erase_and_anonymize_regions(owner_ty)
                        != self.tcx.erase_and_anonymize_regions(self_ty)
                {
                    continue;
                }
                for method in self
                    .tcx
                    .associated_items(trait_ref.def_id)
                    .in_definition_order()
                {
                    if !method.is_fn() {
                        continue;
                    }
                    let name = format!(
                        "<{path} as {trait_path}>::{}",
                        method.name().to_ident_string()
                    );
                    let generic = generic_owner
                        || self
                            .tcx
                            .generics_of(implementation)
                            .requires_monomorphization(self.tcx)
                        || self
                            .tcx
                            .generics_of(method.def_id)
                            .own_requires_monomorphization();
                    let mut api = json!({"name": name, "owner": path, "trait": trait_path,
                        "trait_definition": trait_definition,
                        "definition": self.tcx.def_path_str(method.def_id),
                        "implementation": self.tcx.def_path_str(implementation),
                        "kind": "trait_method", "status": "requires_monomorphization"});
                    if Some(trait_ref.def_id) == self.tcx.lang_items().drop_trait() {
                        // Rust rejects explicit destructor calls. Drop glue is discovered
                        // from real MIR uses, not exposed as an ordinary callable API.
                        api["status"] = json!("compiler_managed");
                    } else if !generic {
                        let args = self
                            .tcx
                            .erase_and_anonymize_regions(trait_ref.args)
                            .extend_to(self.tcx, method.def_id, |param, _| {
                                assert!(matches!(param.kind, ty::GenericParamDefKind::Lifetime));
                                self.tcx.lifetimes.re_erased.into()
                            });
                        if self
                            .tcx
                            .instantiate_and_check_impossible_clauses((method.def_id, args))
                        {
                            api["status"] = json!("unsatisfied_predicates");
                        } else {
                            let instance = ty::Instance::try_resolve(
                                self.tcx,
                                ty::TypingEnv::fully_monomorphized(),
                                method.def_id,
                                args,
                            )
                            .map_err(|_| format!("cannot resolve public trait method {name}"))?
                            .ok_or_else(|| format!("cannot resolve public trait method {name}"))?;
                            api["status"] = json!(if sized_signature(self.tcx, instance) {
                                "monomorphic"
                            } else {
                                "unsupported_unsized_value"
                            });
                            api["definition"] = json!(self.tcx.def_path_str(instance.def_id()));
                            if sized_signature(self.tcx, instance) {
                                self.add_root(name, rustc_internal::stable(instance))?;
                            }
                        }
                    }
                    self.inventory.push(api);
                }
            }
        }
        Ok(())
    }
}
