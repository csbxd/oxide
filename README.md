# Oxide

中文入门：[使用教程：以 Mermaid 渲染器的 Rust → Go 翻译为例](docs/mermaid-tutorial.zh-CN.md)。

Oxide translates Rust into Go for Linux amd64 and arm64. The implementation
uses Cargo and a pinned rustc frontend for language semantics and target
layouts; the backend and runtime live in `oxide-go`. The compiler is still
in development. **mermaid-rs-renderer passes all 72 acceptance inputs across
23 diagram types on native linux/arm64 and linux/amd64**, with SVG and PNG
byte-for-byte equal to native Rust on the same target. Eight language/library
suites also pass 4,011 native comparisons per target, including large-value
storage and panic cleanup. See
[MILESTONES.md](MILESTONES.md) for tested support and remaining work.

Allocation checks use raw `MemStats.Mallocs` and `TotalAlloc` deltas. All 4,011
inputs pass on each native target with zero increments over 100 calls each.
The original three renderer inputs also pass three fresh exact-counter
processes (18 calls each) and 18 benchmark measurements per target, all with
zero raw Go allocations and bytes. These measure warmed calls; Rust heap
allocations still occur.

```text
Cargo + rustc (cfg, macros, traits, monomorphization, drop elaboration)
    -> oxide-rs (MIR, layouts, vtables, static relocations)
    -> oxide-go (Go source + static allocation image)
    -> oxide-go/runtime (Rust storage and target operations)
```

The compiler/runtime relationship follows ccgo/libc: generated code has an
explicit runtime contract. There is no AST translation fallback or compatibility
layer. Unsupported constructs need a correct lowering or a compilation error.
The renderer acceptance target must use translated Rust, including dependencies;
handwritten Go replacements for renderer functionality are excluded.

## Build and use

Requirements: Linux amd64/arm64, Go 1.27.1, Python 3.11+ with safe tar extraction
support, and a linker usable by rustc. Bootstrap downloads the pinned compiler,
rustc-dev, rust-src and both target standard libraries with checksum verification.

```sh
./oxide-rs/build.sh
cd oxide-go
go build -o ../bin/oxide ./cmd/oxide
../bin/oxide translate -manifest /absolute/path/Cargo.toml \
  -target linux/arm64 -roots crate_name::function \
  -out /absolute/path/generated
```

Generated packages import `github.com/csbxd/oxide/oxide-go/runtime`; add this
module to the consuming project's Go module (a local `replace` is useful during
development). Outputs are numbered Go source files (`oxide_gen_00000.go`,
`oxide_gen_00001.go`, ...), `oxide.mir.json` and, when needed, `oxide_alloc.bin`.
Keep all generated Go files and the allocation image together for `go:embed`.
Both `translate` and `emit` use the same writer and preserve declaration/init
order across files. Each output has a target-specific Go build constraint; run
translation separately for each target.

`-overflow-checks=false` selects Rust's unchecked arithmetic profile. The default
is checked. Cargo dependencies are cached, while the selected crate is exported
afresh even if source files have not changed. Failed compiler or generator runs
leave the previous published output untouched.

To reproduce the renderer acceptance suite, from `oxide`:

```sh
python3 fixtures/renderer/test.py
# Retry only Go generation and comparison using cached MIR and references:
python3 fixtures/renderer/test.py --stage go
```

The [Rust fixture](fixtures/renderer/lib.rs) depends on the unchanged upstream
renderer with default features enabled. Its two roots accept source bytes,
render SVG into a caller buffer and write PNG using the original renderer. The
[Go harness](fixtures/renderer/generated_test.go) only invokes those roots and
compares every output byte. Inputs include the original three examples and all
69 unchanged upstream fixture files, with their [provenance and license](fixtures/renderer/cases/upstream/README.md)
preserved. Native Rust successfully renders all 72 cases twice, producing 144
deterministic SVG/PNG reference files under `.cache/renderer-conformance/reference/`.
Cached MIR, generated Go and actual output stay in that cache for diagnosis.

The two-root graph contains about 100,000 functions. Generation, compilation,
default test-time vet checks and all 144 SVG/PNG byte comparisons pass on both
native targets with the current layout, large-value ABI and allocator code.
Dynamic field projections include runtime tail alignment, nested tails and
packed alignment caps, independently covered by 42 native differential cases.
The final packages use Context storage for large Rust values and have passed
the complete renderer corpus again.

Ordinary f32/f64 MIR operations preserve individual rounding, preventing
unintended Go floating-point fusion;
native differential regressions cover this renderer-relevant behavior. These
fixture results do not establish complete Rust or upstream API coverage.
The original three inputs have matching Go/native benchmarks. Go's median
elapsed times are 1.79–2.38 times Rust's on arm64 and 1.13–1.85 times Rust's on
amd64. These are small-sample dev-build comparisons; throughput still needs
work. Both measure zero raw Go allocations and bytes in the Go runs.
[Measurement conditions and timings](MILESTONES.md#renderer-checkpoint) are
recorded separately from byte-correctness acceptance.

`oxide emit -mir FILE -out DIR` reruns the Go backend on an existing MIR export.
`oxide audit -manifest Cargo.toml` uses rust-analyzer for a syntax inventory;
it does not determine semantic translation support.

## Storage and lifetime contract

Rust's compiler supplies size, alignment, field offsets, enum tags, niches and
value ABI classes. Scalar values and scalar pairs use matching Go primitive
representations; memory aggregates use byte storage. This keeps indirect calls
consistent when Rust ABI-compatible types, such as `NonNull<()>` and raw
pointers, have different source-level types. Rust pointers are integer addresses
into explicit storage, not pointers hidden inside Go heap objects. A per-call-chain
`runtime.Context` owns stable, aligned automatic storage outside the moving Go
stack. Address-taken, over-aligned and large locals use that storage; other
locals remain Go values. Generated calls restore their frame mark on return.
Drop order and cleanup paths come from rustc's elaborated MIR.

This preserves the tested address, alignment and drop behavior without an extra
Go heap allocation per local. It is an emulated Rust stack, not a claim of
identical physical native stack placement. Context creation and segment growth
have setup costs; existing segments are reused. `StorageDead` slot reuse, precise
peak stack usage and comprehensive unwind behavior remain open.
Values larger than 16 KiB now use Context storage and indirect parameters and
results. Callees copy value parameters into their own frames, preserving Rust
copy/move behavior and avoiding large Go temporaries. A 256 KiB array regression
reduced direct/move/return allocation counts from 1/2/3 to raw zero on both
targets. The 96-case suite and eight large-value panic cases pass native result
comparisons and exact allocation checks on both targets.

Compiler assertion calls, panic strategy, unwind boundaries and caller locations
come from metadata. Real Rust panic/panic_unwind bodies pass the panic fixture,
including payload downcasts, custom hooks, overflow catches and dynamic drops;
double panics and cross-thread panic behavior are not covered. `Context.Close`
runs registered C++ TLS destructors in reverse order and pthread-key destructor
passes; full native thread-exit semantics remain unverified. A Context must be
closed and must not be used concurrently.

Zero-sized values require no access to their Rust address. Their value reads
and writes preserve evaluation without dereferencing valid Rust dangling
addresses such as an empty `Box` pointer; address-taking still keeps the Rust
address. The panic fixture exercises a zero-sized boxed hook closure.

Generated roots take an explicit `*runtime.Context`; roots marked `#[track_caller]`
also take a trailing Rust caller-location address. Go callers must provide valid
Rust storage for pointer arguments. There is no automatic Go string/slice ABI
conversion. Generated functions use Go calling conventions; matching Rust
memory layouts does not make their entry points native Rust ABI compatible.
RustCall tuple adaptation uses compiler metadata, including closure callsites.
Large arguments are storage addresses; large results use a caller-provided
output address immediately after Context instead of a Go aggregate return.

The [Rust allocator](oxide-go/runtime/allocator_linux.go) uses an
[internal copy of modernc.org/memory v1.11.0](oxide-go/internal/memory/README.md)
with its allocation algorithm, upstream tests and BSD notices retained. Page
registration uses links in off-heap headers instead of a Go map. It retains
at most one freed block in each power-of-two storage class up to 1 MiB; excess
and larger freed blocks return to the backing allocator. With no live user
allocations, filling every cache class retains
2.625 MiB of mapped slabs in the retention test, below its 4 MiB bound. This
excludes live Rust allocations, Context segments and the static image. Alignment,
zeroing, realloc contents and failed-realloc ownership are checked separately.
The off-heap registry removes the profiled Go map-growth allocations. Cold
allocator tests, three fresh renderer exact-counter processes and the full
renderer corpus pass on both native targets. The copy serves Oxide-owned Rust/C
allocations; `modernc.org/libc` retains its own upstream dependency. Zero Go
allocations does not mean zero Rust heap allocations or zero syscalls.

The static allocation image uses one aligned arena. In the initial renderer
image, 75,300 per-object mappings become one mapping, occupying 2,392,064
page-rounded bytes (about 2.392 MB) on this host with 16 KiB pages. This measures
that static image alone, not total process memory or Rust heap usage. The tested
C boundary uses `modernc.org/libc` and runtime wrappers for actual OS calls,
weak function slots, C allocation and errno.

The amd64 backend includes the required SSE/AVX integer operations and SSE
floating-point helpers. CPUID and XGETBV query the actual CPU; Rust's feature
checks remain in the translated program. Native x86 floating instructions retain
their target-specific rounding and NaN behavior. Inline assembly accepts the
validated stdarch CPUID template and comment-only identity barriers; unknown
templates fail compilation. These helpers pass native runtime tests on an
amd64 host, including actual SSE operations, CPUID and XGETBV. The renderer also
passes its separate amd64 acceptance gate.
Volatile memory accesses remain unsupported.

Static ELF module enumeration supports `CGO_ENABLED=0` processes, including the
main executable and vDSO. Dynamically linked `PT_INTERP` images are explicitly
rejected. Backtraces expose real Go code and stack addresses and follow Go stack
movement; mapping those frames to original Rust symbols/lines remains open.

## Verification

```sh
cd oxide-go
go test ./...
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRust' -timeout 30m -v
cd ..
python3 oxide-rs/tests/check.py
```

The default Go suite compiles stored compiler MIR for both target architectures,
executes the host target against Rust reference results, and measures Go
allocations. The opt-in Rust suite rebuilds the fixture with the pinned frontend
and compares native Rust with newly generated Go. At this checkpoint:

| Fixture | Native Rust comparisons per target | Evidence |
| --- | ---: | --- |
| [Core/layout](oxide-go/internal/mir/testdata/core.rs) | 204 | native arm64 and amd64; raw zero-allocation check passes |
| [Numeric](oxide-go/internal/mir/testdata/numeric.rs) | 3,597 | native arm64 and amd64; 128-bit arithmetic, float casts and separate-operation rounding, carrying multiplication and TypeId equality |
| [Vec/Box/String](oxide-go/internal/mir/testdata/alloc.rs) | 18 | native arm64 and amd64; raw zero-allocation check passes |
| [Libc](fixtures/libc/lib.rs) | 18 | native arm64 and amd64; weak getrandom/gettid/statx, variadic open, file I/O, errno and posix_memalign/free |
| [Panic/drop/format](fixtures/panic/lib.rs) | 28 | native arm64 and amd64; hooks, catch/downcast, dynamic destructors and mixed string/integer formatting |
| [Dynamic layouts](oxide-go/internal/mir/testdata/dst.rs) | 42 | native arm64 and amd64; aligned Arc trait objects, nested DSTs, packed slice/trait fields and drops |
| [Large values](oxide-go/internal/mir/testdata/large.rs) | 96 | native arm64 and amd64; by-value copies, return slots, function pointers, virtual calls, closures, constants and repeats |
| [Large panic cleanup](fixtures/large-panic/lib.rs) | 8 | native arm64 and amd64; preserved caller output, payloads and exactly-once large-argument drops |

All eight suites check native results, panic state and restored automatic-storage
frame marks on every measured call. The allocation gate requires exactly zero
raw allocation/byte increments over 100 calls, without averaging; every input
passes on both native targets. References are built and executed with pinned
Rust on each target, without substituting arm64 results for amd64.

Core/numeric/large rebuild `core` with a fixture panic handler and test successful
paths; the other five suites rebuild full `std,panic_unwind`. The panic fixtures
exercise Rust-owned payloads, hooks, downcasts and cleanup, including null vtable
destructor slots and exactly-once drops. Go only invokes the roots and compares
results. The renderer has a separate byte-equivalence and exact-allocation gate.

The [compiler metadata tests](oxide-rs/tests/check.py) cover both targets: HRTB
signatures, trait vtables, struct DSTs, true function reification, RustCall tuple
adaptation, compiler assertion calls, caller locations, scalar/scalar-pair value
ABI metadata, weak external linkage and C variadic signatures. The separately
gated 72-case renderer check is:

```sh
cd oxide-go
OXIDE_RENDERER_TESTS=1 go test ./internal/mir \
  -run '^TestRendererConformance$' -timeout 60m -v
```

See [MILESTONES.md](MILESTONES.md) for standard-library scope, source links and
snapshot refresh commands.

## Ownership and leak chaos checks

Seeded chaos tests check live owners and actual mapping lifetime, independently
of the Go allocation benchmarks. `memory.counters` enables `runtime.HeapStats`:
live Rust/C allocations are backing allocations minus the explicitly retained
cache. A retained frame segment before `Context.Close`, an allocator cache page,
or process-lifetime library state is not itself a leak.

Context/allocator/static-loader checks and translated Rust ownership chaos pass
on native arm64 and amd64; arm64 race runs also pass. The final renderer test
passes **240 randomized calls per target** across two independent processes,
with native output equality and all three ownership ledgers fixed at baseline.

From `oxide-go`:

```sh
go test -tags=memory.counters ./runtime -run 'Chaos|HeapLeakOracle|HeapMappingFailure' -count=1 -v
go test -race -tags=memory.counters ./runtime -run 'Chaos|HeapLeakOracle|HeapMappingFailure' -count=1 -v
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRustOwnershipChaos$' -timeout 30m -v
```

The Context test runs in an isolated process, checks every owned mapping page
with `mincore` after closure, and verifies Rust/libc live-allocation baselines,
TLS isolation, key reuse, destructor order and nested unwind restoration. Its
negative controls deliberately omit Close/free, require detection, then clean
up. Replay it with `OXIDE_CHAOS_SEED=0x12345`; add `OXIDE_CHAOS_LONG=1` for 128
epochs. The Rust ownership fixture performs its allocations, moves, drops and
panic payload cleanup in Rust; `OXIDE_CHAOS_EPOCHS` controls repeated epochs.

Renderer chaos uses three independent ledgers: live Rust owners and bounded
allocator storage, complete libc allocator counters, and file-backed mapping
bytes grouped by device/inode. This includes font mappings outside the Rust
allocator. Context-owned anonymous mappings are checked separately with
`mincore`; unrelated anonymous Go heap mappings are not counted as font leaks.
A fixed nine-Context warm-up covers the pinned regex cache’s initial owner and
eight shards; native Rust and allocation traces confirm that initialization.
After it, Rust live owners stay at 3,271 on arm64 and 3,275 on amd64, and libc
and file ledgers remain unchanged. The baseline never adapts to later growth.

From `oxide`, using retained MIR/references:

```sh
python3 fixtures/renderer/test.py --stage go --chaos
OXIDE_CHAOS_SEED=0x12345 OXIDE_CHAOS_EPOCHS=16 \
  python3 fixtures/renderer/test.py --stage go --chaos
```

For diagnosis, `memory.counters oxide.heaptrace` enables `StartHeapTrace`,
`StopHeapTrace` and `HeapTraceSnapshot` for outstanding Rust allocations and
their return PCs within an explicit window. Tracing allocates Go memory and
must not be used for zero-allocation performance claims; production hooks are no-ops.

`Context.Close` releases its frame/TLS storage and runs registered destructors.
It does not take ownership of ordinary Rust heap objects or consume an
uncaught Rust exception: those require the owning Rust cleanup path, including
`TakeException` where appropriate. These bounded tests do not prove absence of
all leaks for arbitrary Rust programs; see the ledger for measured coverage.
