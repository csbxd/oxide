use rustc_middle::ty::layout::ValidityRequirement;
use rustc_middle::ty::{TyCtxt, TypeVisitableExt};
use rustc_public::abi::{FieldsShape, Scalar, TagEncoding, ValueAbi, VariantsShape};
use rustc_public::mir::alloc::{AllocId, GlobalAlloc};
use rustc_public::mir::mono::{Instance, InstanceKind, StaticDef};
use rustc_public::mir::visit::{Location, MirVisitor};
use rustc_public::mir::{
    Body, CastKind, Mutability, PointerCoercion, Rvalue, StatementKind, Terminator, TerminatorKind,
};
use rustc_public::ty::{Allocation, ClosureKind, ConstantKind, MirConst, RigidTy, Ty, TyKind};
use rustc_public::{CrateDef, CrateDefType, rustc_internal};
use serde_json::{Value, json};
use std::collections::{HashSet, VecDeque};
use std::io::Write;
use std::path::Path;

type Result<T> = std::result::Result<T, String>;

pub fn program(tcx: TyCtxt<'_>, output: &Path) -> Result<()> {
    let mut export = Exporter::new(tcx);
    // A translated library is hosted by Go, so expose Rust's actual normal-exit
    // path (including rt::cleanup) independently of user-visible crate roots.
    let process_exit_symbol = tcx
        .get_diagnostic_item(rustc_span::Symbol::intern("process_exit"))
        .map(|def| {
            export.enqueue(rustc_internal::stable(rustc_middle::ty::Instance::mono(
                tcx, def,
            )))
        });
    let filter = std::env::var("OXIDE_ROOTS").unwrap_or_default();
    let discovered = crate::roots::discover(tcx, &filter)?;
    let mut roots = Vec::new();
    for (name, instance) in discovered.selected {
        let symbol = export.enqueue(instance);
        export.root_symbols.insert(symbol.clone());
        roots.push(json!({"name": name, "symbol": symbol}));
    }
    let mut public_types = Vec::new();
    for (name, ty) in discovered.public_types {
        export.api_type(ty)?;
        public_types.push(json!({"name":name,"type":ty}));
    }
    public_types.sort_by(|a, b| a["name"].as_str().cmp(&b["name"].as_str()));
    if roots.is_empty() && public_types.is_empty() {
        return Err(format!("no monomorphic export roots (filter {filter:?})"));
    }
    while let Some(instance) = export.pending.pop_front() {
        export.function(instance)?;
    }
    for root in &mut roots {
        let symbol = root["symbol"].as_str().unwrap();
        let (params, result) = export
            .root_signatures
            .get(symbol)
            .ok_or_else(|| format!("missing public signature for {symbol}"))?;
        root["params"] = json!(params);
        root["return"] = json!(result);
    }
    for ty in &mut export.types {
        if let Some(symbol) = export.reified_functions.get(&ty["id"].as_u64().unwrap()) {
            ty["function"] = json!(symbol);
        }
        if let Some(metadata) = export.api_metadata.get(&ty["id"].as_u64().unwrap()) {
            ty.as_object_mut()
                .unwrap()
                .extend(metadata.as_object().unwrap().clone());
        }
    }
    let mut api_types: Vec<_> = export
        .types
        .iter()
        .filter(|ty| {
            export
                .api_metadata
                .contains_key(&ty["id"].as_u64().unwrap())
        })
        .cloned()
        .collect();
    api_types.sort_by_key(|ty| ty["id"].as_u64());
    let mut data = json!({
        "compiler": "nightly-2026-09-15 (574ff7d98)",
        "target": tcx.sess.opts.target_triple.to_string(),
        "process_exit_symbol": process_exit_symbol,
        "panic_strategy": format!("{:?}", tcx.sess.panic_strategy()),
        "runtime_checks": {
            "UbChecks": tcx.sess.ub_checks(),
            "ContractChecks": tcx.sess.contract_checks(),
            "OverflowChecks": tcx.sess.overflow_checks(),
        },
        "vtables": export.vtables,
        "upcasts": export.upcasts,
    });
    // json!(values) serializes a borrowed Value tree into a second owned tree.
    // Move the existing graph so writing it does not double its live storage.
    data["roots"] = Value::Array(roots);
    data["public_api"] = Value::Array(discovered.public_api);
    data["public_types"] = Value::Array(public_types);
    data["api_types"] = Value::Array(api_types);
    export
        .public_drop_types
        .sort_by_key(|entry| entry["type"].as_u64());
    data["public_drop_types"] = Value::Array(export.public_drop_types);
    data["functions"] = Value::Array(export.functions);
    data["types"] = Value::Array(export.types);
    data["allocations"] = Value::Array(export.allocations);
    data["thread_locals"] = Value::Object(
        export
            .thread_locals
            .into_iter()
            .map(|(id, value)| (id.to_string(), value))
            .collect(),
    );
    let file = std::fs::File::create(output).map_err(|e| e.to_string())?;
    let mut writer = std::io::BufWriter::new(file);
    serde_json::to_writer(&mut writer, &data).map_err(|e| e.to_string())?;
    writer.flush().map_err(|e| e.to_string())?;
    // Keep the public API audit inexpensive even for a multi-gigabyte MIR graph.
    // These values come from the same completed export, not a second discovery pass.
    let api = json!({"compiler": data["compiler"], "target": data["target"],
        "process_exit_symbol": data["process_exit_symbol"],
        "roots": data["roots"], "public_api": data["public_api"],
        "public_drop_types": data["public_drop_types"],
        "public_types": data["public_types"], "api_types": data["api_types"]});
    let file =
        std::fs::File::create(output.with_extension("api.json")).map_err(|e| e.to_string())?;
    let mut writer = std::io::BufWriter::new(file);
    serde_json::to_writer(&mut writer, &api).map_err(|e| e.to_string())?;
    writer.flush().map_err(|e| e.to_string())
}

struct Exporter<'tcx> {
    tcx: TyCtxt<'tcx>,
    pending: VecDeque<Instance>,
    seen_functions: HashSet<Instance>,
    function_names: std::collections::HashMap<Instance, String>,
    root_symbols: HashSet<String>,
    root_signatures: std::collections::HashMap<String, (Vec<Ty>, Ty)>,
    public_drop_types: Vec<Value>,
    seen_public_drop_types: HashSet<Ty>,
    seen_api_types: HashSet<Ty>,
    api_metadata: std::collections::HashMap<u64, Value>,
    reified_functions: std::collections::HashMap<u64, String>,
    linked_functions: Option<std::collections::HashMap<String, Instance>>,
    seen_types: HashSet<Ty>,
    seen_allocations: HashSet<AllocId>,
    functions: Vec<Value>,
    exported_functions: std::collections::HashMap<String, usize>,
    types: Vec<Value>,
    allocations: Vec<Value>,
    immutable_allocations: std::collections::HashMap<Allocation, (AllocId, usize)>,
    vtables: std::collections::HashMap<String, u64>,
    upcasts: std::collections::HashMap<String, i64>,
    thread_locals: std::collections::HashMap<u64, Value>,
    unsize_seen: HashSet<(Ty, Ty)>,
}

impl<'tcx> Exporter<'tcx> {
    fn new(tcx: TyCtxt<'tcx>) -> Self {
        Self {
            tcx,
            pending: VecDeque::new(),
            seen_functions: HashSet::new(),
            function_names: std::collections::HashMap::new(),
            root_symbols: HashSet::new(),
            root_signatures: std::collections::HashMap::new(),
            public_drop_types: vec![],
            seen_public_drop_types: HashSet::new(),
            seen_api_types: HashSet::new(),
            api_metadata: std::collections::HashMap::new(),
            reified_functions: std::collections::HashMap::new(),
            linked_functions: None,
            seen_types: HashSet::new(),
            seen_allocations: HashSet::new(),
            functions: vec![],
            exported_functions: std::collections::HashMap::new(),
            types: vec![],
            allocations: vec![],
            immutable_allocations: std::collections::HashMap::new(),
            vtables: std::collections::HashMap::new(),
            upcasts: std::collections::HashMap::new(),
            thread_locals: std::collections::HashMap::new(),
            unsize_seen: HashSet::new(),
        }
    }

    fn enqueue(&mut self, instance: Instance) -> String {
        if let Some(name) = self.function_names.get(&instance) {
            return name.clone();
        }
        let symbol = instance.mangled_name();
        self.function_names.insert(instance, symbol.clone());
        if self.seen_functions.insert(instance) {
            self.pending.push_back(instance);
        }
        symbol
    }

    fn public_drop_type(&mut self, ty: Ty) -> Result<()> {
        let ty = crate::type_api::canonical(self.tcx, ty);
        let internal = rustc_internal::internal(self.tcx, ty);
        if !internal.needs_drop(self.tcx, rustc_middle::ty::TypingEnv::fully_monomorphized())
            || !self.seen_public_drop_types.insert(ty)
        {
            return Ok(());
        }
        if !ty.layout().map_err(|e| e.to_string())?.shape().is_sized() {
            return Err(format!("unsized public owned value: {internal}"));
        }
        self.ty(ty)?;
        let symbol = self.enqueue(Instance::resolve_drop_in_place(ty));
        let name = rustc_middle::ty::print::with_no_visible_paths!(
            rustc_middle::ty::print::with_no_trimmed_paths!(internal.to_string())
        );
        self.public_drop_types
            .push(json!({"type": ty, "name": name, "symbol": symbol}));
        Ok(())
    }

    fn api_type(&mut self, ty: Ty) -> Result<()> {
        if !self.seen_api_types.insert(ty) {
            return Ok(());
        }
        self.ty(ty)?;
        let mut description = crate::type_api::describe(self.tcx, ty)?;
        let canonical = crate::type_api::canonical(self.tcx, ty);
        if description.metadata["needs_drop"] == true
            && ty
                .layout()
                .map_err(|error| error.to_string())?
                .shape()
                .is_sized()
        {
            self.public_drop_type(canonical)?;
            description.metadata["drop_symbol"] =
                json!(self.enqueue(Instance::resolve_drop_in_place(canonical)));
        }
        for (field, instance, _) in description.operations {
            description.metadata[field] = json!(self.enqueue(instance));
        }
        if let Some(mut debug) = description.debug {
            for ty in debug.layouts {
                self.ty(ty)?;
            }
            for (field, instance) in debug.instances {
                debug.metadata[field] = json!(self.enqueue(instance));
            }
            description.metadata["debug"] = debug.metadata;
        }
        for (field, mut iteration) in description.iterations {
            for (name, instance) in iteration.instances {
                iteration.metadata[name] = json!(self.enqueue(instance));
            }
            description.metadata[field] = iteration.metadata;
        }
        if let Some(mut json) = description.json {
            for (name, instance) in json.instances {
                json.metadata[name] = json!(self.enqueue(instance));
            }
            description.metadata["json"] = json.metadata;
        }
        self.api_metadata.insert(
            serde_json::to_value(ty).unwrap().as_u64().unwrap(),
            description.metadata,
        );
        for child in description.children {
            self.api_type(child)?;
        }
        Ok(())
    }

    fn function(&mut self, instance: Instance) -> Result<()> {
        let symbol = self.function_names[&instance].clone();
        self.link_function(instance, &symbol)?;
        let mut entry = json!({"symbol": symbol, "name": instance.def.name(),
            "kind": format!("{:?}", instance.kind), "empty_drop": instance.is_empty_shim(),
            "track_caller": instance.requires_caller_location()});
        let def = rustc_internal::internal(self.tcx, instance.def.def_id());
        // Inline alloc sources and re-exports can change the public path. Only
        // rustc-marked allocator declarations select the allocator boundary.
        if instance.is_foreign_item()
            && self.tcx.codegen_fn_attrs(def).flags.contains(
                rustc_middle::middle::codegen_fn_attrs::CodegenFnAttrFlags::RUSTC_STD_INTERNAL_SYMBOL,
            )
            && matches!(
                self.tcx.item_name(def).as_str(),
                "__rust_alloc" | "__rust_alloc_zeroed" | "__rust_dealloc" | "__rust_realloc"
                    | "__rust_alloc_error_handler" | "__rust_no_alloc_shim_is_unstable_v2"
            )
        {
            entry["name"] = json!(format!("alloc::alloc::{}", self.tcx.item_name(def)));
        }
        if !def.is_local()
            && self.tcx.def_path_str(def) == "std::sys::args::unix::imp::argc_argv"
            && self
                .tcx
                .diagnostic_items(def.krate)
                .name_to_id
                .iter()
                .any(|(name, item)| {
                    name.as_str() == "process_exit"
                        && item.krate == def.krate
                        && self.tcx.def_path_str(*item) == "std::process::exit"
                })
        {
            // Rust's ELF init-array hook initializes this process-global state.
            // Go owns executable startup; retain all surrounding Rust args MIR
            // and identify only the OS getter by its compiler-known std crate.
            entry["runtime_boundary"] = json!("std_args");
        }
        if self.tcx.is_constructor(def) {
            if let Ok(abi) = instance.fn_abi() {
                if let TyKind::RigidTy(RigidTy::Adt(adt, _)) = abi.ret.ty.kind() {
                    let adt_internal = rustc_internal::internal(self.tcx, adt);
                    entry["constructor"] =
                        json!(adt_internal.variant_index_with_ctor_id(def).as_usize());
                }
            }
        }
        // Compiler intrinsics are an explicit backend boundary. Their fallback
        // MIR (when available) is kept; it is never replaced by a guessed body.
        if let Some(body) = instance.body() {
            if self.root_symbols.contains(&symbol) {
                // Use the same visible MIR signature as the Go entry point. A
                // reference/raw pointer does not transfer ownership of its pointee.
                let mut params = Vec::new();
                for (index, local) in body.arg_locals().iter().enumerate() {
                    if body.spread_arg() == Some(index + 1) {
                        let TyKind::RigidTy(RigidTy::Tuple(fields)) = local.ty.kind() else {
                            return Err(format!("non-tuple public spread argument in {symbol}"));
                        };
                        params.extend(fields);
                    } else {
                        params.push(local.ty);
                    }
                }
                let result = body.ret_local().ty;
                for &ty in params.iter().chain(std::iter::once(&result)) {
                    self.api_type(ty)?;
                }
                self.root_signatures
                    .insert(symbol.clone(), (params, result));
            }
            self.collect_metadata(&body)?;
            let mut found = Collect::default();
            found.visit_body(&body);
            for ty in found.types {
                self.ty(ty)?;
            }
            for c in found.constants {
                self.constant(&c)?;
            }
            let mut calls = serde_json::Map::new();
            let mut intrinsic_validity = serde_json::Map::new();
            let mut call_locations = serde_json::Map::new();
            let mut assert_calls = serde_json::Map::new();
            let mut call_untuple = serde_json::Map::new();
            for (bb, block) in body.blocks.iter().enumerate() {
                match &block.terminator.kind {
                    TerminatorKind::Assert { msg, .. } => {
                        let (callee, args, optional) = self.assert_call(instance, bb, msg)?;
                        call_locations.insert(
                            bb.to_string(),
                            self.call_location(
                                &body,
                                &block.terminator,
                                instance.requires_caller_location(),
                            )?,
                        );
                        assert_calls.insert(
                            bb.to_string(),
                            json!({
                                "symbol": self.enqueue(callee), "args": args, "optional": optional,
                            }),
                        );
                    }
                    TerminatorKind::Call {
                        func,
                        args: operands,
                        ..
                    } => {
                        let ty = func.ty(body.locals()).map_err(|e| e.to_string())?;
                        let internal = rustc_internal::internal(self.tcx, ty);
                        if internal.fn_sig(self.tcx).abi() == rustc_abi::ExternAbi::RustCall
                            && let Some(tuple) = operands.last()
                        {
                            let tuple_ty = tuple.ty(body.locals()).map_err(|e| e.to_string())?;
                            if !matches!(tuple_ty.kind(), TyKind::RigidTy(RigidTy::Tuple(_))) {
                                return Err(format!(
                                    "RustCall final argument is not a tuple: {tuple_ty:?}"
                                ));
                            }
                            call_untuple.insert(bb.to_string(), json!(operands.len() - 1));
                        }
                        if let Some((def, args)) = ty.kind().fn_def() {
                            let callee = Instance::resolve(def, args).map_err(|e| e.to_string())?;
                            if callee.requires_caller_location()
                                || callee.intrinsic_name().as_deref() == Some("caller_location")
                            {
                                call_locations.insert(
                                    bb.to_string(),
                                    self.call_location(
                                        &body,
                                        &block.terminator,
                                        instance.requires_caller_location(),
                                    )?,
                                );
                            }
                            if let Some(name) = callee.intrinsic_name() {
                                if let Some(requirement) = match name.as_str() {
                                    "assert_inhabited" => Some(ValidityRequirement::Inhabited),
                                    "assert_zero_valid" => Some(ValidityRequirement::Zero),
                                    "assert_mem_uninitialized_valid" => {
                                        Some(ValidityRequirement::UninitMitigated0x01Fill)
                                    }
                                    _ => None,
                                } {
                                    let args_internal =
                                        rustc_internal::internal(self.tcx, callee.args());
                                    let valid = self
                                        .tcx
                                        .check_validity_requirement((
                                            requirement,
                                            rustc_middle::ty::TypingEnv::fully_monomorphized()
                                                .as_query_input(args_internal.type_at(0)),
                                        ))
                                        .map_err(|e| e.to_string())?;
                                    intrinsic_validity.insert(bb.to_string(), json!(valid));
                                }
                            }
                            let target = match callee.kind {
                                InstanceKind::Virtual { idx } => format!("<virtual:{idx}>"),
                                InstanceKind::Intrinsic
                                    if matches!(
                                        callee.intrinsic_name().as_deref(),
                                        Some(
                                            "const_allocate"
                                                | "const_deallocate"
                                                | "carryless_mul"
                                                | "unchecked_funnel_shl"
                                                | "unchecked_funnel_shr"
                                                | "minimumf16"
                                                | "minimumf32"
                                                | "minimumf64"
                                                | "minimumf128"
                                                | "maximumf16"
                                                | "maximumf32"
                                                | "maximumf64"
                                                | "maximumf128"
                                        )
                                    ) =>
                                {
                                    // These intrinsics supply their runtime implementation
                                    // in Rust. Export that MIR instead of duplicating it.
                                    if callee.body().is_none() {
                                        return Err(format!(
                                            "missing Rust intrinsic body: {}",
                                            callee.name()
                                        ));
                                    }
                                    self.enqueue(callee)
                                }
                                InstanceKind::Intrinsic | InstanceKind::LlvmIntrinsic => {
                                    let intrinsic = callee
                                        .intrinsic_name()
                                        .filter(|n| n != "unknown")
                                        .unwrap_or_else(|| callee.mangled_name());
                                    format!("<intrinsic:{}>", intrinsic)
                                }
                                _ => self.enqueue(callee),
                            };
                            calls.insert(bb.to_string(), json!(target));
                        } else {
                            calls.insert(bb.to_string(), json!("<indirect>"));
                        }
                    }
                    TerminatorKind::Drop { place, .. } => {
                        let ty = place.ty(body.locals()).map_err(|e| e.to_string())?;
                        let callee = Instance::resolve_drop_in_place(ty);
                        if callee.requires_caller_location() {
                            call_locations.insert(
                                bb.to_string(),
                                self.call_location(
                                    &body,
                                    &block.terminator,
                                    instance.requires_caller_location(),
                                )?,
                            );
                        }
                        let target = match callee.kind {
                            InstanceKind::Virtual { idx } => format!("<virtual:{idx}>"),
                            _ => self.enqueue(callee),
                        };
                        calls.insert(bb.to_string(), json!(target));
                    }
                    _ => {}
                }
            }
            entry["body"] = serialize_body(body)?;
            entry["calls"] = calls.into();
            entry["assert_calls"] = assert_calls.into();
            entry["call_untuple"] = call_untuple.into();
            entry["intrinsic_validity"] = intrinsic_validity.into();
            entry["call_locations"] = call_locations.into();
        } else {
            // Foreign functions/intrinsics still have a typed signature.
            if let Ok(abi) = instance.fn_abi() {
                // rustc appends track_caller's hidden location to the ABI,
                // but callers and definitions in this IR use MIR parameters.
                let count = abi.args.len() - usize::from(instance.requires_caller_location());
                let args = &abi.args[..count];
                if self.root_symbols.contains(&symbol) {
                    self.api_type(abi.ret.ty)?;
                    for arg in args {
                        self.api_type(arg.ty)?;
                    }
                    self.root_signatures.insert(
                        symbol.clone(),
                        (args.iter().map(|arg| arg.ty).collect(), abi.ret.ty),
                    );
                }
                self.ty(abi.ret.ty)?;
                for a in args {
                    self.ty(a.ty)?;
                }
                entry["signature"] = json!({
                    "params": args.iter().map(|a|a.ty).collect::<Vec<_>>(),
                    "return": abi.ret.ty,
                    "abi": format!("{:?}", abi.conv),
                    "variadic": abi.c_variadic,
                    "fixed_count": abi.fixed_count,
                    "can_unwind": self.tcx.fn_abi_of_instance(
                        rustc_middle::ty::TypingEnv::fully_monomorphized().as_query_input((
                            rustc_internal::internal(self.tcx, instance),
                            rustc_middle::ty::List::empty(),
                        )),
                    ).map_err(|error| format!("function ABI: {error:?}"))?.can_unwind,
                });
            }
        }
        if self.functions.len() % 1000 == 0 {
            eprintln!(
                "oxide-rs: {} instances, {} types, {} pending",
                self.functions.len(),
                self.types.len(),
                self.pending.len()
            );
        }
        if let Some(index) = self.exported_functions.get(&symbol) {
            let previous = &self.functions[*index];
            if function_signature(previous) != function_signature(&entry) {
                return Err(format!("conflicting signatures for symbol {symbol}"));
            }
            if previous.get("body").is_none() && entry.get("body").is_some() {
                self.functions[*index] = entry;
            }
        } else {
            self.exported_functions.insert(symbol, self.functions.len());
            self.functions.push(entry);
        }
        Ok(())
    }

    fn link_function(&mut self, instance: Instance, symbol: &str) -> Result<()> {
        if !instance.is_foreign_item() {
            return Ok(());
        }
        if let Some(def) = self.tcx.lang_items().panic_impl() {
            if !self.tcx.is_foreign_item(def) {
                let implementation: Instance =
                    rustc_internal::stable(rustc_middle::ty::Instance::mono(self.tcx, def));
                if implementation.mangled_name() == symbol {
                    self.enqueue(implementation);
                    return Ok(());
                }
            }
        }
        use rustc_middle::middle::codegen_fn_attrs::CodegenFnAttrFlags;
        let def = rustc_internal::internal(self.tcx, instance.def.def_id());
        if !self
            .tcx
            .codegen_fn_attrs(def)
            .flags
            .contains(CodegenFnAttrFlags::RUSTC_STD_INTERNAL_SYMBOL)
        {
            return Ok(());
        }
        if self.linked_functions.is_none() {
            let mut definitions = std::collections::HashMap::new();
            let mut crates = rustc_public::external_crates();
            crates.push(rustc_public::local_crate());
            for krate in crates {
                let crate_num = rustc_internal::crate_num(&krate);
                if self.tcx.is_panic_runtime(crate_num)
                    && self.tcx.required_panic_strategy(crate_num)
                        != Some(self.tcx.sess.panic_strategy())
                {
                    continue;
                }
                // Linkable definitions are recorded explicitly in metadata.
                // Enumerating every DefIndex also visits intentionally sparse
                // proc-macro metadata, which cannot be queried as Rust MIR.
                for &(symbol, _) in self.tcx.exported_non_generic_symbols(crate_num) {
                    let rustc_middle::middle::exported_symbols::ExportedSymbol::NonGeneric(def) =
                        symbol
                    else {
                        continue;
                    };
                    if !matches!(
                        self.tcx.def_kind(def),
                        rustc_hir::def::DefKind::Fn | rustc_hir::def::DefKind::AssocFn
                    ) {
                        continue;
                    }
                    if self.tcx.is_foreign_item(def)
                        || self
                            .tcx
                            .generics_of(def)
                            .requires_monomorphization(self.tcx)
                        || !self
                            .tcx
                            .codegen_fn_attrs(def)
                            .flags
                            .contains(CodegenFnAttrFlags::RUSTC_STD_INTERNAL_SYMBOL)
                    {
                        continue;
                    }
                    let implementation: Instance =
                        rustc_internal::stable(rustc_middle::ty::Instance::mono(self.tcx, def));
                    let name = implementation.mangled_name();
                    if let Some(previous) = definitions.insert(name.clone(), implementation) {
                        if previous != implementation {
                            return Err(format!("multiple Rust definitions for symbol {name}"));
                        }
                    }
                }
            }
            self.linked_functions = Some(definitions);
        }
        if let Some(implementation) = self.linked_functions.as_ref().unwrap().get(symbol).copied() {
            self.enqueue(implementation);
        }
        Ok(())
    }

    fn assert_call(
        &self,
        instance: Instance,
        bb: usize,
        message: &rustc_public::mir::AssertMessage,
    ) -> Result<(Instance, Vec<rustc_public::mir::Operand>, bool)> {
        use rustc_hir::attrs::lang_items::LangItem;
        use rustc_public::mir::AssertMessage;
        let internal = rustc_internal::internal(self.tcx, instance);
        let kind = match internal.def {
            rustc_middle::ty::InstanceKind::Intrinsic(def) => {
                rustc_middle::ty::InstanceKind::Item(def)
            }
            kind => kind,
        };
        let body = self.tcx.instance_mir(kind);
        let terminator = body.basic_blocks.iter().nth(bb).unwrap().terminator();
        let rustc_middle::mir::TerminatorKind::Assert { msg, .. } = &terminator.kind else {
            return Err(format!(
                "assert metadata differs from MIR at {instance:?} bb{bb}"
            ));
        };
        let (lang_item, args) = match message {
            AssertMessage::BoundsCheck { len, index } => {
                (LangItem::PanicBoundsCheck, vec![index.clone(), len.clone()])
            }
            AssertMessage::MisalignedPointerDereference { required, found } => (
                LangItem::PanicMisalignedPointerDereference,
                vec![required.clone(), found.clone()],
            ),
            AssertMessage::InvalidEnumConstruction(value) => {
                (LangItem::PanicInvalidEnumConstruction, vec![value.clone()])
            }
            _ => (msg.panic_function(), vec![]),
        };
        let def = self
            .tcx
            .lang_items()
            .get(lang_item)
            .ok_or_else(|| format!("missing assertion panic lang item {lang_item:?}"))?;
        let callee = rustc_internal::stable(rustc_middle::ty::Instance::mono(self.tcx, def));
        Ok((callee, args, msg.is_optional_overflow_check()))
    }

    fn call_location(
        &mut self,
        body: &Body,
        terminator: &Terminator,
        tracked: bool,
    ) -> Result<Value> {
        // caller_location treats the inherited value as opaque. A marker tells
        // us whether rustc forwards that parameter or chooses an inlined span.
        let marker = MirConst::from_bool(false);
        let location = body.caller_location(terminator, tracked.then(|| marker.clone()));
        if location == marker {
            return Ok(json!({"inherited": true}));
        }
        self.ty(location.ty())?;
        let ConstantKind::Allocated(allocation) = location.kind() else {
            return Err(format!("caller location is not allocated: {location:?}"));
        };
        let [(0, provenance)] = allocation.provenance.ptrs.as_slice() else {
            return Err(format!("invalid caller location pointer: {allocation:?}"));
        };
        let offset = allocation.read_uint().map_err(|e| e.to_string())?;
        self.allocation(provenance.0)?;
        Ok(
            json!({"allocation": provenance.0, "offset": u64::try_from(offset).map_err(|e| e.to_string())?}),
        )
    }

    fn collect_metadata(&mut self, body: &Body) -> Result<()> {
        for block in &body.blocks {
            for stmt in &block.statements {
                if let StatementKind::Assign(
                    _,
                    Rvalue::Cast(
                        CastKind::PointerCoercion(
                            PointerCoercion::ReifyFnPointer(_)
                            | PointerCoercion::ClosureFnPointer(_),
                        ),
                        operand,
                        _,
                    ),
                ) = &stmt.kind
                {
                    let ty = operand.ty(body.locals()).map_err(|e| e.to_string())?;
                    let instance = match ty.kind() {
                        TyKind::RigidTy(RigidTy::FnDef(def, args)) => {
                            Instance::resolve_for_fn_ptr(def, &args)
                        }
                        TyKind::RigidTy(RigidTy::Closure(def, args)) => {
                            Instance::resolve_closure(def, &args, ClosureKind::FnOnce)
                        }
                        _ => return Err(format!("reified function has non-callable type: {ty:?}")),
                    };
                    let instance = instance.map_err(|e| e.to_string())?;
                    let symbol = self.enqueue(instance);
                    let id = serde_json::to_value(ty)
                        .map_err(|e| e.to_string())?
                        .as_u64()
                        .unwrap();
                    self.reified_functions.insert(id, symbol);
                }
                if let StatementKind::Assign(_, Rvalue::ThreadLocalRef(item)) = &stmt.kind {
                    let id = serde_json::to_value(item)
                        .map_err(|e| e.to_string())?
                        .as_u64()
                        .ok_or("thread-local item id")?;
                    if !self.thread_locals.contains_key(&id) {
                        let stat = StaticDef::try_from(*item).map_err(|e| e.to_string())?;
                        if self
                            .tcx
                            .is_foreign_item(rustc_internal::internal(self.tcx, stat.def_id()))
                        {
                            let external = self.external_static(stat)?;
                            self.thread_locals.insert(id, json!({"external": external}));
                            continue;
                        }
                        match stat.eval_initializer() {
                            Ok(data) => {
                                self.allocation_refs(&data)?;
                                self.thread_locals.insert(id, json!(data));
                            }
                            Err(e) => {
                                self.thread_locals.insert(
                                    id,
                                    json!({"unsupported": format!("{}: {e}", stat.name())}),
                                );
                            }
                        }
                    }
                }
                if let StatementKind::Assign(
                    _,
                    Rvalue::Cast(CastKind::PointerCoercion(PointerCoercion::Unsize), operand, dst),
                ) = &stmt.kind
                {
                    let src = operand.ty(body.locals()).map_err(|e| e.to_string())?;
                    self.unsize_vtables(src, *dst)?;
                }
            }
        }
        Ok(())
    }

    fn unsize_vtables(&mut self, src: Ty, dst: Ty) -> Result<()> {
        if !self.unsize_seen.insert((src, dst)) {
            return Ok(());
        }
        match (src.kind(), dst.kind()) {
            (
                TyKind::RigidTy(RigidTy::Ref(_, a, _) | RigidTy::RawPtr(a, _)),
                TyKind::RigidTy(RigidTy::Ref(_, b, _) | RigidTy::RawPtr(b, _)),
            ) => self.unsize_tail(a, b),
            (TyKind::RigidTy(RigidTy::Adt(a, aa)), TyKind::RigidTy(RigidTy::Adt(b, ba)))
                if a == b =>
            {
                let fields = a.variants()[0].fields();
                for field in fields {
                    let left = field.ty_with_args(&aa);
                    let right = field.ty_with_args(&ba);
                    if left != right {
                        self.unsize_vtables(left, right)?;
                    }
                }
                Ok(())
            }
            _ => self.unsize_tail(src, dst),
        }
    }

    fn unsize_tail(&mut self, src: Ty, dst: Ty) -> Result<()> {
        if rustc_internal::internal(self.tcx, src).has_escaping_bound_vars() {
            return Err(format!(
                "unsize source has escaping bound variables: {src:?}"
            ));
        }
        if let TyKind::RigidTy(RigidTy::Pat(inner, _)) = src.kind() {
            return self.unsize_tail(inner, dst);
        }
        if let TyKind::RigidTy(RigidTy::Pat(inner, _)) = dst.kind() {
            return self.unsize_tail(src, inner);
        }
        if let (
            TyKind::RigidTy(RigidTy::Ref(_, a, _) | RigidTy::RawPtr(a, _)),
            TyKind::RigidTy(RigidTy::Ref(_, b, _) | RigidTy::RawPtr(b, _)),
        ) = (src.kind(), dst.kind())
        {
            return self.unsize_tail(a, b);
        }
        if matches!(src.kind(), TyKind::RigidTy(RigidTy::Dynamic(..)))
            && matches!(dst.kind(), TyKind::RigidTy(RigidTy::Dynamic(..)))
        {
            let a = rustc_internal::internal(self.tcx, src);
            let b = rustc_internal::internal(self.tcx, dst);
            let (rustc_middle::ty::Dynamic(pa, _), rustc_middle::ty::Dynamic(pb, _)) =
                (a.kind(), b.kind())
            else {
                unreachable!()
            };
            let slot = if pa.principal_def_id() == pb.principal_def_id()
                || pb.principal_def_id().is_none()
            {
                None
            } else {
                self.tcx.supertrait_vtable_slot((a, b))
            };
            self.upcasts.insert(
                format!(
                    "{}/{}",
                    serde_json::to_value(src).unwrap(),
                    serde_json::to_value(dst).unwrap()
                ),
                slot.map_or(-1, |v| v as i64),
            );
            return Ok(());
        }
        if matches!(dst.kind(), TyKind::RigidTy(RigidTy::Dynamic(..))) {
            let principal = dst.kind().trait_principal();
            if !src.layout().map_err(|e| e.to_string())?.shape().is_sized() {
                return Ok(());
            }
            // Keep the binder intact. Extracting principal.value and wrapping it
            // in a dummy binder is invalid for higher-ranked Fn traits.
            let global = GlobalAlloc::VTable(src, principal);
            let id = global
                .vtable_allocation()
                .ok_or_else(|| "missing vtable allocation".to_string())?;
            let id_num = serde_json::to_value(id)
                .map_err(|e| e.to_string())?
                .as_u64()
                .unwrap();
            self.vtables.insert(
                format!(
                    "{}/{}",
                    serde_json::to_value(src).unwrap(),
                    serde_json::to_value(dst).unwrap()
                ),
                id_num,
            );
            // Allocation relocations contain the actual method instances.
            return self.allocation(id);
        }
        if let (TyKind::RigidTy(RigidTy::Adt(a, aa)), TyKind::RigidTy(RigidTy::Adt(b, ba))) =
            (src.kind(), dst.kind())
        {
            if a == b {
                if let Some(field) = a.variants()[0].fields().last() {
                    return self.unsize_tail(field.ty_with_args(&aa), field.ty_with_args(&ba));
                }
            }
        }
        Ok(())
    }

    fn ty(&mut self, ty: Ty) -> Result<()> {
        self.ty_inner(ty)
    }

    fn ty_inner(&mut self, ty: Ty) -> Result<()> {
        if !self.seen_types.insert(ty) {
            return Ok(());
        }
        let ty_kind = ty.kind();
        let name = format!("type#{}", serde_json::to_value(ty).unwrap_or(json!(0)));
        let mut info = json!({"id":ty, "name":name});
        if matches!(ty_kind, TyKind::RigidTy(RigidTy::Dynamic(..))) {
            info["kind"] = json!("dynamic");
            info["size"] = json!(0);
            info["align"] = json!(1);
            info["sized"] = json!(false);
            self.types.push(info);
            return Ok(());
        }
        if let TyKind::RigidTy(RigidTy::FnDef(def, args)) = ty_kind {
            info["kind"] = json!("fn");
            if !rustc_internal::internal(self.tcx, &args).has_escaping_bound_vars() {
                if let Ok(instance) = Instance::resolve(def, &args) {
                    if !matches!(
                        instance.kind,
                        InstanceKind::Virtual { .. }
                            | InstanceKind::Intrinsic
                            | InstanceKind::LlvmIntrinsic
                    ) {
                        info["function"] = json!(self.enqueue(instance));
                    }
                }
            }
            info["size"] = json!(0);
            info["align"] = json!(1);
            info["sized"] = json!(true);
            self.types.push(info);
            return Ok(());
        }
        match ty.layout() {
            Ok(layout) => {
                let l = layout.shape();
                info["size"] = json!(l.size.bytes());
                info["align"] = json!(l.abi_align);
                info["sized"] = json!(l.is_sized());
                info["fields"] = json!(offsets(&l.fields));
                let primitive = |scalar: &Scalar| match scalar {
                    Scalar::Initialized { value, .. } | Scalar::Union { value } => *value,
                };
                match &l.abi {
                    ValueAbi::Scalar(scalar) => {
                        info["value_abi"] = json!("Scalar");
                        info["abi_scalar"] = json!(primitive(scalar));
                    }
                    ValueAbi::ScalarPair { a, b, b_offset } => {
                        info["value_abi"] = json!("ScalarPair");
                        info["abi_pair"] = json!({
                            "a": primitive(a), "b": primitive(b), "b_offset": b_offset.bytes(),
                        });
                    }
                    ValueAbi::Vector { .. } => info["value_abi"] = json!("Vector"),
                    ValueAbi::ScalableVector { .. } => info["value_abi"] = json!("ScalableVector"),
                    ValueAbi::Aggregate { .. } => info["value_abi"] = json!("Aggregate"),
                }
                match l.variants {
                    VariantsShape::Multiple {
                        tag,
                        tag_encoding,
                        tag_field,
                        variants,
                    } => {
                        info["variants"] = json!(
                            variants
                                .iter()
                                .map(|v| v.offsets.iter().map(|o| o.bytes()).collect::<Vec<_>>())
                                .collect::<Vec<_>>()
                        );
                        let (Scalar::Initialized { value, .. } | Scalar::Union { value }) = tag;
                        let mut tag_info = json!({"offset":offsets(&l.fields)[tag_field],
                            "size":value.size(&rustc_public::target::MachineInfo::target()).bytes(), "primitive":value});
                        match tag_encoding {
                            TagEncoding::Direct => tag_info["encoding"] = json!("direct"),
                            TagEncoding::Niche {
                                untagged_variant,
                                niche_variants,
                                niche_start,
                            } => {
                                tag_info["encoding"] = json!("niche");
                                tag_info["untagged"] = json!(untagged_variant);
                                tag_info["first"] = json!(niche_variants.start());
                                tag_info["last"] = json!(niche_variants.end());
                                tag_info["start"] = json!(niche_start.to_string());
                            }
                        }
                        info["tag"] = tag_info;
                    }
                    VariantsShape::Single { index } => {
                        info["variant"] = json!(index);
                    }
                    VariantsShape::Empty => {}
                }
            }
            Err(e) => {
                info["layout_error"] = json!(e.to_string());
            }
        }
        match ty_kind {
            TyKind::RigidTy(RigidTy::Bool) => info["kind"] = json!("bool"),
            TyKind::RigidTy(RigidTy::Char) => info["kind"] = json!("char"),
            TyKind::RigidTy(RigidTy::Int(i)) => {
                info["kind"] = json!(format!("{i:?}").to_lowercase())
            }
            TyKind::RigidTy(RigidTy::Uint(i)) => {
                info["kind"] = json!(format!("{i:?}").to_lowercase())
            }
            TyKind::RigidTy(RigidTy::Float(i)) => {
                info["kind"] = json!(format!("{i:?}").to_lowercase())
            }
            TyKind::RigidTy(RigidTy::Ref(_, inner, _) | RigidTy::RawPtr(inner, _)) => {
                info["kind"] = json!("pointer");
                info["pointee"] = json!(inner);
                self.ty(inner)?;
            }
            TyKind::RigidTy(RigidTy::Array(inner, len)) => {
                info["kind"] = json!("array");
                info["element"] = json!(inner);
                info["length"] = json!(len.eval_target_usize().map_err(|e| e.to_string())?);
                self.ty(inner)?;
            }
            TyKind::RigidTy(RigidTy::Slice(inner)) => {
                info["kind"] = json!("slice");
                info["element"] = json!(inner);
                self.ty(inner)?;
            }
            TyKind::RigidTy(RigidTy::Str) => info["kind"] = json!("str"),
            TyKind::RigidTy(RigidTy::Pat(inner, _)) => match inner.kind() {
                TyKind::RigidTy(RigidTy::Ref(_, p, _) | RigidTy::RawPtr(p, _)) => {
                    info["kind"] = json!("pointer");
                    info["pointee"] = json!(p);
                    self.ty(p)?;
                }
                _ => {
                    info["kind"] = json!("aggregate");
                }
            },
            TyKind::RigidTy(RigidTy::FnDef(def, args)) => {
                info["kind"] = json!("fn");
                // Function items are zero-sized, but a cast can reify one.
                if let Ok(i) = Instance::resolve(def, &args) {
                    info["function"] = json!(self.enqueue(i));
                }
            }
            TyKind::RigidTy(RigidTy::FnPtr(signature)) => {
                info["kind"] = json!("fnptr");
                let abi = signature.fn_ptr_abi().map_err(|e| e.to_string())?;
                info["fn_abi"] = json!(format!("{:?}", abi.conv));
                info["fn_variadic"] = json!(abi.c_variadic);
                info["fn_fixed_count"] = json!(abi.fixed_count);
                // A signature's late-bound regions only exist inside its binder.
                // Erase them before exporting parameter layouts; using sig.value
                // directly lets bound regions escape into rustc layout queries.
                let internal = rustc_internal::internal(self.tcx, ty);
                info["fn_can_unwind"] = json!(
                    self.tcx
                        .fn_abi_of_fn_ptr(
                            rustc_middle::ty::TypingEnv::fully_monomorphized().as_query_input((
                                internal.fn_sig(self.tcx),
                                rustc_middle::ty::List::empty(),
                            )),
                        )
                        .map_err(|error| format!("function pointer ABI: {error:?}"))?
                        .can_unwind
                );
                let sig = self
                    .tcx
                    .instantiate_bound_regions_with_erased(internal.fn_sig(self.tcx));
                let inputs: Vec<Ty> = sig
                    .inputs()
                    .iter()
                    .map(|t| rustc_internal::stable(*t))
                    .collect();
                // Keep logical parameters intact, but describe the compiler's
                // RustCall ABI explicitly. fn_ptr_abi().conv normalizes it to
                // Rust and cannot tell callback consumers to unpack the tuple.
                let spread = if internal.fn_sig(self.tcx).abi() == rustc_abi::ExternAbi::RustCall {
                    let index = inputs
                        .len()
                        .checked_sub(1)
                        .ok_or("RustCall function pointer has no tuple parameter")?;
                    if !matches!(inputs[index].kind(), TyKind::RigidTy(RigidTy::Tuple(_))) {
                        return Err(
                            "RustCall function pointer final parameter is not a tuple".into()
                        );
                    }
                    index as i64
                } else {
                    -1
                };
                info["fn_spread_arg"] = json!(spread);
                let output = rustc_internal::stable(sig.output());
                info["fn_inputs"] = json!(inputs);
                info["fn_output"] = json!(output);
                for t in inputs {
                    self.ty(t)?
                }
                self.ty(output)?;
            }
            TyKind::RigidTy(RigidTy::Closure(def, args)) => {
                info["kind"] = json!("aggregate");
                for kind in [ClosureKind::Fn, ClosureKind::FnMut, ClosureKind::FnOnce] {
                    if let Ok(instance) = Instance::resolve_closure(def, &args, kind) {
                        info["function"] = json!(self.enqueue(instance));
                        break;
                    }
                }
            }
            TyKind::RigidTy(RigidTy::Tuple(fields)) => {
                info["kind"] = json!("aggregate");
                info["field_types"] = json!(fields);
                for t in fields {
                    self.ty(t)?;
                }
            }
            TyKind::RigidTy(RigidTy::Adt(def, args)) => {
                info["kind"] = json!("aggregate");
                info["adt_kind"] = json!(format!("{:?}", def.kind()));
                info["pack"] = json!(def.repr().pack.unwrap_or(0));
                let mut field_types = Vec::new();
                for variant in def.variants_iter() {
                    let mut fields = Vec::new();
                    for field in variant.fields() {
                        let ft = field.ty_with_args(&args);
                        self.ty(ft)?;
                        fields.push(ft);
                    }
                    field_types.push(fields);
                }
                info["variant_field_types"] = json!(field_types);
                info["variant_names"] =
                    json!(def.variants_iter().map(|v| v.name()).collect::<Vec<_>>());
                if def.kind() == rustc_public::ty::AdtKind::Enum {
                    info["discriminants"] = json!(
                        def.variants_iter()
                            .map(|v| def.discriminant_for_variant(v.idx()).val.to_string())
                            .collect::<Vec<_>>()
                    );
                }
            }
            TyKind::RigidTy(RigidTy::Never) => info["kind"] = json!("never"),
            TyKind::RigidTy(RigidTy::Coroutine(def, args)) => {
                info["kind"] = json!("aggregate");
                let count = info["variants"].as_array().map_or_else(
                    || {
                        info["variant"]
                            .as_u64()
                            .map_or(0, |index| index as usize + 1)
                    },
                    Vec::len,
                );
                info["discriminants"] = json!(
                    (0..count)
                        .map(|index| def
                            .discriminant_for_variant(
                                &args,
                                rustc_internal::stable(rustc_abi::VariantIdx::from_usize(index))
                            )
                            .val
                            .to_string())
                        .collect::<Vec<_>>()
                );
            }
            other => {
                info["kind"] = json!("unsupported");
                info["detail"] = json!(format!("{other:?}"));
            }
        }
        self.types.push(info);
        Ok(())
    }

    fn constant(&mut self, c: &MirConst) -> Result<()> {
        match c.kind() {
            ConstantKind::Allocated(a) => self.allocation_refs(a)?,
            ConstantKind::Ty(t) => {
                if let rustc_public::ty::TyConstKind::Value(_, a) = t.kind() {
                    self.allocation_refs(a)?;
                }
            }
            _ => {}
        }
        Ok(())
    }

    fn allocation_refs(&mut self, a: &Allocation) -> Result<()> {
        for (_, prov) in &a.provenance.ptrs {
            self.allocation(prov.0)?;
        }
        Ok(())
    }

    fn external_static(&mut self, stat: StaticDef) -> Result<Value> {
        let def = rustc_internal::internal(self.tcx, stat.def_id());
        let attrs = self.tcx.codegen_fn_attrs(def);
        let weak = attrs.import_linkage == Some(rustc_hir::attrs::Linkage::ExternalWeak);
        let symbol = self
            .tcx
            .symbol_name(rustc_middle::ty::Instance::mono(self.tcx, def))
            .name;
        let ty = stat.ty();
        self.ty(ty)?;
        let mut value = json!({
            "name": stat.name(), "symbol": symbol, "weak": weak,
            "linkage": attrs.import_linkage.map_or("External".to_string(), |l| format!("{l:?}")),
            "kind": "static", "type": ty,
        });
        // LLVM represents a weak function import as a private pointer slot
        // whose value is the function's linked address, or null if absent.
        let internal = rustc_internal::internal(self.tcx, ty);
        if weak
            && let rustc_middle::ty::Adt(adt, args) = internal.kind()
            && self
                .tcx
                .is_lang_item(adt.did(), rustc_hir::attrs::lang_items::LangItem::Option)
            && let rustc_middle::ty::FnPtr(..) = args.type_at(0).kind()
        {
            let function_type = rustc_internal::stable(args.type_at(0));
            self.ty(function_type)?;
            value["kind"] = json!("function");
            value["function_type"] = json!(function_type);
        }
        Ok(value)
    }

    fn allocation(&mut self, id: AllocId) -> Result<()> {
        if !self.seen_allocations.insert(id) {
            return Ok(());
        }
        let mut data = json!({"id":id});
        match GlobalAlloc::from(id) {
            GlobalAlloc::Memory(a) => {
                self.allocation_refs(&a)?;
                // rustc pools anonymous read-only initializers, upgrading the
                // shared alignment. Named statics and mutable memory stay unique.
                if a.mutability == Mutability::Not
                    && !a.bytes.is_empty()
                    && a.bytes.iter().all(Option::is_some)
                {
                    let mut key = a.clone();
                    key.align = 1;
                    if let Some(&(target, index)) = self.immutable_allocations.get(&key) {
                        let align = self.allocations[index]["memory"]["align"]
                            .as_u64()
                            .ok_or("missing pooled allocation alignment")?;
                        self.allocations[index]["memory"]["align"] = json!(align.max(a.align));
                        data["alias"] = json!(target);
                    } else {
                        self.immutable_allocations
                            .insert(key, (id, self.allocations.len()));
                        data["memory"] = json!(a);
                    }
                } else {
                    data["memory"] = json!(a);
                }
            }
            GlobalAlloc::Function(i) => {
                data["function"] = json!(self.enqueue(i));
            }
            GlobalAlloc::Static(s)
                if self
                    .tcx
                    .is_foreign_item(rustc_internal::internal(self.tcx, s.def_id())) =>
            {
                data["external"] = self.external_static(s)?;
            }
            GlobalAlloc::Static(s) => match s.eval_initializer() {
                Ok(a) => {
                    self.allocation_refs(&a)?;
                    data["memory"] = json!(a);
                }
                Err(e) => data["unsupported"] = json!(format!("{}: {e}", s.name())),
            },
            v @ GlobalAlloc::VTable(..) => {
                if let Some(alloc) = v.vtable_allocation() {
                    self.allocation(alloc)?;
                    data["alias"] = json!(alloc);
                } else {
                    data["unsupported"] = json!(format!("{v:?}"));
                }
            }
            GlobalAlloc::TypeId { .. } => {
                // As in rustc_codegen_llvm::common::alloc_to_backend, the
                // TypeId hash is already in the pointer offset. Its CTFE-only
                // provenance contributes a zero base address at runtime.
                data["address"] = json!(0);
            }
        }
        self.allocations.push(data);
        Ok(())
    }
}

fn function_signature(function: &Value) -> Value {
    if let Some(body) = function.get("body") {
        let count = body["arg_count"].as_u64().unwrap() as usize;
        let locals = body["locals"].as_array().unwrap();
        json!({"params": locals[1..=count].iter().map(|local| &local["ty"]).collect::<Vec<_>>(),
            "return": locals[0]["ty"]})
    } else {
        json!({"params": function["signature"]["params"], "return": function["signature"]["return"]})
    }
}

fn serialize_body(mut body: Body) -> Result<Value> {
    let blocks = std::mem::take(&mut body.blocks);
    let mut value = serde_json::to_value(body).map_err(|e| e.to_string())?;
    let mut serialized = Vec::with_capacity(blocks.len());
    for block in blocks {
        let terminator = &block.terminator;
        let TerminatorKind::SwitchInt { discr, targets } = &terminator.kind else {
            serialized.push(serde_json::to_value(block).map_err(|e| e.to_string())?);
            continue;
        };
        // serde_json::Value only stores 64-bit integers. MIR switch values are
        // u128 bit patterns, including negative i128 discriminants, so preserve
        // every bit as a decimal string rather than passing through a float.
        serialized.push(json!({
            "statements": block.statements,
            "terminator": {
                "source_info": terminator.source_info,
                "kind": {"SwitchInt": {
                    "discr": discr,
                    "targets": {
                        "branches": targets.branches().map(|(value, target)| (value.to_string(), target)).collect::<Vec<_>>(),
                        "otherwise": targets.otherwise(),
                    }
                }}
            }
        }));
    }
    value["blocks"] = json!(serialized);
    Ok(value)
}

fn offsets(fields: &FieldsShape) -> Vec<usize> {
    match fields {
        FieldsShape::Primitive => vec![],
        FieldsShape::Arbitrary { offsets } => offsets.iter().map(|o| o.bytes()).collect(),
        FieldsShape::Union(n) => vec![0; n.get()],
        FieldsShape::Array { stride, count } => {
            // Array element offsets are computed using stride in the backend.
            if *count == 0 {
                vec![]
            } else {
                vec![0, stride.bytes()]
            }
        }
    }
}

#[derive(Default)]
struct Collect {
    types: Vec<Ty>,
    constants: Vec<MirConst>,
}
impl MirVisitor for Collect {
    fn visit_ty(&mut self, ty: &Ty, _: Location) {
        self.types.push(*ty);
    }
    fn visit_mir_const(&mut self, c: &MirConst, loc: Location) {
        self.constants.push(c.clone());
        self.super_mir_const(c, loc);
    }
}
