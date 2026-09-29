# Oxide

[中文使用教程](docs/mermaid-tutorial.zh-CN.md)

Oxide translates Rust libraries and their dependencies into Go for Linux amd64
and arm64. Cargo and a pinned rustc provide type checking, monomorphized MIR,
target layouts and drop glue. The translator and runtime live in `oxide-go`;
`oxide-rs` supplies compiler metadata. The runtime contract follows ccgo/libc.

Go calls the original library directly. No project-specific Rust wrapper is
required. Generated APIs expose compiler-described Rust values, named fields,
enums and explicit destruction. Unsupported constructs require a correct
lowering or an explicit diagnostic. The renderer acceptance target cannot use
handwritten Go implementations of Rust functionality.

The static API passes both native targets: 85 small differential cases, 17
compile-time rejection cases, 72 renderer inputs / 144 SVG and PNG files,
74 API probes, 20 CLI cases and ownership checks. Warmed measurement windows
report zero Go objects and bytes, with Rust/libc/file-mapping baselines restored.
[MILESTONES.md](MILESTONES.md) records the tested subsets and remaining limits.

## Build and translate

Requirements: Linux amd64/arm64, Go 1.27.1, Python with safe tar extraction, and
a linker usable by rustc. Bootstrap installs the pinned compiler, rustc-dev,
rust-src and both target libraries with checksum verification.

```sh
./oxide-rs/build.sh
(cd oxide-go && go build -o ../bin/oxide ./cmd/oxide)
bin/oxide translate \
  -manifest ../mermaid-rs-renderer/Cargo.toml \
  -features scene -target linux/arm64 -out .cache/renderer-go
```

Default Cargo features remain enabled. `-no-default-features` disables them.
An omitted `-roots` exports public free functions, re-exports, inherent methods,
callable constructors and monomorphic trait methods, including provided
defaults. `-roots crate_name::function` selects an explicit subset. Generic
declarations remain in the inventory until Rust supplies a concrete
instantiation. Invalid by-value unsized signatures cannot become callable roots.

Output includes numbered Go files, `oxide.mir.json`, a small
`oxide.mir.api.json` inventory and, when needed, `oxide_alloc.bin`.
Keep every generated Go file and allocation image together. Generated packages
import `github.com/csbxd/oxide/oxide-go/runtime`; add the module or a local
`replace` to the consuming Go project. Translate each target separately.

Public paths determine names: `crate::render` becomes `Render`,
`crate::layout::render` becomes `Layout_Render`, and
`crate::RenderOptions::modern` becomes `RenderOptions_Modern`. Trait methods
encode their UFCS paths. Name collisions fail explicitly.

`oxide emit -mir FILE -out DIR` regenerates Go from an existing export.
`oxide audit -manifest Cargo.toml` uses rust-analyzer for a syntax inventory;
it does not establish translation support. Checked integer arithmetic is the
default; `-overflow-checks=false` selects the unchecked Rust profile.

## Calling Rust from Go

Primitive parameters/results use Go scalar types. Each Rust type also gets a
nominal `Rust__Name` layout type with compiler-derived size and alignment
constants. Its static API separates `Ref__Name` (shared borrow), `Mut__Name`
(mutable place), and `Value__Name` (owner in stable Rust storage). Fields and
methods have concrete Go signatures; there are no runtime type descriptors,
string field lookups or callback tables. Genuine Rust aliases share Go identity;
unrelated Rust types with the same layout do not. Zero-sized private markers
also prevent explicit Go conversions between different types or borrow/owner
roles. Thin handles remain 8 bytes and unsized views 16 bytes.

For example, with the generated renderer imported as `renderer`:

```go
ctx := oxide.NewContext()
defer ctx.Close()
mark := ctx.Mark()
defer ctx.Restore(mark)

source := renderer.Borrow__Str(ctx.CopyString("flowchart LR; A --> B"))
result := renderer.Render(ctx, source)
defer result.Drop(ctx)
view := result.Ref()
if view.Variant() == renderer.Variant__Core_Result_Result__Of__Alloc_String_String__And__Anyhow_Error__End__Err {
    message := view.Field__Err__0().Display(ctx)
    defer message.Drop(ctx)
    panic(strings.Clone(message.Ref().String()))
}
svg := view.Field__Ok__0().Bytes() // Borrow valid until result.Drop.
_, err := output.Write(svg)
```

Public aliases use their shortest exported path, then lexical order. Compound
types encode their concrete arguments. The double-underscore namespaces keep
layout types, views, factories and variant constants distinct from root
functions. Private fields have no accessor. Missing trait implementations have
no method, so an unsupported call fails during Go compilation.

```go
options := renderer.Default__RenderOptions(ctx)
font := options.Mut().Field__Theme().Field__FontFamily()
font.Replace(ctx, renderer.String__Alloc_String_String(ctx, "sans-serif"))
options.Mut().Field__Layout().Field__NodeSpacing().Set(60)
result := renderer.RenderWithOptions(ctx, renderer.Borrow__Str(ctx.CopyString(source)), options)
// RenderWithOptions consumed options. Only result is still owned here.
result.Drop(ctx)
```

The generated operations include:

- `New__Name` for uninitialized storage and `Default__Name` for actual Rust
  defaults. Zero bits are not a general Rust default. Scalar places have typed
  `Get` / `Set` methods; `Field__Name` and `Index` return concrete borrowed places.
- `New__Enum__Variant` takes statically typed payloads and writes the compiler's
  direct tag or niche. `Variant` returns a distinct Go tag type; variant field
  access checks the active tag.
- `Init` and `Move` transfer ownership. `Replace` saves the incoming value,
  runs the previous Rust destructor, and installs the new bits even if that
  destructor panics. Independent zero-sized owners still receive their drops.
- `String__Name`, `Bytes__Name` and `Vec__Name` construct compiler-identified
  global-allocator containers. Vec capacity starts uninitialized: call `InitAt`
  and then `SetLen`. OS strings and paths preserve arbitrary Linux bytes.
- Shared and mutable `Iter` methods call real Rust map iterators. The iterator's
  `Mut().Next` returns its concrete Rust `Option<Item>`; borrowed tuple elements
  expose typed `Deref` methods.
- `Ref().Debug`, `Display`, `JSON` and `JSONValue`, plus `FromJSON__Name`, call
  the actual compiler-resolved Rust implementations. Formatting and JSON return
  owned Rust values. Deserialized borrows require the original input storage to
  remain valid. `JSONValue` uses Rust `to_value`, including its float conversion
  and object ordering.
- `Value.Drop` calls rustc's destructor directly at the object's actual address.
  Shared references have no `Drop`, scalar setter or owning conversion.

Copying an owning Go handle does not clone Rust ownership. By-value calls,
initialization, moves and destruction consume the ownership bits. Go checks
nominal types and method sets, but cannot enforce linear ownership, reference
lifetimes or all aliasing rules. `UnsafeRef__Name`, `UnsafeMut__Name` and
`UnsafeValue__Name` are explicit entry points for externally managed storage.
A zero-copy `Bytes()` or `Span().Bytes()` view obtained from a shared reference
must not be written through; Go slices cannot express a read-only byte borrow.
Raw pointer headers can be accessed through their stable address with explicit
unsafe operations; `SetRef` does not construct null references.
Packed fields can be read or moved through mutable places; creating a Rust
reference to an unaligned place is rejected. `Callback__Name` and `ABI__Name`
aliases describe exact translated function-pointer signatures for low-level Go
callbacks, without exposing numeric compiler type IDs.

## Manual storage and lifetimes

`Context` owns aligned automatic storage outside the moving Go stack.
An owned result retains exactly its result slot. Scalar and borrowed boundaries
need no extra frame or temporary fat-pointer allocation. Drop remaining owners
before restoring their frame. `Context.Close` releases automatic storage and
TLS state; it does not discover arbitrary Rust heap owners.

For a value that must outlive its Context frame, allocate its header separately:

```go
saved := renderer.Alloc__Theme()
mark := ctx.Mark()
saved.Init(renderer.Theme_Modern(ctx))
ctx.Restore(mark)
saved.Close(ctx) // Rust Drop, then free the header even if Drop panics.
```

`Storage__Name.Free` frees only the enclosing allocation. `Value__Name.Drop`
destroys the Rust contents; `Storage__Name.Close` does both. Borrowed fields
have no `Free` method. `Rust__Name` preserves Rust byte size and nominal identity;
Go's native alignment is at most 8. Views and allocators enforce the actual
`RustAlign__Name`, including alignment 16/64, without claiming that an ordinary
Go layout value has that alignment. Unsized types have metadata-bearing views
instead of a fabricated fixed-size Go layout.

The compiler supplies size, alignment, field offsets, niches, metadata and
internal value ABI classes. Rust type identity is separate from shared Go ABI
representations. Addressable, over-aligned and large locals use Context
storage; other locals can remain Go values. Internal values larger than 16 KiB
use copied indirect arguments/results, avoiding oversized Go temporaries.
Generated cleanup follows rustc's elaborated MIR.

This is an emulated Rust stack. Exact physical native stack placement,
StorageDead slot reuse and identical peak memory are not claimed. Context
creation/growth, static initialization and cold allocation have setup costs;
warmed tests require raw `MemStats.Mallocs` and `TotalAlloc` deltas of zero.
Rust heap allocations still occur.

The allocator retains the pinned modernc memory algorithm with an off-heap
page registry. The C/OS boundary uses modernc libc. Actual CPUID/XGETBV and
required SSE/AVX operations run on amd64. Unsupported assembly templates and
volatile accesses are rejected. Async, general native threading, broad network
APIs and exhaustive Rust provenance/unwind semantics remain unsupported or
unverified. Generated Go entry points use Go calling conventions, not native
Rust function ABI.

## Verify

```sh
(cd oxide-go && go test ./...)
(cd oxide-go && OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRust' -timeout 30m -v)
python3 oxide-rs/tests/check.py
python3 oxide-rs/tests/check_type_api.py
python3 oxide-rs/tests/check_json_api.py
python3 fixtures/renderer/test.py
python3 fixtures/upstream/run.py check
```

The [upstream Rust regression fixture](fixtures/upstream/README.md) integrates
the pinned `coretests` and `alloctests`, including internal and auxiliary test
targets. It reruns the host target's recorded successes and fails on any
regression. `scan` measures additional cases; `promote` reruns every recorded
success together with selected candidates, or the full inventory when no
selection is supplied, before adding successes. The Go test entry is enabled by
`OXIDE_RUST_TESTS=1`; direct `check` invocation is suitable for CI.

The Linux arm64 baseline contains 2,507 independently verified cases:
2,505 from `coretests` and both allocation-error auxiliary targets. All passed
native Rust and generated Go, followed by an independent `check` rerun.
The full 4,662-case inventory has been measured; main `alloctests` and its
internal target currently encounter shared support-code blockers.

Native references must execute on the same target as generated Go. Renderer
tests compare complete SVG/PNG bytes, API results, CLI process behavior and
ownership/accounting baselines. Native oracle programs are test data; they
are not translation inputs.

The full renderer graph needs substantial compile memory and disk space.
Its test script retains default vet and uses `-p=1`, `GOGC=25`,
`GOMEMLIMIT=20GiB` and `GOMAXPROCS=4` unless explicitly overridden. The Go
memory limit is a soft GC setting, not a runtime reservation. Place `TMPDIR`
and `GOTMPDIR` on a filesystem large enough for the build.
