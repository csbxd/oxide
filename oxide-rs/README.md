# oxide-rs

This crate is the Rust-side compiler frontend used by `oxide-go`. It uses the
pinned `rustc_public` MIR interface instead of translating syntax trees. Cargo
performs cfg expansion, macro expansion, type checking, monomorphization and
drop elaboration; the exporter records the resulting MIR, actual target
layouts, static allocations and reachable call graph.

`build.sh` installs the pinned nightly compiler into `oxide/.cache` and builds
`bin/oxide-rs`. Unknown or compiler-dependent layouts are obtained from rustc
inside the exporter instead of guessed in Go. The compiler revision is part of
the MIR interchange contract; changing it requires regenerating the output.

Run the frontend regression checks after building:

```sh
sh oxide/oxide-rs/build.sh
python3 oxide/oxide-rs/tests/check.py
python3 oxide/oxide-rs/tests/check_roots.py
python3 oxide/oxide-rs/tests/check_type_api.py
python3 oxide/oxide-rs/tests/check_json_api.py
python3 oxide/oxide-rs/tests/check_panic.py
```

Default roots follow rustc's public module namespace, including dependency
reexports, renamed aliases, public inherent methods and tuple constructors.
Private modules do not become public merely because their functions use `pub`.
Roots retain their callable Rust path; aliases may share one MIR instance.
Explicit and derived trait implementations on public ADTs also contribute
monomorphic roots, named with Rust UFCS, such as
`<facade::Point as core::clone::Clone>::clone`. Rust resolves default methods
and checks their predicates before they become roots. Destructors are compiler
managed and remain reachable through real drop glue, not callable roots.

`public_api` records each discovered function path, definition, kind, selection
and status. Each successful export also writes an adjacent `.api.json` file
containing the compiler, target, roots and inventory from the same export, so
coverage tools need not load the full MIR graph.
Each root also records its original Rust `params` and `return` type IDs;
compiler `spread_arg` tuples follow the same expansion as the generated entry
point. These IDs preserve ownership distinctions even when multiple Rust types
share one Go ABI representation.

`public_drop_types` records the sized, owned types in the API type closure for which
rustc reports `needs_drop`, together with their real `drop_in_place` instance.
The exporter enqueues that glue and writes the same list in the API sidecar.
References and raw pointers have no owned destructor. Their pointee can have a
separate descriptor and drop instance, but dropping a reference never drops its
pointee. Go callers release owned values through the descriptor's real Rust drop
operation; closing a runtime context does not implicitly drop Rust heap values.
`process_exit_symbol` binds the actual std `process_exit` diagnostic item when
std is linked. This preserves Rust's normal runtime cleanup and output flushing
when a Go executable hosts the library; no-std exports leave it empty.

`public_types` maps public Rust type paths to compiler type IDs. `api_types`
contains the operable closure of root signatures, concrete public types, public
fields, enum payloads and container elements. Each descriptor has a real Rust
name and a `canonical` ID computed by rustc region erasure. Distinct Rust types
are never equated just because their Go ABI representations happen to match.
`members` records each variant's field name, type, visibility and actual byte
offset; `variants_info` records inhabitance and non-exhaustive restrictions.
Private fields remain inaccessible. A private DST tail's descriptor is retained
because it determines the enclosing value's dynamic size and alignment.

String, Box and Global allocator identities come from lang items; Vec, Result,
Option, Path and OsStr identities come from compiler diagnostic items. Container
data/length/capacity offsets are computed through the pinned standard library's
named fields and rustc target layouts, then checked against scalar ABI shapes.
ZST Vec capacity and non-null empty-buffer alignment are explicit metadata.
Unsized str/slice/Path/OsStr views are distinct from stored fat references;
pointer descriptors retain reference/raw-pointer kind and mutability.

`default_symbol` and `display_symbol` bind actual Rust Default and ToString
implementations after predicate checking. Debug binds the pinned Rust
`Argument::new_debug`, `Arguments::new` and `alloc::fmt::format` instances, with
compiler-provided intermediate layouts. The template encoding is checked against
a native `format_args!` expansion; Go does not implement Rust's formatting rules.
These operations and destructors are generated only for the API type closure,
not every internal type in a large dependency graph. The type metadata test also
checks real initialized String/Vec storage and struct offsets against native Rust.
Map descriptors identify HashMap and BTreeMap through diagnostic items and bind
their concrete Rust `iter`/`iter_mut` and `Iterator::next` instances. Their iterator
state and actual `Option` item result join the API closure; Go need not know a
map's private table/node representation. JSON operations similarly bind an
already linked serde_json crate's real serializer/deserializer instances after
checking each type's predicates. `value_symbol` binds `to_value::<&T>` separately
from string serialization: it borrows T and returns Rust's real JSON Value,
preserving that operation's object ordering and floating-point conversion.
Generic type/const parameters require concrete Rust instantiations;
an unbounded set of blanket-trait implementations is not enumerated. The finite
trait inventory covers explicit/derived implementations in a public type's
defining crate and the facade crate. Generic methods on concrete type aliases
that still need impl-argument inference are reported as requiring
monomorphization. Recursive module reexports are recorded at the cycle instead
of inventing infinitely many alias paths. `OXIDE_ROOTS` selects exact paths
(commas inside UFCS type arguments are preserved), and rejects unknown or
non-instantiable selections. It can also explicitly select a private local
function for a compiler probe. The multi-crate checks verify these boundaries
and compile an independent downstream Rust consumer of the exported paths.

The checks export the same fixture for Linux arm64 and amd64. They verify
higher-ranked function pointer signatures, `dyn Write` arguments, closure
vtables, struct DST field offsets, generic function reification with a trait
object argument, compiler drop/caller metadata, and external call arity.
Exported signatures describe MIR parameters; rustc's hidden caller-location
ABI argument is recorded by `track_caller`, not added to the parameter list.
`call_locations` records whether each call inherits that location or uses a
compiler-generated static allocation, including MIR inlining scopes.
Function pointers retain their logical `fn_inputs`. The mandatory
`fn_spread_arg` is `-1` for an ordinary ABI, or the final tuple parameter index
for rustc's RustCall ABI. Callback declarations expand only that marked tuple;
an ordinary tuple parameter remains one argument. Missing ABI metadata is an
error, since the lowered ABI name alone cannot distinguish RustCall from Rust.
`runtime_checks` records rustc's actual UB, contract, and overflow check settings.
Late-bound signature regions are erased by rustc before querying argument layouts; the exporter
retains the full runtime signature and never guesses a layout from a lifetime.

Switch branch values are decimal strings to preserve all 128 bits. Function
symbols are unique; equivalent external declarations share one entry, and
conflicting signatures are rejected. TypeId provenance uses a zero relocation
base, following the pinned rustc LLVM backend: the constant bytes already hold
the hash. The checks compare these bytes with native rustc output.

Foreign static allocations include their linker symbol, import linkage,
declared type, and whether a weak import is a function pointer slot. A weak
import is not automatically considered absent: the backend must resolve the
symbol or explicitly provide the target's missing-symbol behavior. External
function and function-pointer signatures include their calling convention,
variadic flag, and fixed argument count. Function-pointer reification uses
rustc's resolver, including the shim for `#[track_caller]` functions.
`call_untuple[bb]` records the final tuple argument's index when the call's
actual Rust ABI is `RustCall`. Callee bodies retain rustc's `spread_arg` for
reconstructing a tuple local at entry; ordinary tuple arguments are not
flattened based on their shape. Function-pointer types separately export
`fn_spread_arg`: `-1` for ordinary signatures, or the final tuple index for
RustCall. This comes from the compiler signature before the backend calling
convention normalizes RustCall to Rust. Static Go callback aliases consume this
required metadata; regenerate older exports rather than guessing a tuple ABI.
`value_abi` records rustc's scalar, scalar-pair, vector, or aggregate backend
representation. `abi_scalar` records the actual primitive for scalar layouts;
`abi_pair` records both primitives and the second component's byte offset.
This is necessary for ABI-compatible function-pointer casts involving pointer
newtypes such as `NonNull`, even though their MIR types remain aggregates.

`panic_strategy` records the selected compiler panic strategy. `assert_calls`
records each MIR assertion's actual panic lang item, visible arguments, and
whether overflow checks control it. Its location is in `call_locations`.
The compiler supplies the panic message through the linked lang-item body.
`can_unwind` and `fn_can_unwind` distinguish C and C-unwind signatures that
otherwise have the same machine calling convention.

During export, the selected Cargo panic-runtime dependency is forced into
rustc's crate graph. Rust internal symbols are resolved through the compiler's
exported symbol table, and `panic_impl` through the actual language item.
This preserves Rust panic hooks, payload boxing, and cleanup as ordinary MIR.
The panic regression check rebuilds std and its panic runtime, then verifies
both Linux targets. See [UNWIND.md](UNWIND.md) for the runtime boundary.
`runtime_boundary: "std_args"` identifies the pinned Unix std argument getter
whose ELF initialization is supplied by the Go executable. Its body is retained;
the tag requires the actual std crate's compiler diagnostic item, not a matching
user-defined module or function name.
