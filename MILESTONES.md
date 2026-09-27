# Oxide translation milestones

Snapshot: 2026-09-26. Targets: **linux/amd64 and linux/arm64**.

This is a support ledger, not a claim of full Rust or standard-library support.
“Verified subset” means the named cases have generated, compiled Go and agree
with native Rust on the recorded target. All eight language/library differential
suites and the complete 72-input renderer corpus execute on native arm64 and
amd64 hardware, including the current dynamic-layout and large-value ABI changes.
“Exported” means compiler metadata is available; it does not mean runnable Go.

Allocation acceptance checks raw `MemStats.Mallocs` and `TotalAlloc` deltas,
without averaging. All 4,011 fixture inputs pass 100 warmed calls per target;
three fresh renderer processes and 18 benchmark measurements per target also
report raw zero objects/bytes for the original flowchart/sequence/class inputs.
A single-allocation regression guards against
`AllocsPerRun`'s integer rounding. These are measured Go-heap results, not a
claim of zero Rust heap allocations or identical native physical stack usage.

The [Rust Reference](https://doc.rust-lang.org/reference/introduction.html) is the
semantic baseline. [Type layout](https://doc.rust-lang.org/reference/type-layout.html)
can depend on compiler and target, so rustc supplies actual sizes, alignments,
field offsets, tags and niches. Go representation guesses are not used as a
substitute. Compiler, interchange and runtime change together without a
compatibility fallback.

## Milestone state

| Milestone | State | Exit criterion |
| --- | --- | --- |
| M0: compiler spine | verified subset | Cargo → pinned rustc MIR/layouts → compiled Go; target selection and fresh export |
| M1: core language/layout | in progress | expand the differential suite beyond the current layout, pointer, arithmetic and drop cases |
| M2: ownership and standard library | in progress | alloc types, panic/unwind, TLS and collection behavior verified end to end |
| M3: dependency closure | full graph compiles, passes default vet and executes on arm64/amd64 | expand execution coverage for required OS/runtime boundaries |
| M4: mermaid-rs-renderer | 72-case acceptance passed on both native targets with current large-value ABI | all 144 SVG/PNG files per target match native Rust; broader upstream API coverage remains open |

## Compiler pipeline

| Area | Status | Evidence / remaining work |
| --- | --- | --- |
| Cargo discovery and package selection | implemented, driver tested | pinned Cargo; explicit selected package/library/bin |
| Repeated exports | verified | unique final rustc export argument forces a fresh root; changing roots in an unchanged crate produces new MIR |
| Compiler options | verified | checked/unchecked overflow flags; ambient encoded Rust flags and export roots cannot silently override the request |
| cfg, modules, macros, traits, generic instances | exported | Cargo/rustc handle these before lowering; no source AST translation fallback |
| HRTB fn signatures and dyn function reification | metadata verified | `oxide-rs/tests/check.py`, both targets; compiler-erased bound regions |
| Function symbol identity | implemented | duplicate extern declarations coalesce by symbol; inconsistent signatures fail |
| Rust ABI versus MIR signatures | verified subset | visible MIR parameters exclude the hidden caller location; compiler `call_untuple` and `spread_arg` drive RustCall tuple adaptation; generated functions use Go ABI, not the native Rust calling convention |
| Value ABI and indirect calls | verified subset | rustc `Scalar`/`ScalarPair` classes and component layouts determine Go representations; ABI-compatible pointer newtypes and raw pointers work through real Rust formatting callbacks |
| Assertion and unwind metadata | verified subset | compiler `assert_calls`, panic strategy, caller locations and `can_unwind` determine boundaries; real panic/panic_unwind bodies execute in 28 differential cases |
| External declarations and weak slots | verified subset | compiler symbol/linkage/function type, C variadic fixed argument counts; real getrandom/gettid/statx weak function slots pass the libc fixture |
| Static memory and relocations | verified subset | aligned arena for the allocation image, relocations and aliases; initial renderer image uses one mapping for 75,300 objects, with malformed layouts/relocations rejected |
| `TypeId` provenance | implemented | hash bytes from rustc; compiler TypeId relocations have address zero, matching rustc LLVM lowering |
| Target isolation | verified | generated Go has Linux/architecture build constraints |
| Generated source files | verified subset | translate/emit share a declaration-preserving writer; numbered Go files keep source/init order and avoid oversized source files |
| Syntax audit | inventory only | rust-analyzer node/dependency counts; it makes no semantic support claims |

## Language, layout and lifetime

| Feature | Status | Evidence / limits |
| --- | --- | --- |
| fixed-width integers, bool, char, usize/isize | verified subset | differential arithmetic and bit-pattern tests; not every intrinsic |
| i128/u128 | verified subset | casts, unary/bitwise ops, checked multiplication, masked shifts, comparisons, switches, signed/unsigned division/remainder and carrying multiplication; numeric fixture compares native results |
| floating point | verified subset | f32/f64 ↔ i128/u128, saturation, NaN/infinity, boundary rounding and integer-to-f32 double-rounding; ordinary scalar/SIMD operations explicitly round per MIR operation to prevent unintended Go fusion, with native f32/f64 counterexamples including signed zero; general IEEE/math conformance remains open |
| repr(Rust), repr(C), packed, aligned structs | verified subset | compiler offsets, packed unaligned access and actual 64-byte-aligned addressable locals |
| zero-sized values | verified subset | reads/stores do not touch Rust dangling addresses; address-taking is preserved; real zero-sized boxed hook closure succeeds |
| arrays and subslices | verified subset | element stride, pointer arithmetic, array pattern subslices and transmute use the projected array size |
| unions / transmute / raw pointers | partial | byte storage and typed projections; general alias/provenance model not exhaustively verified |
| enum tags and niches | verified subset | negative repr(i8), repr(i128), Option<NonZeroU64/U128>, signed/u128 switch and synthetic niche wrap across zero |
| slices / str / DST | verified DST subset | fat pointers and vtable metadata; projections and size/align recursively include runtime tail alignment and packed caps; 42 Arc/nested/slice/packed differential cases pass on both targets; broader DST/API coverage remains open |
| closures / dyn Trait | verified subset | RustCall tuple adaptation, compiler vtables/reification, real hook callbacks, dynamic methods and concrete/null vtable destructors; broader dispatch combinations remain open |
| deterministic Drop | verified subset | lexical drop order, exactly-once Box/dyn payload destruction, Vec trait-object element order and null vtable drop slots; exhaustive unwind paths pending |
| addressable automatic storage | verified subset | stable off-heap Context frames survive real Go stack movement and GC; warmed frame reuse is 0 Go alloc |
| large value storage and calls | verified subset on both targets | values larger than 16 KiB use Context storage, copied indirect parameters and caller-provided result slots; 96 success and eight unwind cases pass with zero warmed Go allocations |
| physical stack / storage reuse | incomplete | small scalar locals can stay in Go frames; addressable/large Rust storage is an emulated stack; StorageDead slot reuse and native peak-memory equivalence remain open |
| Rust heap allocator | verified subset | pinned modernc memory algorithm with off-heap page registry; alignment, zeroing, realloc grow/shrink/content and failure preservation; raw cold-allocation tests, updated fixtures and renderer exact counters pass on both targets |
| panic/unwind | verified subset | actual Rust-owned payloads, catch_unwind, u64 downcast, overflow catches, hook payload/location counts and dynamic drops match native Rust; double-panic, foreign exception and cross-thread behavior remain unverified |
| caller locations | verified subset | compiler callsite allocations, inherited and inlined scopes, direct track_caller line/column propagation; actual compiler reification shims are exported, with broader indirect-call execution still pending |
| async/coroutines | unsupported | state-machine/executor contract not implemented |
| SIMD | partial | compiler vector layouts and required lane operations lower; integer pack, multiply/add, shuffle, shift and carryless multiply helpers have unit tests; actual SSE compare/min/max/conversion/reciprocal helpers pass native amd64 tests; comprehensive SIMD semantics remain unverified |
| inline assembly | explicit templates only | complete stdarch CPUID template with checked operands/options invokes actual CPUID on amd64; fully parsed comment-only same-place identity in/out invokes a compiler barrier; unknown templates fail compilation |
| volatile memory accesses | unsupported | rejected explicitly |

## Standard library support

A row covers only the explicitly tested APIs; translating a generic body is
not evidence that an entire module is supported.

| Library/API family | Status | Tested subset / remaining boundary |
| --- | --- | --- |
| `core::mem` | verified subset | size_of, align_of, offset_of, equal-size transmute; size_of_val/align_of_val for aligned/nested/packed DSTs and packed ManuallyDrop tails; broader MaybeUninit/ManuallyDrop cases pending |
| `core::ptr` | verified subset | address-of, read/write, add, read_unaligned/write_unaligned and pointer/integer casts; volatile unsupported |
| `core::num` / primitive integer methods | verified subset | wrapping operations, rotations, nonzero niches, 128-bit division/remainder/casts, signed/unsigned carrying_mul_add at 8/16/32/64/128 bits and disjoint_bitor |
| `core::option` | verified subset | NonZeroU64/U128 representation and match; complete Option API not claimed |
| `core::result` | verified subset | catch results and dropping Err<Box<dyn Any + Send>> execute with native-equivalent payload destruction; broader Result API coverage pending |
| `core::array`, `slice`, `str` | partial | array and slice storage/projections; broad iterator/UTF-8 suites pending |
| `core::ops`, `marker`, `convert`, `clone`, `cmp` | partial | rustc resolves concrete instances; fixture-used operations only |
| `core::fmt`, `core::error`, `core::any` | verified subset | HRTB/dyn metadata, TypeId equality, mixed string/u64 formatting callbacks and dynamic payload downcasts; complete formatting/error APIs remain unverified |
| `core::cell`, `pin`, `borrow`, `iter` | unverified | translated when reachable, no complete API acceptance suite |
| `core::sync::atomic` | verified single-thread subset | AtomicUsize hook/drop counters and relaxed load/store/fetch operations; memory-order and multithreaded conformance pending |
| `core::arch` / CPU feature detection | verified runtime subset | actual CPUID/XGETBV and required SSE/AVX helpers pass on native amd64 hardware; Rust feature checks remain reachable, no hardcoded CPU capabilities; the complete translated renderer is checked separately |
| `alloc::alloc` | verified subset | allocator primitives and fresh Rust allocation call chains; transparent Alignment/NonNull ABI parameters |
| `alloc::boxed`, `vec`, `string` | verified subset | Box creation/drop, Vec growth/push/reverse, String creation/push_str/drop; 18 fresh native-Rust differential cases, zero extra Go allocations |
| `alloc::collections` | renderer-exercised subset | BTree and VecDeque paths participate in matching renderer outputs; dedicated collection/API and drop tests remain open |
| `alloc::rc` | unverified | no dedicated Rc/Weak ownership tests |
| `alloc::sync` | verified Arc subset | payloads with 32/64-byte alignment, clone/drop and nested DSTs pass the 42-case fixture on both targets; weak-reference and concurrency coverage remains open |
| `std::collections` | renderer-exercised subset | original Rust BTreeMap, HashMap/HashSet, BinaryHeap and VecDeque paths contribute to matching outputs; complete ordering/hash/API conformance remains open |
| `std::fmt`, `error`, `any`, `panic` | verified subset | 28 native differential cases cover custom hooks, panic_any, catch_unwind/downcast, overflow panic and mixed format! output; complete error/formatting APIs and double-panic paths remain open |
| `std::fs`, `io`, `path`, `env`, `ffi`, `os` | partial C boundary | real extern-C differential fixture verifies weak slots, variadic open, mkstemp/read/write/lseek/close/unlink, errno and posix_memalign/free; broad std API behavior remains unverified |
| `std::sync`, `thread`, TLS | partial runtime boundary | single-thread renderer Mutex/LazyLock paths execute; Context TLS isolation, C++ destructor LIFO/re-registration and up to four pthread-key destructor passes tested at Context.Close; complete thread exit, synchronization and native thread semantics remain open |
| `std::time` | implemented subset | monotonic clock helpers; broad layout and behavioral tests pending |
| `std::backtrace` / ELF loader boundary | partial | CGO-disabled static ELF main executable/vDSO enumeration; dynamic `PT_INTERP` explicitly rejected. Real Go PC/SP backtraces follow Go stack movement; Rust source symbol/line mapping is not implemented |
| `std::net`, `process` | unsupported | no complete OS boundary |
| `std::future`, `task`, async | unsupported | no executor/coroutine acceptance |
| re-exported core/alloc modules in std | same limits | no separate implementation or compatibility shim |

## Renderer checkpoint

The acceptance input is [fixtures/renderer](fixtures/renderer/Cargo.toml), a
Rust path dependency on the unchanged upstream `mermaid-rs-renderer` with
**default features enabled**. Both roots accept arbitrary source bytes:
`render_svg(source, length, output, capacity)` copies the complete SVG to a
caller buffer; `write_png(source, length, path, path_length)` invokes the original
PNG writer. The Go harness only supplies ABI storage and compares bytes; it
contains no renderer implementation or fixed-case dispatch.

The corpus contains **72 inputs across 23 diagram types**: the original three
examples and all 69 upstream `.mmd` fixtures, copied unchanged with directory
structure, [source commit and license](fixtures/renderer/cases/upstream/README.md)
preserved. Native Rust renders each input twice and confirms deterministic SVG
and PNG bytes. All 72 native cases pass on **both architectures**, producing
**144 reference files per target**.
Neither errors nor unequal output are skipped by the comparison gate.

The two-root export rebuilds `std,panic_unwind` and contains about 100,000
functions, with actual assertion calls, unwind paths and RustCall adaptation.
The shared writer emits numbered Go source files for the whole program.
The current graph passes generation, compilation, default vet and **all 72 cases
on both native targets**, with **144 SVG/PNG files per target byte-for-byte equal
to native Rust**. amd64 references were generated on the same machine and every
saved output was independently compared. The earlier amd64 failure in
`upstream/flowchart/aspect_chain` came from using a static 16-byte `ArcInner`
prefix for a trait payload requiring 32-byte alignment.

The general [field projection](oxide-go/internal/mir/generate.go) and
[dynamic layout](oxide-go/internal/mir/lower.go) code now rounds the prefix to
the tail's runtime alignment, propagates nested DST size/alignment, and caps
field alignment for packed parents. The new 42-case Rust fixture passes on
both native targets, checking aligned Arc payloads, clone/drop, nested trait
tails and packed slice/trait fields, with zero Go allocations in warmed calls.
With the final dynamic layout, large-value ABI and off-heap registry, full Go
comparisons took **168.20 seconds on arm64** and **335.699 seconds on amd64**.
These are acceptance durations, not comparable native rendering benchmarks.
The full corpus passes again on both targets with the final generated code.

Ordinary floating-point MIR operations preserve individual rounding;
native f32/f64 regression cases cover the unintended Go fusion found while
expanding the renderer suite.
Separately, 68 current runtime tests pass on native amd64 hardware, including actual
SSE operations, CPUID/XGETBV, integer helpers, allocator, backtrace and libc
tests with updated raw allocation checks. The 18 upstream/registry tests also pass on
that hardware; final logs are retained with the allocator revalidation artifacts.

The final warmed-call benchmarks use the off-heap registry. arm64 pins both
Go and native Rust to the same performance core, **CPU 2**; amd64 compares both
on the same remote hardware. Each entry is the median of three runs with three
timed calls per run (`-benchtime=3x -count=3` for Go). All 18 Go measurements per
target report **0 raw Go allocations and 0 raw Go bytes**
through `Go-allocs-total`/`Go-bytes-total`, without dividing by the call count.
The [native benchmark](fixtures/renderer/bench.rs) calls the same Rust ABI roots.

| Target | Format | Input | Native Rust ms/op | Translated Go ms/op | Go / Rust elapsed time |
| --- | --- | --- | ---: | ---: | ---: |
| arm64, CPU 2 | SVG | flowchart | 298.015 | 691.018 | 2.32× |
| arm64, CPU 2 | SVG | sequence | 1.968 | 4.086 | 2.08× |
| arm64, CPU 2 | SVG | class | 1.314 | 3.124 | 2.38× |
| arm64, CPU 2 | PNG | flowchart | 519.625 | 1,110.176 | 2.14× |
| arm64, CPU 2 | PNG | sequence | 223.124 | 439.637 | 1.97× |
| arm64, CPU 2 | PNG | class | 194.756 | 349.183 | 1.79× |
| amd64 | SVG | flowchart | 953.355 | 1,442.996 | 1.51× |
| amd64 | SVG | sequence | 4.982 | 9.215 | 1.85× |
| amd64 | SVG | class | 4.235 | 6.891 | 1.63× |
| amd64 | PNG | flowchart | 1,317.125 | 2,331.277 | 1.77× |
| amd64 | PNG | sequence | 711.357 | 804.408 | 1.13× |
| amd64 | PNG | class | 619.030 | 719.250 | 1.16× |

SVG timing includes rendering and the copy into the caller buffer. PNG timing
includes rendering, PNG encoding and file writes. Context/input setup and byte
comparisons are outside the timed loop; each timed call checks the restored
Go frame mark. Both harnesses verify full output before and after measurement.
Native Rust uses the pinned 2026-09-15 compiler, rebuilt `std,panic_unwind`,
`-Zalways-encode-mir -Zmir-opt-level=0 -Coverflow-checks=yes` and the fixture's
unoptimized dev profile. Go uses its normal compiler optimizations on code from
that MIR configuration. Native measurements were taken earlier on the same
hardware (and fixed CPU 2 for arm64); Rust source and build configuration did
not change. The amd64 native log is from 2026-09-26 11:04:34 UTC.
These are small-sample dev-configuration comparisons, not release-build results.
Go remains slower in these measurements despite zero measured Go allocations;
Rust heap allocations still occur. Fixed affinity follows observed hybrid-CPU
scheduling variance, and earlier unpinned/rounded measurements remain in the
cache rather than being substituted into this table.

Current arm64 evidence under `.cache/renderer-conformance/` is
`benchmark-registry-final.json`, `bench-registry-pinned.log` and
`native-bench-pinned.csv`. Final amd64 evidence is under
`.cache/renderer-amd64/remote/registry/logs/`, including `benchmark-result.json`,
`go-benchmark.log`, `native-benchmark.log` and `registry-acceptance-pipeline.log`.

A separate automatic-storage probe under `.cache/stack-probe/` exposed three
256 KiB array paths that allocated 1/2/3 Go objects per direct/move/return call.
Native assembly used the stack; Go diagnostics reported `too large for stack`.
The general [large-value ABI](oxide-go/internal/mir/large_value.go) now places
values larger than 16 KiB in Context storage and passes addresses, copying
parameters into callee frames and publishing results only on normal return.
The original probe now reports **zero** warmed Go allocations. Its before/after
logs remain cached; the formal 96-case success and eight-case unwind suites
pass on both native targets.

The initial renderer static image has 75,300 memory records. The
[arena loader](oxide-go/runtime/globals.go) replaces one mapping per record with
one aligned mapping. That image packs 2,384,128 bytes; with alignment and
this host's 16 KiB page size the mapping occupies **2,392,064 bytes (about
2.392 MB)**. This is static-image storage, not total RSS or Rust heap usage.
[Runtime tests](oxide-go/runtime/globals_test.go) verify many small records,
alignment, relocation, aliases and malformed-image rejection.

References, MIR, generated Go and actual outputs are retained under
`.cache/renderer-conformance/`, preserving case-relative paths. The
[test script](fixtures/renderer/test.py) compares complete SVG **and** PNG bytes.
The final build passes the complete corpus on both targets, including large-value
storage and calls. Fixture coverage does not establish full upstream
parser/layout/render API coverage. The syntax inventory is
[mermaid-rs-renderer.audit.md](mermaid-rs-renderer.audit.md).

The [Rust heap allocator](oxide-go/runtime/allocator_linux.go) uses the
[internal memory package](oxide-go/internal/memory/README.md), copied from
`modernc.org/memory v1.11.0` with upstream source/tests and
[BSD license](oxide-go/internal/memory/LICENSE) retained. Its allocation
algorithm is unchanged; the page registry is an intrusive list in mapped page
headers. The 64-bit header grows from 32 to 48 bytes, preserving 16-byte alignment.
This is scoped to Oxide-owned Rust/C allocations, not a module-wide replacement
of `modernc.org/libc` dependencies. Each storage class up to 1 MiB caches at
most one freed allocation; excess and larger blocks return to the backing
allocator. The
[retention test](oxide-go/runtime/allocator_linux_test.go) fills every class,
then frees all user allocations: counters measure **2.625 MiB** of retained
mapped slabs, below the **4 MiB** bound. This is separate from live Rust
allocations, Context segments and the static image. Zero Go allocations does
not mean zero Rust heap allocations.

The exact renderer allocation gate originally caught **one 2,304-byte object
and one 32-byte object** in a timed PNG call. Its profile attributes both to
`modernc.org/memory.Allocator.mmap` growing the mapping registry (`regs[p]`).
This explains why rounded per-call allocation counts were insufficient. The
upstream-based correction now moves registry links into off-heap headers.
The [registry tests](oxide-go/internal/memory/registry_test.go) check list
removal, closure, alignment and zero raw Go allocation/byte increments with
fresh allocators and no warm-up. Updated fixtures on both targets pass all
4,011 inputs with raw zero over 100 calls each; arm64 evidence and input/source
hashes are in `.cache/exact-conformance/summary-registry.json`. The first updated
renderer exact check passes all 18 calls, including the formerly failing PNG
call. Two additional fresh processes each pass another 18 exact calls; a rate-1
memory profile has no samples on the timed render stack or in the internal
allocator. Three fresh amd64 processes also pass all 18 exact calls each,
with matching bytes and restored frames. The final arm64 corpus passes all
72 byte comparisons; the final amd64 corpus and all raw-total benchmarks also
pass. Independent rereads compare every saved SVG/PNG byte; the amd64 evidence
collector also records SHA-256 manifests.

## Reproducible checks

From `oxide/oxide-go`:

The final `go test ./...` run passes with the current allocator and shared
exact-count helpers; the retained log is `.cache/final-go-tests.log`.

```sh
go test ./...
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRust' -timeout 30m -v
go test -race ./runtime
go test -gcflags=all=-d=checkptr=2 ./runtime
go test -tags=memory.counters ./runtime -run '^TestHeapIdleRetentionBound$' -v
go test ./runtime -run '^$' -bench '^BenchmarkRustAllocation$' -benchmem
GOOS=linux GOARCH=amd64 go test -c ./runtime -o /tmp/oxide-runtime-amd64.test
```

From `oxide`, `python3 oxide-rs/tests/check.py` verifies compiler metadata on
both targets, including HRTB signatures, RustCall adaptation, assertion calls,
caller locations, true function reification, weak linkage and C variadics.

| Differential fixture | Native comparisons per target | Generated amd64 | Generated arm64 | Raw Go allocation check (both targets) |
| --- | ---: | --- | --- | --- |
| [core](oxide-go/internal/mir/testdata/core.rs) | 204 | compiled and executed | compiled and executed | zero Go allocations and restored frame marks |
| [numeric](oxide-go/internal/mir/testdata/numeric.rs) | 3,597 | compiled and executed | compiled and executed | zero Go allocations and restored frame marks |
| [alloc](oxide-go/internal/mir/testdata/alloc.rs) | 18 | compiled and executed | compiled and executed | zero Go allocations and restored frame marks |
| [libc](fixtures/libc/lib.rs) | 18 | compiled and executed | compiled and executed | restored frame marks; zero Go allocations in the separate warmed-case recheck |
| [panic/drop/format](fixtures/panic/lib.rs) | 28 | compiled and executed | compiled and executed | restored frame marks; zero Go allocations in the separate warmed-case recheck |
| [DST](oxide-go/internal/mir/testdata/dst.rs) | 42 | compiled and executed | compiled and executed | zero Go allocations and restored frame marks; dynamic alignment/size and exactly-once Arc drops |
| [large](oxide-go/internal/mir/testdata/large.rs) | 96 | compiled and executed | compiled and executed | every warmed call checks native value, panic state and frame; zero Go allocations |
| [large panic](fixtures/large-panic/lib.rs) | 8 | compiled and executed | compiled and executed | preserved output on unwind, exactly-once argument Drop, correct payload/hook and zero warmed Go allocations |

The default MIR suite uses saved compiler exports and native core results.
Fresh core/numeric/large export rebuilds `core` with a real fixture panic handler;
these tests only exercise successful paths. Alloc/libc/panic/DST/large-panic rebuild full
`std,panic_unwind`. The numeric fixture combines boundary values, deterministic
random inputs and float-rounding counterexamples, including separate f32/f64
multiplication/addition and signed-zero results. Libc comparisons use validity
flags and fixed-content hashes, not random data, thread IDs or addresses.
The panic fixture checks native expected values before comparison: u64 payload
downcast to 42, overflow/no-overflow outcomes, hook payload/location counts,
exactly-once concrete drops, ordered Vec<Box<dyn Trait>> destruction with mixed
trivial elements, null destructor slots, and a byte hash of mixed string/u64
formatting. Rust code performs every panic and payload operation; the Go harness
only compares scalar results. Runtime arithmetic tests independently compare
128-bit results with `math/big`.
The runners are [conformance_test.go](oxide-go/internal/mir/conformance_test.go),
[numeric_test.go](oxide-go/internal/mir/numeric_test.go),
[libc_test.go](oxide-go/internal/mir/libc_test.go),
[panic_test.go](oxide-go/internal/mir/panic_test.go),
[dst_test.go](oxide-go/internal/mir/dst_test.go),
[large_test.go](oxide-go/internal/mir/large_test.go) and
[large_panic_test.go](oxide-go/internal/mir/large_panic_test.go). The alloc driver and full-std
fixtures retain Cargo artifacts under `.cache/` while forcing fresh MIR exports.

All eight generated harnesses share [allocations_test.go](oxide-go/internal/mir/testdata/allocations_test.go).
It sets `GOMAXPROCS(1)`, warms the call, records raw `Mallocs` and `TotalAlloc`,
runs 100 calls with value/panic/frame checks, and requires both counter deltas
to equal zero. Fixture I/O, formatting and Context construction remain outside
the window. A regression deliberately allocates one 32-byte object in 100 calls
and requires the helper to observe it, preventing a return to rounded averages.
All 4,011 inputs pass this exact check on each target. arm64 logs and
`summary-registry.json` are under `.cache/exact-conformance/`; amd64 logs and
`summary-registry.json` are under `.cache/strict-amd64/remote/`. Older allocation
logs are retained as history. No result implies zero Rust heap allocation or
covers all OS/panic APIs.

Each target builds and executes its own pinned native Rust references. The
amd64 checks do not reuse arm64 expected results. Runtime-only changes reuse
those unchanged references and validated target MIR, then regenerate/relink Go.
Successful numeric exports and their source hash are retained by the formal
gate in `.cache/numeric-conformance/` to support this recheck without stale MIR.

The separate DST suite has seven cases and six seeds, including Arc trait
payloads aligned to 32/64 bytes, nested unsized tail projections,
`size_of_val`/`align_of_val`, exactly-once Arc drops, and packed slice/trait
tails read through raw unaligned pointers. It avoids constructing invalid
references to packed fields. All 42 cases pass against native Rust on arm64 and
amd64 with frame restoration. Its allocation recheck passes raw zero on both.
Fresh exports and results are retained under `.cache/dst-conformance/`, including
`summary.json`, `gate-resumed.log` and `remote-results/go-test.log`.
The default [DST layout regression](oxide-go/internal/mir/dst_layout_test.go)
also compiles for both targets and executes on the host, checking inner, nested
and packed offsets/sizes for runtime alignments of 8, 32 and 64 bytes.
Run its acceptance gate from `oxide-go`:

```sh
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRustDSTConformance$' -timeout 30m -v
```

The large-value suite fixes its corpus at 12 cases and eight seeds: direct
indexing, by-value movement/return, callee mutation without caller mutation,
`x = f(x)`, function pointers, a transparent-wrapper function-pointer conversion,
virtual calls, struct returns with Drop, large constants, large repeated array
elements and FnOnce/RustCall adaptation. The full-std panic fixture adds two
cases and four seeds: panic before producing a large return must preserve the
caller buffer, and unwinding a consumed large parameter must drop it exactly
once. It also exercises a large closure environment and `Result` return.
Each of the 104 new inputs checks its native value, panic state and frame on
every one of 100 warmed calls and requires **0 Go allocations**. The completed
arm64 results and both target exports are in `.cache/large-conformance/` and
`.cache/large-panic-conformance/`. Native amd64 references and generated Go also
pass all 104 cases; the logs and summary are in `.cache/large-amd64/remote/`.
The eight suites therefore cover **4,011 native comparisons per target**.

```sh
OXIDE_RUST_TESTS=1 go test ./internal/mir \
  -run '^TestRustLarge(Value|Panic)Conformance$' -timeout 30m -v
```

To refresh core snapshots after changing the compiler interchange, from
`oxide/oxide-go`:

```sh
OXIDE_RUST_TESTS=1 OXIDE_UPDATE_FIXTURES=1 \
  go test ./internal/mir -run '^TestRustConformance$' -v
```

To reproduce the full renderer acceptance suite, from `oxide`:

```sh
python3 fixtures/renderer/test.py
# Retry Go generation/comparison against the retained MIR and native references:
python3 fixtures/renderer/test.py --stage go
# Rebuild native references or re-export the Rust graph separately:
python3 fixtures/renderer/test.py --stage native
python3 fixtures/renderer/test.py --stage export
# After generating the host-target package, measure the original three inputs:
cd .cache/renderer-conformance/go
CGO_ENABLED=0 GOWORK=off GOCACHE="$PWD/../go-cache" GOTMPDIR="$PWD/../go-tmp" \
  taskset -c 2 go test -mod=mod -run '^$' -bench '^BenchmarkRenderer(SVG|PNG)$' \
  -benchtime=3x -count=3 -benchmem -timeout 20m
```

To reproduce the native arm64 timing with the same Rust configuration, from
`oxide` after creating the native references:

```sh
OXIDE_BENCH_SYSROOT="$(bin/oxide-rs --print-sysroot)"
env -u CARGO_ENCODED_RUSTFLAGS \
  RUSTC="$OXIDE_BENCH_SYSROOT/bin/rustc" RUSTC_BOOTSTRAP=1 \
  RUSTC_WRAPPER= RUSTC_WORKSPACE_WRAPPER= \
  RUSTFLAGS='-Zalways-encode-mir -Zmir-opt-level=0 -Coverflow-checks=yes' \
  CARGO_TARGET_DIR="$PWD/.cache/renderer-conformance/cargo-target" \
  "$OXIDE_BENCH_SYSROOT/bin/cargo" build -Zbuild-std=std,panic_unwind --locked \
  --manifest-path fixtures/renderer/Cargo.toml --target aarch64-unknown-linux-gnu \
  --bin oxide-renderer-bench
taskset -c 2 .cache/renderer-conformance/cargo-target/aarch64-unknown-linux-gnu/debug/oxide-renderer-bench \
  fixtures/renderer/cases .cache/renderer-conformance/reference \
  .cache/renderer-conformance/native-bench-output
```

The equivalent Go gate, from `oxide/oxide-go`, is:

```sh
OXIDE_RENDERER_TESTS=1 go test ./internal/mir \
  -run '^TestRendererConformance$' -timeout 60m -v
```

Renderer acceptance is separately gated because it builds the complete
application dependency graph. Setting either gate runs the requested check;
a compiler or runtime failure is reported as a test failure.

Update this ledger in the same change as each newly verified library family.
Do not promote an exported module to “supported” based only on graph traversal.

## Ownership and leak chaos

These checks add live-owner and mapping-lifetime oracles to the existing native
result/allocation acceptance. They do not use RSS stability or rounded Go
allocation averages as evidence of leak freedom. `HeapStats`, available with
`memory.counters`, reports live backing allocations separately from cached
allocations/mappings. A cached mapping can also contain live objects, so its
bytes cannot be subtracted from total bytes to estimate live payload size.

| Area | Coverage and oracle | Current evidence |
| --- | --- | --- |
| [Context/TLS lifecycle](oxide-go/runtime/context_chaos_linux_test.go) | deterministic push/pop frames, 1 MiB alignment, cross-segment allocation, nested Go panic/state restoration, TLS isolation, Cxa LIFO/re-registration, pthread generation reuse and 1–4 destructor passes; mappings created during destructors included | native arm64/amd64 counters pass; arm64 default/race pass; 4 seeds × 8 epochs × 96 operations = 3,072 frame operations per target; extra 128-epoch seeds `0x12345` (arm64) and `0xbadcafe` (amd64) also pass |
| [Rust/C heap](oxide-go/runtime/allocator_chaos_linux_test.go) | alloc/realloc/free model, content and overlap checks, failure preservation, concurrent barriers, exact drained owner counts and cache accounting | native arm64/amd64 and arm64 race pass; 13,056 random steps, 18 drained epochs and 114 class-boundary cases; additional amd64 seed also passes |
| [Static-image failure rollback](oxide-go/runtime/globals_chaos_linux_test.go) | malformed aliases/relocations and missing symbols must reject the image and release its mapped arena; isolated mapping oracle | arm64/race pass; native amd64 seed `0x5eed1234` passes 512 malformed loads; the detected constructor-failure mapping leak is fixed |
| [Translated Rust ownership](fixtures/chaos/lib.rs) | actual Vec/String/Box/Arc, aligned trait objects, move/clone/resize/shrink/swap/drop and caught Rust panic payloads; native scalar results plus live-owner baseline | arm64 passes 192 calls / 36,960 operations and extended 768 calls / 147,840 operations; amd64 passes two 32-epoch seeds / 295,680 operations; live owners return to zero |
| [Renderer ownership](fixtures/renderer/chaos_test.go) | randomized SVG/PNG and fresh/reused Contexts; native bytes, frame restoration, fixed Rust-owner baseline, complete libc counters and file-backed mapping ledger | final native arm64/amd64 each pass two independent runs totaling 20 epochs / 240 random calls; Rust owners stay at each target’s baseline (3,271 / 3,275), with unchanged libc and file ledgers |

The Context oracle first detects deliberately omitted Close and large free via
`mincore`, then verifies cleanup. With counters enabled it also detects an
omitted small free even when cache retention leaves mapped bytes unchanged.
Every epoch closes both Contexts, confirms all fields and libc TLS fields are
zero, checks every owned mapping is actually unmapped, and restores both
`HeapStats.LiveAllocations` and `libc.MemStat` to baseline. libc's permanent
environment storage is initialized before that baseline. Context/TLS mapped
storage stays below 32 MiB before destructors, with less than 4 MiB additional
destructor storage; the bounded workload stays below 64 MiB.

The Rust fixture's negative control intentionally retains a translated Box,
requires a live-owner increase, then reclaims it through translated Rust Drop.
Normal Rust/C heap objects and uncaught Rust exceptions remain owned by their
Rust code; `Context.Close` does not implicitly free them. Context exception
transport tests explicitly take the opaque handle and free its test-owned
storage. Process-lifetime statics and allocator caches have separate accounting.

The optional `memory.counters oxide.heaptrace` build records outstanding Rust
allocations and return PCs only within `StartHeapTrace`/`StopHeapTrace` windows,
reported by `HeapTraceSnapshot`. It allocates Go memory and is excluded from
zero-Go-allocation performance measurements. Normal production hooks are
inlined no-ops. The combined arm64 chaos run with race detection and both tags
passes (17.777 seconds); this is diagnostic validation, not a throughput result.

The first renderer ownership check observed growing regex caches after changing
Contexts. Native Rust shows the same behavior with fresh threads; live-allocation
traces attribute the 24-allocation / 7,186-byte class-SVG increment to regex-automata
pool/DFA caches. The pinned dependency has eight pool stacks plus its initial-owner
cache. This is legitimate process-lifetime initialization, not a renderer leak
fix. Every final run therefore uses **nine fixed Contexts**, each exercising all
six selected SVG/PNG inputs (108 calls), then freezes all baselines. No adaptive
warm-up or increasing live-owner baseline is allowed.

The final three-ledger harness passes on both native architectures:

| Target | Seed | Epochs | Random calls | Stable Rust live owners |
| --- | --- | ---: | ---: | ---: |
| arm64 | `0x72656e646572` | 4 | 48 | 3,271 |
| arm64 | `0x12345` | 16 | 192 | 3,271 |
| amd64 | `0x72656e646572` | 4 | 48 | 3,275 |
| amd64 | `0xabcdef` | 16 | 192 | 3,275 |

Every epoch retains its own process baseline, rather than requiring live owners
to be zero or identical across targets. The complete libc snapshot remains
`{Allocs: 4, Bytes: 262144, Mmaps: 4}`. File-backed mapping bytes from
`/proc/self/maps`, grouped by device/inode, remain exactly at their baseline;
this covers fontdb/memmap2 storage outside the Rust allocator. A real temporary-file
mmap negative control is detected and then unmapped back to baseline. Native
SVG/PNG bytes and frame restoration are checked for every invocation.

Mapped-byte checks use the established baseline plus the independently bounded
allocator cache; they do not subtract cached mapping bytes from live storage.
The amd64 runs' maximum **epoch checkpoints** are 7,634,944 and 7,700,480 bytes,
not measurements of peak memory inside a render call. Anonymous Go heap mappings
are outside the file ledger; Context-owned anonymous mappings have their separate
`mincore` oracle. An audit of the frozen renderer graph found its direct OS
mappings are file-backed, so these ledgers cover that graph's reachable mapping
sources. Arbitrary user extern/syscall code remains outside that coverage claim.

Final evidence is in `.cache/renderer-conformance/heap-chaos-summary.json`
(`final_runs`) and `.cache/leak-chaos/remote/renderer-summary.json`. Context,
allocator and static-loader results are in `.cache/context-chaos/`, `.cache/chaos/`
and `.cache/leak-chaos/remote/runtime-summary.json`. The final default
`go test ./...` also passes; the previous 72-case and 4,011-input acceptance
results remain separate from these ownership tests.

From `oxide-go`:

```sh
go test -tags=memory.counters ./runtime -run 'Chaos|HeapLeakOracle|HeapMappingFailure' -count=1 -v
go test -race -tags=memory.counters ./runtime -run 'Chaos|HeapLeakOracle|HeapMappingFailure' -count=1 -v
OXIDE_CHAOS_SEED=0x12345 OXIDE_CHAOS_LONG=1 \
  go test -tags=memory.counters ./runtime -run '^TestContextLifecycleChaos$' -count=1 -v
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRustOwnershipChaos$' -timeout 30m -v
```

The runtime failures include seed and operation/epoch for replay. Translated
ownership and renderer chaos use `OXIDE_CHAOS_SEED` and `OXIDE_CHAOS_EPOCHS`.
From `oxide`, replay the renderer's default or extended run against retained MIR
and native references:

```sh
python3 fixtures/renderer/test.py --stage go --chaos
OXIDE_CHAOS_SEED=0x12345 OXIDE_CHAOS_EPOCHS=16 \
  python3 fixtures/renderer/test.py --stage go --chaos
```

The amd64 extended seed is `0xabcdef`. These bounded tests found no unmatched
owners or unbounded retained storage in the tested workloads. Longer runs
increase coverage; they do not prove leak freedom for every allocation path or
arbitrary Rust program.
