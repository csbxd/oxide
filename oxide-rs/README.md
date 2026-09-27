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
python3 oxide/oxide-rs/tests/check_panic.py
```

The checks export the same fixture for Linux arm64 and amd64. They verify
higher-ranked function pointer signatures, `dyn Write` arguments, closure
vtables, struct DST field offsets, generic function reification with a trait
object argument, compiler drop/caller metadata, and external call arity.
Exported signatures describe MIR parameters; rustc's hidden caller-location
ABI argument is recorded by `track_caller`, not added to the parameter list.
`call_locations` records whether each call inherits that location or uses a
compiler-generated static allocation, including MIR inlining scopes.
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
flattened based on their shape.
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
