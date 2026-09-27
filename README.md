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

The generic type API passes native Rust comparisons on both targets, including
manual ownership, dynamic layouts, Rust formatting and serde operations with
exact warmed allocation checks. On both native targets, the direct renderer
graph passes 72 inputs / 144 SVG and PNG files, 74 API probes, 20 CLI cases and
ownership checks. Every API allocation window reports zero Go objects/bytes,
with Rust, libc and file-mapping ownership restored. The preceding façade
checkpoint is retained as historical evidence at `8ae41df`.
[MILESTONES.md](MILESTONES.md) separates current evidence from that checkpoint
and tracks support for each standard-library family.

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

Primitive parameters/results use Go scalar types. Rust aggregates use
`oxide.Value`, which points to stable Rust storage and its type descriptor.
A Rust `&T` parameter accepts a view of `T`. String/path references accept
`oxide.Span`; `Context.CopyString` and `CopyBytes` copy Go input into stable
storage. Typed slices accept array, slice or Vec views with the correct element
type.

For example, with the generated renderer imported as `renderer`:

```go
ctx := oxide.NewContext()
defer ctx.Close()
mark := ctx.Mark()
defer ctx.Restore(mark)

result := renderer.Render(ctx, ctx.CopyString("flowchart LR; A --> B"))
defer result.Drop(ctx)
if result.Variant() == "Err" {
    message := result.Field("0").Display(ctx)
    defer message.Drop(ctx)
    panic(message.StringCopy())
}
svg := result.Field("0").Bytes() // Borrow valid until result.Drop.
_, err := output.Write(svg)
```

Every public function has signature metadata, for example
`RenderTypes.Params` and `RenderTypes.Result`. Public named types have aliases
such as `TypeRenderOptions`; `RustType(name)` resolves compiler names and
public aliases. Callers do not embed type IDs or field offsets.

```go
options := renderer.TypeRenderOptions.Default(ctx)
font := options.Field("theme").Field("font_family")
font.Replace(ctx, font.Type.String(ctx, "sans-serif"))
options.Field("layout").Field("node_spacing").SetFloat(60)
result := renderer.RenderWithOptions(ctx, ctx.CopyString(source), options)
// RenderWithOptions consumed options. Only result is still owned here.
result.Drop(ctx)
```

The descriptors support:

- Named fields and enum variants, including compiler-provided direct and niche
  tags. `Type.Enum` consumes initialized field values.
- `Type.Uninit` for explicitly uninitialized storage, primitive setters,
  `Value.Init` for moves and `Replace` with Rust assignment behavior: snapshot
  the incoming value before dropping the old one, and install it even if that
  destructor panics. Zero-sized owners can share an address and still receive
  their own drops.
- Owned Rust `String` and global-allocator `Vec` construction using actual
  compiler header offsets. Vec starts at length zero: use `InitAt`, then
  publish the initialized elements with `SetLen`.
- Byte-preserving Linux `OsString` / `PathBuf` construction and borrowed
  `OsStr` / `Path`, including non-UTF-8 filenames.
- Actual Rust `Default`, `Display` and `Debug` implementations when the
  compiler resolves them for the concrete type. Formatting returns an owned
  Rust String which must also be dropped.
- HashMap/BTreeMap `Iterator` and `IteratorMut`, backed by the actual Rust
  iterator and `Iterator::next`. `Next` returns the real `Option<Item>`;
  map items contain borrowed key/value references accessed with `Deref`.
- `Value.JSON`, `JSONValue` and `Type.FromJSON` when the dependency graph contains
  `serde_json` and the concrete type implements the required trait. They call
  real serializers/deserializers and return owned Rust Results. `JSONValue`
  invokes `to_value(&value)`, preserving its numeric conversion and object
  ordering instead of going through an intermediate JSON string. Deserializing
  borrowed types can retain references to the input Span; preserve its frame.
- `Value.Drop(ctx)`, which calls compiler drop glue at the value's actual
  address. Missing required destructors fail explicitly.

`Value` is a view; copying it does not clone Rust ownership. By-value calls,
`Init`, `Enum` and `Drop` consume the transferred ownership bits. Do not
reuse or drop another copy afterwards. Field/index views borrow the owner.
Go cannot enforce Rust borrow rules, reference lifetimes or initialized values.
Zeroed bytes are not a general Rust default.

## Manual storage and lifetimes

`Context` owns aligned automatic storage outside the moving Go stack.
Public owned results retain their value slot; temporary reference headers are
restored before returning. Drop all remaining owners before restoring their
frame. `Context.Close` releases automatic storage and TLS state; it does not
discover and destroy arbitrary Rust heap owners.

For a value that must outlive its Context frame, allocate its header separately:

```go
saved := renderer.TypeTheme.HeapAlloc()
mark := ctx.Mark()
saved.Value.Init(renderer.Theme_Modern(ctx))
ctx.Restore(mark)
// saved still owns the Theme and its Rust allocations.
saved.Close(ctx) // Rust Drop, then free the header even if Drop panics.
```

`Storage.Free` frees only the allocation containing the value. `Value.Drop`
destroys its Rust contents. Use `Storage.Close` when both are needed; field
views deliberately have no Free method.

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
```

Native references must execute on the same target as generated Go. Renderer
tests compare complete SVG/PNG bytes, API results, CLI process behavior and
ownership/accounting baselines. Native oracle programs are test data; they
are not translation inputs.

The full renderer graph needs substantial compile memory and disk space.
Its test script retains default vet and uses `-p=1`, `GOGC=25`,
`GOMEMLIMIT=20GiB` and `GOMAXPROCS=4` unless explicitly overridden. The Go
memory limit is a soft GC setting, not a runtime reservation. Place `TMPDIR`
and `GOTMPDIR` on a filesystem large enough for the build.
