# Oxide translation milestones

Snapshot: 2026-09-27. Targets: **linux/amd64 and linux/arm64**.

## Direct library API (M6)

The renderer façade is removed from the translation input. Go
calls the original Cargo library with compiler-described `Type` / `Value`
views and explicitly invokes Rust drop glue. The earlier 977-root acceptance
below is the historical checkpoint at `8ae41df`, not evidence for the new graph.

| Direct API area | Current evidence |
| --- | --- |
| Compiler descriptors | both target metadata gates pass; actual canonical type identity, public/private fields, variant inhabitation, size/alignment, container offsets and private DST tails |
| Default / Display / Debug | compiler-resolved methods; Debug builds the pinned compiler's formatting argument types and calls actual Rust formatting functions |
| Go construction and cleanup | both native targets pass 14 cases: String, Vec, Option/Result, named variants, replacement, packed/aligned values, OS bytes, manual header allocation/free, private DST tails and aligned Box/dyn borrows |
| Allocation checks | every generic case passes 100 warmed calls with raw zero Go allocations/bytes, correct return-slot frames and Rust live owners restored |
| Direct owned-return boundary | 44 cases on both native targets, including thin/fat Box borrows, pointer metadata, exact result-slot frames and Drop panic propagation |
| Generic serde bindings | six cases × 100 calls on each native target; real to_string/from_str/to_value, u128, Vec, serde_json::Value, single-direction traits, input borrows and error Display; to_value preserves f32 promotion and key ordering; absent-dependency negative controls |
| Map access | real immutable/mutable HashMap/BTreeMap iterators and Iterator::next; both native targets pass access and mutation through borrowed entries |
| SIMD insertion | both native targets pass 768 lane/seed records × 100 calls; integer high bits, signed zero, NaN payloads, three-lane padding and overlapping assignment; raw Go zero and restored frames |
| Renderer final direct graph | both native targets pass all 74 API cases, 72 SVG/PNG cases, 20 CLI processes, eight ownership cases, exact allocation and ownership chaos |

The final graph has 973 upstream roots, 203 public type aliases, 788 API type
descriptors and 325 compiler destructors on each target. The two manifests agree
on the full 96-package normal/build dependency closure, versions and features.
The 74 direct API cases retain complete native output comparisons and 222 raw
allocation windows, with Rust live owners, libc counters and file mappings
checked against a fixed per-case baseline. Successful file writes must recreate
their output on every invocation.

| Final renderer gate | linux/arm64 | linux/amd64 |
| --- | --- | --- |
| Full generation, compile/link and default vet | passed; 396 Go files | passed; 401 Go files |
| 74 direct API comparisons / 222 raw allocation windows / three ownership ledgers | passed, 5.907 s | passed, 37.334 s |
| 72 cases / 144 complete SVG and PNG files | passed | passed |
| Eight owned returns / 24 raw allocation windows | passed | passed |
| Exact renderer allocation and fixed-baseline ownership chaos | passed | passed |
| Combined image/ownership/exact/chaos batch | passed, 224.375 s | passed, 1046.613 s |
| 20 CLI processes: exit, stdout, stderr and files | passed | passed, 9.228 s |

The final producer is `558bfc7cfec2a0eb27727dc42115a9d66300bbed825e15a9bb556ea969fcb0c0`.
MIR SHA256 is `d4043dd1505b9af227c514a8d68ccce2f1babbc977a4e18240ceae668a6ab7b0`
on arm64 and `b75b52545e33b47d2393d18cb5c6389637106944b3780d111ce09c6c2ca864d6`
on amd64. Exports and compiler options are recorded in
`.cache/renderer-direct/{arm64,amd64}/export-result.json`.
ARM evidence is `api-iteration2.log`, `regression-jsonvalue.log` and
`cli-jsonvalue.log` in its cache. AMD evidence combines `api-corrected.json`
with `final/validation-summary.json`: an earlier observer put Go defers inside
a loop, allocating three Go objects per constructor call. Moving cleanup
outside the loop restored raw zero without changing Rust operations or output
checks. The original failed log remains; the production graph is unchanged.

`Value.Drop` invokes drop glue in place. `Storage.Free` releases only manually
allocated value storage; `Storage.Close` performs both operations. Context
Restore/Close do not implicitly destroy arbitrary owned Rust values. Public
function signature metadata and type aliases avoid numeric type IDs in clients.
The new API does not preserve the old public `DropT<ID>` or aggregate ABI.

The new generic tests execute on both native targets:

| Gate | Cases per target | Measured calls per case | Evidence |
| --- | ---: | ---: | --- |
| Direct Rust types / memory | 14 | 100 | `.cache/direct-type-conformance/summary.json` |
| Public ownership / panic | 44 | 100 | `.cache/interop/ownership-borrows.log`, `.cache/direct-type-conformance/amd64-ownership-go.log` |
| serde JSON / JSONValue | 6 | 100 | `.cache/json-conformance/final-value-summary.json` |
| SIMD lane insertion | 768 | 100 | `.cache/simd-insert-conformance/summary.json`, `remote/summary.json` |
| Assignment replacement / Drop | 5 | 100 | `.cache/replace-conformance/gate.log`, `amd64-summary.json` |

Each target builds and executes its own pinned Rust reference. The SIMD gate
compares individual lane bits, including NaN payloads, rather than hashes or
approximate numeric values. A real allocation negative control verifies that
the raw Go counter would catch even one allocation in a measurement window.
The tutorial's complete Go example also builds and produces SVG and PNG bytes
equal to its native flowchart reference on arm64.

Replacement tests cover independent zero-sized owners at the same address,
normal assignment, old-destructor panic and destructors that mutate the original
right-hand-side storage. The right-hand side is saved before Drop and installed
on both normal and unwind paths, matching native Rust; frames and live owners
return to baseline. A separate process test checks real Rust MIR cleanup's
double-panic abort on both targets. Go-authored defer cleanup is not substituted
for a Rust cleanup boundary. After this runtime-only correction, renderer API
and owned-return gates pass again on both targets: arm64 takes 7.763 s and
amd64 24.515 s. These runs retain all 222 API and 24 owned-return allocation
windows and ownership assertions. The updated runtime source SHA256 is
`a89b77f6db8cb7eef39ecf7543987caab3985d5dee4a868ef12a5884f1de8a1e`.
Evidence is `.cache/renderer-direct/arm64/replace-addendum.log` and the
amd64 `replace-addendum/replace-addendum.json` and logs. The generated Rust graph
is unchanged; the complete image/CLI evidence above is retained for paths
which do not call the Go `Value.Replace` helper.

From `oxide-go`, regenerate these checks with:

```sh
OXIDE_RUST_TESTS=1 go test ./internal/mir \
  -run '^TestRust(TypeAPI|PublicOwnership|JSONAPI|SIMDInsert|Replace)Conformance$' \
  -count=1 -timeout=30m -v
```

This is a support ledger, not a claim of full Rust or standard-library support.
“Verified subset” means the named cases have generated, compiled Go and agree
with native Rust on the recorded target. All eight language/library differential
suites and the complete 72-input renderer corpus execute on native arm64 and
amd64 hardware, including the current dynamic-layout and large-value ABI changes.
“Exported” means compiler metadata is available; it does not mean runnable Go.
At the historical M5 checkpoint, the public-library graph passed all 74 API, 20 CLI and 72-source regression
cases on both targets, including compiler drop helpers, direct-return ownership,
exact renderer allocation checks and the three-ledger ownership-chaos regression.

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
| M5: renderer public library API | public export and acceptance matrix passed on both native targets | all 977 monomorphic upstream/test roots selected, 139 compiler drop types; 74 API, 20 CLI and 72-source regression cases; 44 dedicated ownership and eight direct renderer-return cases per target; individual derived/default methods and unbounded generic instantiations are not fully tested |
| M6: direct Go use of Rust libraries | verified subset on both native targets | upstream manifest without façade; Go constructs/borrows/drops Rust values; complete renderer matrix and generic type/ownership/JSON/SIMD differential gates pass |

The M4 results describe the two-observer acceptance at commit `6989058`.
M5 used a fixture that re-exported the upstream library and four test observers.
M6 directly translates the unchanged upstream manifest with its default features
plus `scene`; the fixture Cargo package now contains native oracle binaries only.
The [authoritative API inventory](fixtures/renderer/api-inventory.md)
identifies 28 public free functions, 32 inherent methods, 18 handwritten trait
methods, 430 derive-generated methods and 15 callable tuple-variant constructors
with `scene` enabled. Counting aliases and provided trait defaults gives 1,089
upstream paths: 973 monomorphic and 116 requiring concrete type/const arguments.
M5's four test observers brought its selected count to 977; M6 has only the 973
upstream roots. Export coverage does
not imply individual behavioral coverage of every derived/default method;
blanket implementations and all generic instantiations are not a finite root set.

## Compiler pipeline

| Area | Status | Evidence / remaining work |
| --- | --- | --- |
| Cargo discovery and package selection | implemented, driver tested | pinned Cargo; explicit selected package/library/bin |
| Repeated exports | verified | unique final rustc export argument forces a fresh root; changing roots in an unchanged crate produces new MIR |
| Public API selection and aliases | verified export subset | empty roots traverses the public namespace, re-exports, inherent/trait methods, provided defaults and callable constructors; sidecar records canonical trait definitions and generic boundaries; all 973 upstream renderer roots selected on both targets |
| Public Go root names | verified subset | preserve module/type paths and aliases; UFCS names include type and trait; deterministic encoding and explicit collision errors; direct calls, aliases, methods, track_caller and variadic forwarding covered by root tests |
| Compiler options | verified | checked/unchecked overflow, Cargo features and disabling default features; ambient encoded Rust flags and export roots cannot silently override the request |
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
| `core::option` | verified subset | NonZeroU64/U128 representation and match; Go named-variant construction and String ownership use actual compiler tags/niches; complete Option API not claimed |
| `core::result` | verified subset | catch results and dropping Err<Box<dyn Any + Send>> execute with native-equivalent payload destruction; direct Go Results preserve nested owners, borrowed results and serde errors; broader Result API coverage pending |
| `core::array`, `slice`, `str` | partial | array and slice storage/projections; broad iterator/UTF-8 suites pending |
| `core::ops`, `marker`, `convert`, `clone`, `cmp` | partial | rustc resolves concrete instances; fixture-used operations only |
| `core::fmt`, `core::error`, `core::any` | verified subset | HRTB/dyn metadata, TypeId equality, mixed string/u64 formatting callbacks and dynamic payload downcasts; generic Go Debug/Display execute compiler-resolved Rust formatting, including Box<dyn Debug>; complete formatting/error APIs remain unverified |
| `core::cell`, `pin`, `borrow`, `iter` | unverified | translated when reachable, no complete API acceptance suite |
| `core::sync::atomic` | verified single-thread subset | AtomicUsize hook/drop counters and relaxed load/store/fetch operations; memory-order and multithreaded conformance pending |
| `core::arch` / CPU feature detection | verified runtime subset | actual CPUID/XGETBV and required SSE/AVX helpers pass on native amd64 hardware; Rust feature checks remain reachable, no hardcoded CPU capabilities; the complete translated renderer is checked separately |
| `alloc::alloc` | verified subset | allocator primitives and fresh Rust allocation call chains; transparent Alignment/NonNull ABI parameters |
| `alloc::boxed`, `vec`, `string` | verified subset | Box creation/drop, Vec growth/push/reverse, String creation/push_str/drop; direct Go String/Vec construction, moves, field replacement, Box dereference, manual header allocation/free and in-place Rust Drop; both-target 14-case type API and 44-case ownership gates retain raw zero Go allocations |
| `alloc::collections` | renderer-exercised subset | BTree and VecDeque paths participate in matching renderer outputs; dedicated collection/API and drop tests remain open |
| `alloc::rc` | unverified | no dedicated Rc/Weak ownership tests |
| `alloc::sync` | verified Arc subset | payloads with 32/64-byte alignment, clone/drop and nested DSTs pass the 42-case fixture on both targets; weak-reference and concurrency coverage remains open |
| `std::collections` | verified iterator / renderer subset | actual HashMap/BTreeMap iter/iter_mut/Iterator::next exposed to Go, including mutation through borrowed entries; original Rust HashMap/HashSet, BinaryHeap and VecDeque paths contribute to matching outputs; complete ordering/hash/API conformance remains open |
| `std::fmt`, `error`, `any`, `panic` | verified subset | 28 native differential cases cover custom hooks, panic_any, catch_unwind/downcast, overflow panic and mixed format! output; complete error/formatting APIs and double-panic paths remain open |
| `std::fs`, `io`, `path`, `ffi`, `os` | verified boundary subset | extern-C fixture verifies weak slots, variadic open, mkstemp/read/write/lseek/close/unlink, errno and posix_memalign/free; directory tests cover fdopendir descriptor ownership, lstat symlink behavior and unlinkat relative removal; both-target API/CLI probes compare configuration/file errors and SVG/PNG/layout-dump output; broad std API behavior remains unverified |
| `std::env::args_os` | verified startup subset on both targets | actual startup argument bytes, including empty and invalid UTF-8 strings, forward/reverse iteration and preservation after the embedding Go program edits os.Args; environment-variable and full env API conformance remain open |
| `std::sync`, `thread`, TLS | partial runtime boundary | single-thread renderer Mutex/LazyLock paths execute; Context TLS isolation, C++ destructor LIFO/re-registration and up to four pthread-key destructor passes tested at Context.Close; complete thread exit, synchronization and native thread semantics remain open |
| `std::time` | verified rendering subset on both targets | real timed render/layout calls and CLI timing check nonnegative fields and exact totals; elapsed durations are not expected to match independent native runs; broad clock/calendar behavior remains unverified |
| `std::backtrace` / ELF loader boundary | partial | CGO-disabled static ELF main executable/vDSO enumeration; dynamic `PT_INTERP` explicitly rejected. Real Go PC/SP backtraces follow Go stack movement; Rust source symbol/line mapping is not implemented |
| `std::process` | verified exit subset on both targets | native/Go exit codes, buffered stdout cleanup and C++ TLS destruction match; stack Drop and pthread-key destructors are not run by process exit; process spawning, pipes, signals and broad process APIs remain unverified |
| `std::net` | unsupported | no complete OS boundary |
| `std::future`, `task`, async | unsupported | no executor/coroutine acceptance |
| re-exported core/alloc modules in std | same limits | no separate implementation or compatibility shim |

## Historical renderer checkpoints (M4/M5)

This section records the acceptance at `6989058`, `37afa3b` and `8ae41df`.
Its façade and `DropT<ID>` interfaces have been removed by M6. Use the direct
API section above and the current tutorial for the supported calling contract.

The M5 acceptance input was a fixture Cargo library, a
Rust path dependency on the unchanged upstream `mermaid-rs-renderer` with
**default features enabled plus `scene`**. Its façade re-exported the public
library without shadowing names. Test observers are separate:
`fixture::render_svg(source, length, output, capacity)` copies the complete SVG;
`fixture::write_png(source, length, path, path_length)` invokes the PNG writer;
`fixture::cli_main` invokes the original CLI; and `api::run` executes Rust API
probes. Go supplies ABI storage, process inputs and comparison oracles, with no
renderer implementation.

### Expanded public library graph

Pinned rustdoc public reachability and the compiler's `.api.json` sidecar agree
on every free/inherent/constructor path. The
[coverage gate](fixtures/renderer/check_api.py) checks all upstream paths,
canonical trait identities, selection/root symbols and native probe manifests.
Both target sidecars pass: 1,093 total paths, 977 selected monomorphic roots and
116 generic declarations. Eight negative controls reject missing roots/aliases,
unselected APIs, unregistered additions, incorrect traits and stale probes.

| Expanded acceptance | arm64 | amd64 |
| --- | --- | --- |
| Native 74 API cases, each size query plus two complete byte results | passed | passed |
| Native 20 CLI process cases | passed | passed |
| All 977 public/test export roots and coverage gate | passed | passed |
| Complete Go generation, compile/link and default vet | final helper graph passed | final helper graph passed |
| 74 API native/Go comparisons, frame/output guards, three warmed raw allocation windows per case | final graph passed; all 222 windows raw zero | final graph passed, 23.936 s; all 222 windows raw zero |
| 20 CLI native/Go exit/stdout/stderr/file comparisons | final graph passed | final graph passed, 5.367 s |
| 72 source / 144 SVG+PNG byte regression using the expanded graph | final graph passed, 165.25 s | final graph passed, 456.118 s |
| Direct owned returns: eight cases, 24 raw allocation windows, compiler drops and Context closure | passed, 2.84 s; live owners restored, raw Go objects/bytes zero | passed, 6.243 s; live owners restored, raw Go objects/bytes zero |
| Exact renderer heap checks and fixed-baseline three-ledger chaos | final graph passed, 10.32 s and 53.93 s | final graph passed, 28.503 s and 177.302 s |

The pre-helper public graph had 123,607 MIR functions, approximately 831 MiB
JSON and 396 Go files on arm64. Final sidecars retain all 977 roots and add 139
compiler drop types. Acceptance durations above are test durations, not native
performance comparisons.

The [Rust API probes](fixtures/renderer/api.rs) call the actual configuration,
strict parser, layout, dimension, quality, theme, timing, scene and constructor
APIs. Timed APIs execute normally and check total/phase relationships; elapsed
values are excluded from cross-execution byte equality. Scene output compares
all Rust Debug commands. Graph map presentation is sorted without discarding
fields; file errors only replace the caller's temporary path. Concrete serde
instances are tested, while untested derived/default methods remain explicitly
marked in the [machine inventory](fixtures/renderer/api-inventory.json).

Direct Go callers receiving owned Rust values have a separate ownership
acceptance boundary. The initial cached probe observed `String` increasing live owners
from zero to one, and `Vec<String>` from zero to three; `Context.Close` leaves
those owners live, while an explicit Rust release returns each baseline to zero.
This is not a Context mapping leak: closing automatic storage does not invoke
the returned value's Rust destructor. The 74 API observers perform their own
Rust drops and therefore did not validate this boundary. M5's public `DropT<ID>`
helpers called actual compiler drop glue. Logical root parameter/return IDs
and `public_drop_types` select the Rust destructor without guessing from a
shared Go representation. Small values are consumed by value; large values
use their initialized caller storage. All raw-bit copies become unusable after
ownership is consumed. The dedicated 44-case fixture passes on both native
targets, including exact allocation checks. The
then-used `fixtures/renderer/write_owned_test.py` generator added eight
renderer cases: Theme, Config, RenderOptions, successful/failed parse and render,
and scene rendering that consumes RenderOptions. It calls public Go roots and
`DropT<ID>` directly, without a Rust release observer. On both targets, each of 24
measured windows observes owned Rust allocations before Drop, restores its
live-owner/frame baseline after Drop and records zero raw Go allocations/bytes.
After Context.Close, Rust and libc counters match the fixed nine-Context warm
baseline. These eight cases are separate from the 74 observer-based API cases.

The [CLI matrix](fixtures/renderer/test_cli.py) runs separate processes for
help/version, bad input/options, stdin/stdout, SVG/PNG files, dimensions,
configuration, both layout dumps, real timing and multi-diagram Markdown.
It compares exit status, stdout, stderr and every file. Timing stderr retains
its original measured payload and is checked for schema, nonnegative fields
and exact sums before its deterministic contract result is compared.

Final arm64 logs are `.cache/renderer-api/{owned-arm64,api-owned-arm64,cli-owned-arm64,regression-owned-arm64}.log`.
Final amd64 results are `.cache/renderer-api/amd64-final/validation-summary.json`,
`go-results.json` and `logs/go-*.log`; every acceptance process exited successfully.
The earlier public API baseline remains under `.cache/renderer-api/amd64/`.
The small API sidecar permits coverage checks
without loading the complete MIR. These new results do not replace the older
4,011-input language/library suites or claim full standard-library coverage.

### Prior SVG/PNG and performance baseline

The corpus contains **72 inputs across 23 diagram types**: the original three
examples and all 69 upstream `.mmd` fixtures, copied unchanged with directory
structure, [source commit and license](fixtures/renderer/cases/upstream/README.md)
preserved. Native Rust renders each input twice and confirms deterministic SVG
and PNG bytes. All 72 native cases pass on **both architectures**, producing
**144 reference files per target**.
Neither errors nor unequal output are skipped by the comparison gate.

The preceding two-observer export rebuilt `std,panic_unwind` and contained about 100,000
functions, with actual assertion calls, unwind paths and RustCall adaptation.
The shared writer emits numbered Go source files for the whole program.
That graph passed generation, compilation, default vet and **all 72 cases
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
Those timings belong to the preceding graph, not the expanded API acceptance.

Ordinary floating-point MIR operations preserve individual rounding;
native f32/f64 regression cases cover the unintended Go fusion found while
expanding the renderer suite.
Separately, 68 current runtime tests pass on native amd64 hardware, including actual
SSE operations, CPUID/XGETBV, integer helpers, allocator, backtrace and libc
tests with updated raw allocation checks. The 18 upstream/registry tests also pass on
that hardware; final logs are retained with the allocator revalidation artifacts.

The recorded two-observer warmed-call benchmarks use the off-heap registry. arm64 pins both
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

The recorded arm64 benchmark evidence under `.cache/renderer-conformance/` is
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
The preceding build passed the complete corpus on both targets, including
large-value storage and calls. Expanded API acceptance is tracked separately
above; neither matrix proves every implementation branch. The syntax inventory is
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

The final `go test ./...` run passes, including the public-root/drop-helper
changes; the retained log is `.cache/renderer-api/final-go-test.log`.
The preceding allocator checkpoint is retained in `.cache/final-go-tests.log`.

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
Public-root checks also cover re-exports, callable constructors, canonical trait
identities, provided defaults and generic declarations.

The separate [process differential fixture](oxide-go/internal/mir/process_conformance_test.go)
passes on both native targets. Five argument byte vectors include empty,
Unicode and invalid UTF-8 inputs; each is compared before and after the Go host
changes `os.Args`, giving ten startup comparisons. Four exit codes (`0`, `7`,
`-1`, `256`) compare native status, buffered stdout and TLS destructor output.
The observed GNU exit behavior runs C++ TLS destructors but not Rust stack Drop
or pthread-key destructors. These 14 process comparisons are additional to,
and not included in, the historical eight-suite total of 4,011. Run from
`oxide-go`:

```sh
OXIDE_RUST_TESTS=1 go test ./internal/mir \
  -run '^TestRustProcessConformance$' -timeout 30m -v
```

The [public-ownership fixture](oxide-go/internal/mir/public_ownership_test.go)
adds 44 native cases per target. An external Go package directly consumes
compiler-described `Value` results with `Value.Drop` at their actual address;
there is no Rust release wrapper. Strings, `Vec<String>`, thin/dynamic boxes,
64-byte alignment, 256 KiB values, enums, zero-sized values, borrowed elements,
thin/fat pointer metadata and destructor-panic cleanup all pass. Each input checks 100 warmed calls with
exact Go object/byte deltas, Rust live-owner baselines, restored frames and
native drop/panic results. Both targets compile with `-smallframes` and default
vet and execute against their own pinned native Rust references. Results are
retained in `.cache/public-ownership-conformance/`, separately from the older
4,011-input suite. Run from `oxide-go`:

```sh
OXIDE_RUST_TESTS=1 go test ./internal/mir \
  -run '^TestRustPublicOwnershipConformance$' -timeout 30m -count=1 -v
```

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
# Check the small public API sidecar independently:
OXIDE_CHECK_CACHE=".cache/renderer-direct/$(go env GOHOSTARCH)"
python3 fixtures/renderer/check_api.py \
  "$OXIDE_CHECK_CACHE/oxide.mir.api.json" \
  --cases "$OXIDE_CHECK_CACHE/reference/api/cases.json" \
  --cli-results "$OXIDE_CHECK_CACHE/cli/native/results.json"
# After generating the host-target package, measure the original three inputs:
cd "$OXIDE_CHECK_CACHE/go"
CGO_ENABLED=0 GOWORK=off GOCACHE="$PWD/../go-cache" GOTMPDIR="$PWD/../go-tmp" \
  taskset -c 2 go test -tags=memory.counters -mod=mod -run '^$' -bench '^BenchmarkRenderer(SVG|PNG)$' \
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
  CARGO_TARGET_DIR="$PWD/.cache/renderer-direct/arm64/cargo-target" \
  "$OXIDE_BENCH_SYSROOT/bin/cargo" build -Zbuild-std=std,panic_unwind --locked \
  --manifest-path fixtures/renderer/Cargo.toml --target aarch64-unknown-linux-gnu \
  --bin oxide-renderer-bench
taskset -c 2 .cache/renderer-direct/arm64/cargo-target/aarch64-unknown-linux-gnu/debug/oxide-renderer-bench \
  fixtures/renderer/cases .cache/renderer-direct/arm64/reference \
  .cache/renderer-direct/arm64/native-bench-output
```

The equivalent Go gate, from `oxide/oxide-go`, is:

```sh
OXIDE_RENDERER_TESTS=1 go test ./internal/mir \
  -run '^TestRendererConformance$' -timeout 60m -v
```

Renderer acceptance is separately gated because it builds the complete
application dependency graph. Setting either gate runs the requested check;
a compiler or runtime failure is reported as a test failure.
The runner preserves default vet while using `-p=1` and, unless explicitly
overridden in the environment, `GOGC=25`, `GOMEMLIMIT=20GiB`, `GOMAXPROCS=4` for
Go build/test commands. These control build/test concurrency and GC pressure;
the soft memory setting neither reserves 20 GiB nor describes a per-call cost.

Update this ledger in the same change as each newly verified library family.
Do not promote an exported module to “supported” based only on graph traversal.

## Historical ownership and leak chaos

The measurements below predate M6 and remain regression context. M6 repeats
the three-ledger checks against the direct-library graph with its own frozen
baseline; see the current acceptance table above.

The recorded renderer chaos results below use the preceding two-observer
graph. They remain evidence for those runs; the expanded public API graph has
separate export, API/CLI, SVG/PNG and final ownership replay results above.

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
