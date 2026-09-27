# Rust unwinding boundary

All references below use the pinned compiler revision
`574ff7d98bd6d037e5236a8453029173b32631fd`.

## Exported code

The frontend links `panic_impl` to its actual language-item definition. For
Rust internal symbols it searches rustc's exported non-generic symbol table,
not function names or every metadata DefIndex. Proc-macro metadata is sparse
and is not a source of runtime definitions.

Cargo passes its rebuilt panic runtime as an unused dependency. The exporter
adds the rustc `force` extern modifier for the selected `panic_unwind` or
`panic_abort` dependency, retaining Cargo's exact metadata path. This matters
when compiling an rlib: normal panic-runtime injection can be skipped at that
stage. Dependencies compiled without MIR still cannot be translated.

The generated graph includes the Rust panic handler, hooks, panic counters,
`PanicPayload::take_box`, the owned `Box<dyn Any + Send>` payload, and the real
catch callbacks. It also includes the selected runtime's exception allocation
and cleanup. The backend must not replace these paths with a message string.

Relevant compiler sources:

- [rlib panic-runtime selection](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/compiler/rustc_metadata/src/creader.rs#L940)
- [forced extern dependency](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/compiler/rustc_session/src/config.rs#L980)
- [assertion panic calls](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/compiler/rustc_codegen_ssa/src/mir/block.rs#L802)

## Backend contract

`panic_strategy` is `Unwind`, `Abort`, or `ImmediateAbort`.
`Function.assert_calls[bb]` contains `symbol`, `args` (MIR operands), and
`optional`. An optional overflow assertion is disabled when
`runtime_checks.OverflowChecks` is false. Every assertion has a compiler
caller location in `call_locations[bb]`; the backend calls its target with
the visible arguments followed by the hidden location. The Rust target body
constructs the correct panic message or formatted payload.

On Linux arm64 and amd64, `panic_unwind::imp::Exception` has size 64,
alignment 16, and fields at offsets 0, 32, and 40. These are the native unwind
header, canary pointer, and owned 16-byte boxed trait object. The exporter
queries these layouts; the backend should consume them rather than duplicate
them.

The remaining native unwind boundary is `_Unwind_RaiseException`:

- Argument: pointer to `_Unwind_Exception`.
- Return: four-byte C `_Unwind_Reason_Code` enum.
- Calling convention: canonical C with `can_unwind=true` (C-unwind).
- A successful raise does not return normally. The translated runtime retains
  the real exception pointer and follows MIR unwind edges and cleanup blocks.

The `catch_unwind` intrinsic calls `try_fn(data)` and returns false on normal
completion. If that call unwinds, it passes the exception pointer to
`catch_fn(data, exception)` and returns true. The catch callback cannot unwind.
The actual Rust `do_catch` invokes `__rust_panic_cleanup`, takes ownership of
the boxed payload, decreases the panic count, and initializes the union's
error field. Nested catches and `resume_unwind` must preserve ownership.

Relevant library sources:

- [catch intrinsic](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/core/src/intrinsics/mod.rs#L2322)
- [catch union and callbacks](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/std/src/panicking.rs#L498)
- [panic hook and payload ownership](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/std/src/panicking.rs#L765)
- [exception allocation and cleanup](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/panic_unwind/src/gcc.rs#L57)

## Backtrace boundary

`_Unwind_Backtrace` must call the translated callback once per actual frame,
honor its reason code, and expose real instruction, entry, and frame addresses
through `_Unwind_GetIP`, `_Unwind_FindEnclosingFunction`, and `_Unwind_GetCFA`.
Go's public stack APIs provide PC and entry addresses but do not provide CFA;
returning zero is not a complete implementation.

`dl_iterate_phdr` must describe actual loaded ELF objects and valid program
headers. Rust's callback consumes the address bias, name, program-header
pointer, and header count. Main executable and vDSO information can be obtained
from Linux auxiliary vectors; dynamic libraries require the loader's object
list. An empty success result loses observable backtrace data.

- [libunwind backtrace adapter](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/backtrace/src/backtrace/libunwind.rs#L115)
- [ELF library callback](https://github.com/rust-lang/rust/blob/574ff7d98bd6d037e5236a8453029173b32631fd/library/backtrace/src/symbolize/gimli/libs_dl_iterate_phdr.rs#L61)
