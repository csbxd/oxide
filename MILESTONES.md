# Oxide translation milestones

Snapshot: 2026-10-01. Targets: **linux/amd64 and linux/arm64**.

## SIMD comparison-mask consumers (M18 follow-up)

The f128 comparison mask now remains usable through select, all/any,
bitmask extraction, bitwise combinations and integer reductions. These
operations use typed zero/comparison/bitwise helpers for I128/U128 rather
than native Go integer operators; bitmask extraction reads bit 127 from
the high word. Select accepts masks whose element width differs from the
selected data, as permitted by Rust, and snapshots operands before writes.

Generated-code regressions cover signed/unsigned 128-bit masks, narrower
masks, all truth patterns, NaN comparison, selected NaN payloads/signed zero,
destination aliasing, mixed-width selection and both words of an integer
reduction. A fresh Rust-to-Go fixture verifies all relevant intrinsic paths
and 1,024 native arm64 result records with zero warmed Go allocations and
restored Context frames; generated code compiles for amd64 and arm64.

## Built-in binary128 transcendental math (M18)

The 13 binary128 math cases left open by M17 now pass with the default
runtime. No user provider or native C library is needed. The runtime uses
the fixed-size binary256 arithmetic and selected functions from
[`github.com/shogo82148/floats` v0.4.0](https://github.com/shogo82148/floats/tree/v0.4.0)
and `github.com/shogo82148/ints` v0.1.3. Inputs extend exactly to 237-bit
significands and results round back to binary128; the existing core
arithmetic/FMA/sqrt implementation is unchanged. `Context.Binary128Math`
remains an optional override of these built-in operations.

Exp/log families use generated coefficients and Horner evaluation with fused
arithmetic. Inverse hyperbolic functions avoid cancellation through log1p;
erfc computes small tails directly with a continued fraction. Pow preserves
small differences from one even with huge exponents and handles signed
zero/infinity and negative bases explicitly. Gamma and inverse-trigonometric
functions use the wider library implementations, with corrected lgamma
sign/domain handling. Binary16's remaining C math entries (including cbrt,
asin, atan, sinh, tanh, erf and erfc) use the pinned modernc libc bodies.

Trigonometric reduction multiplies the full binary128 significand by a
16,896-bit expansion of 2/pi. The generator verifies that the expansion is
the exact integer floor using directed MPFR bounds. Continued-fraction
convergents then certify a residual lower bound for every binary128 exponent
and every 113-bit significand. This covers large finite arguments which
cannot be reduced correctly using a rounded floating-point pi.
Both the constant generator and independent test oracle live in
`oxide-go/runtime/testdata`; neither C nor MPFR is linked into generated Go.

The checked-in MPFR corpus covers **15,612 cases across 27 operations**,
including the full exponent range, dense ordinary inputs, subnormal tails,
NaNs/infinities/zero signs, near-one huge powers and random large-angle
reduction. All measured results are within **1 ULP**; special-value classes
and defined signs are checked separately. The oracle uses MPFR's ternary
inexact result and subnormalize operation to prevent reference double
rounding. This is measured numerical coverage, not a claim that every
transcendental result is correctly rounded for every possible input.

A new Rust/Go fixture exercises all 30 math method paths for both float
widths, with **1,800 records** and raw-zero warmed Go allocations. It invokes
the real compiler-provided methods, including C-boundary methods, sin_cos,
arbitrary-base log and lgamma's sign pointer. Rust's unspecified
transcendental precision permits small differences; native results are
compared within four ULP, while the independent runtime gate uses one ULP.
The sign of Gamma at negative integer poles and -infinity is undefined and
is not compared across libc implementations. Signed zero remains checked.

Both native targets pass the runtime tests, the 15,136-record core float
fixture and the 1,800-record math-method fixture, including their raw-zero
Go allocation and frame checks. The amd64 references use the matching
nightly-2026-09-15 compiler and LLVM 23.1.1, installed in an isolated test
directory on the supplied x86_64 Linux machine.

Full upstream promotion reran all **4,636 prior successes plus 13 additions:
4,649 passed** in 1,787.90 seconds with no regression or timeout. Compiler,
source, adapter, reference flags and discovery context are unchanged from
M17. The baseline now contains 2,843 core, 1,478 main alloc, 326 internal alloc
and both auxiliary successes. The remaining four unsized FnOnce failures
and seven native timeouts are outside this float change and were not selected;
two upstream cases remain ignored. Evidence: `.cache/quadmath/`,
`.cache/float-math-conformance/`,
`.cache/upstream/arm64/f128-math-scan-report.json` and
`.cache/upstream/arm64/f128-math-promotion-report.json`.

## Binary16 and binary128 (M17)

`f16` and `f128` now have exact 2-byte and 16-byte IEEE representations,
including scalar ABI wrappers/constants, public storage, packed fields,
transmute and indirect calls. rustc still supplies alignment and field offsets;
addressable binary128 storage uses the existing aligned Context frames.
Go's by-value `F128` representation has alignment 8, not Rust's alignment 16.

The runtime implements arithmetic, remainder, ordered/unordered comparisons,
negation, abs/copysign, min/max, integral rounding, square root, fused
multiply-add, integer powers, all float-width casts and saturating integer
casts through 128 bits. Binary128 uses fixed-width integer significands and
256-bit products/FMA sums, with guard/sticky bits and one final rounding.
It never narrows arithmetic to f64 and never allocates arbitrary-precision
objects. Binary16 operations use wider arithmetic with direct binary16
rounding. SIMD arithmetic/comparisons, unary operations, square root,
conversions and min/max reductions use the same numeric helpers.
Rust's remaining core methods, parsing and formatting retain their MIR bodies.

Correctness is defined by Rust's semantics, not by reproducing compiler bugs.
[LLVM #98389](https://github.com/llvm/llvm-project/issues/98389) is covered by
the exact `0x520b * 0x00e9 + 0x2ff6 -> 0x3001` FMA regression; the incorrect
f32 intermediate produces `0x3000`. Native default debug also reproduces an
arm64 f16-to-i128 narrowing/sign-extension defect. Reference builds now use
`-Cllvm-args=-global-isel=0`, preserving debug assertions and MIR while changing
only native instruction selection. The dedicated differential test compares
SelectionDAG debug and optimized references before checking generated Go.
The pinned native software copysign can quiet a signaling f16 NaN even on
SelectionDAG; that probe uses the specified raw-bit sign operation as its
oracle. Generated copysign preserves the signaling bit and payload.

Validation includes all 65,536 f16 bit patterns, every finite positive f16
midpoint and adjacent f64 values, 20,000 random f16 FMA triples, exact 66,000-bit
reference arithmetic for f128 boundary/random operations, direct narrowing
and integer-cast checks, and negative integer powers whose result is subnormal
despite overflow of the corresponding positive power. The Rust/Go fixture
compares **15,136 records** (172 cases x 88 inputs), including 16-byte alignment,
packed loads, NaN payloads and ABI-compatible transparent function pointers.
Both targets compile; native arm64 executes. Warmed runtime and generated-code
checks measure zero Go objects/bytes and restored Context frames. Existing
12,644-record numeric tests, default Go tests, both-target compiler metadata,
and 136 upstream-runner tests pass.

The 177 previously blocked float candidates were rescanned with unchanged
upstream bodies and assertions: **164 pass**, including 66 f16 and 55 f128
float-method cases, 19 decimal-to-float, 15 float-to-decimal, six round-trip
and three conversion cases. The remaining **13 require a binary128 math
provider**: acosh, asinh, atanh, exp, exp2, gamma, ln, ln_gamma, log, log2,
log10, powf and real_consts. They fail explicitly without a provider.
Binary128 transcendental functions are not claimed as built-in support.
`Context.Binary128Math` accepts full binary128 bits, supplies unary/binary
operations and the lgamma sign, and is tested for full-bit transport and zero
dispatch allocations. Binary16 transcendental intrinsics use wider Go math;
their precision remains unspecified as in Rust's API contract.

The new reference flags change the recorded upstream context. Full promotion
reran **all 4,472 prior successes plus 164 additions: 4,636 passed** in
1,763.22 seconds, with no regression or timeout. The baseline was migrated
only after checking that every old success remains, discovery is identical,
and the only context change is the native instruction-selection flag.
There are 2,830 core, 1,478 main alloc, 326 internal alloc and two auxiliary
successes. Promotion leaves 24 cases unselected and two ignored: the 13 math
provider cases were measured separately; the four unsized FnOnce failures
and seven native slice timeouts remain historical, not remeasured by M17.
Evidence:
`.cache/upstream/arm64/soft-float-scan-report.json`,
`.cache/upstream/arm64/soft-float-promotion-report.json`,
`.cache/upstream/arm64/soft-float-baseline-migration.json` and
`.cache/soft-float-conformance/`. Native amd64 replay remains unverified.

## Single-rounding f32 fused multiply-add (M16)

The unchanged num::floats::mul_add::test_f32 case passes after adding the
missing fmaf32 dispatch. FMA32 specializes the pinned Rust libm
[fma_wide_round](https://github.com/rust-lang/compiler-builtins/blob/main/libm/src/math/generic/fma_wide.rs)
algorithm to round-to-nearest f32. A float32 product is exact
in float64; halfway sums use their discarded error before conversion to f32,
avoiding double rounding. The source attribution and upstream license notices
are retained in the runtime. The existing fmaf64 path remains math.FMA.

The numeric differential fixture now verifies 116 cases x 109 inputs = 12,644
records. New probes cover intrinsic calls over arbitrary bit patterns,
halfway sums with tiny positive/negative addends, subnormal ties and cancellation
of intermediate overflow. Native arm64 results, raw-zero warmed Go allocations
and restored Context frames pass; both targets compile. Independent runtime
tests compare 52,820 finite boundary/random triples with exact 1024-bit
arithmetic, and check NaN/infinity/signed-zero combinations separately.
Default Go and both-target compiler metadata checks pass.

Full inventory promotion measured all 4,662 cases in 8,069.71 seconds:
**4,472 passed**, 180 generation failures, one native assertion failure,
seven native timeouts and two upstream ignored cases. No case is unselected.
The baseline retains all 4,471 M15 successes and adds only the f32 FMA case;
compiler/source/dependency/adapter context and discovery are unchanged.
It contains 2,666 coretests, 1,478 main alloctests, 326 internal alloc cases
and both auxiliary targets. Every passing case ran native Rust, fresh export,
Go build and generated execution.

All 180 generation failures were checked: 176 require f16/f128 ABI support
and four require by-value unsized FnOnce support. The native f16 FMA assertion
still returns 0x3000 instead of 0x3001; seven unoptimized native slice tests
retain their extreme inputs and exceed the 30-second limit. These require
broader features or changes outside translation and were left unchanged.
There is no generated-Go execution failure or previous-baseline regression.
Evidence: `.cache/upstream/arm64/fma32-full-report.json`,
`.cache/upstream/arm64/fma32-before-report.json`,
`.cache/upstream/arm64/fma32-scan-report.json` and `.cache/numeric-conformance/`.
Native amd64 replay of these new cases remains unverified.

## C variadic formatting and temporary argument storage (M15)

The unchanged ptr::test_variadic_fnptr test passes after connecting real
libc printf instead of rejecting its declaration. Printf and snprintf use
the pinned modernc libc implementations; fixed C arguments retain validated
scalar signatures, and promoted f64 varargs carry their original bits.
Direct and indirect calls reuse the translated variadic function ABI.

Fresh libc differential tests cover nine cases x three inputs = 27 records.
New probes invoke actual formatting with signed integers, high u64 values,
f64, strings and %n; check truncation, complete return counts and termination;
and call snprintf/printf through variadic function pointers. An initial
allocation check exposed one escaping Go argument array per indirect call.
The emitter now snapshots those words in temporary Context storage and
restores it with defer, including panic exits. All 27 comparisons and raw-zero
warmed Go-allocation/frame windows pass. Both targets compile and arm64
executes the native comparisons. Default Go and compiler metadata tests pass.

Promotion reran every M14 success plus the pointer candidate: **all 4,471
passed** native Rust and generated Go in 995.99 seconds, without failure or
timeout. The baseline contains 2,665 coretests, 1,478 main alloctests, 326
internal alloc cases and both auxiliary targets. Context, discovered IDs and
every prior success are preserved; 189 cases were unselected and two remain
ignored. Evidence: `.cache/upstream/arm64/printf-final-report.json`,
`.cache/upstream/arm64/printf-scan-report.json` and `.cache/libc-conformance/`.
Native amd64 replay of these new cases remains unverified.

## Coroutine discriminants (M14)

Three unchanged future::join upstream tests now pass native Rust and
generated Go. Coroutine layouts already exported Rust's tag encoding and
variant field offsets, but their discriminant values were missing. The exporter
now queries the pinned compiler's CoroutineDef::discriminant_for_variant for
every layout variant. State values and field layouts are not guessed; existing
MIR polling, pinning, wake and drop operations are retained.

The numeric differential fixture now covers 111 cases x 109 inputs = 12,099
records. New manual-poll coroutine probes check immediate Ready, multiple
Pending polls, nested awaits and cancellation while suspended. Native Drop
traces distinguish completion (second then first guard) from cancellation
(only the initialized first guard), and verify poll counts and borrowed capture
storage. Raw-zero warmed Go allocations and restored Context frames pass.
Both targets compile and arm64 executes native comparisons. These checks
establish a polling/lifetime subset, not a general executor or native threading
contract. Default Go and compiler metadata checks pass.

Promotion reran all 4,467 M13 successes plus the three candidates: **all
4,470 passed** native Rust and generated Go in 944.43 seconds, without failure
or timeout. The baseline comprises 2,664 coretests, 1,478 alloctests, 326
internal alloc cases and both auxiliary targets. Context, discovered IDs and
all earlier successes are retained. The 190 unselected cases and two ignored
cases remain outside the passing set. Evidence:
`.cache/upstream/arm64/coroutine-tag-final-report.json`,
`.cache/upstream/arm64/coroutine-tag-scan-report.json` and
`.cache/numeric-conformance/`. Native amd64 execution of the new cases remains
unverified.

## SIMD negation and absolute value (M13)

The unchanged coretests/simd::testing case now passes native Rust and
generated Go. Two missing intrinsic entries, simd_neg and simd_fabs, reuse
the existing vector snapshots and lane layout. Floating lanes flip or clear
the sign bit, preserving signed zero, infinities, NaN payloads and signaling
bits. Signed integer lanes use wrapping negation, including minimum values.
Neither operation introduces heap storage or a function-specific wrapper.

The numeric differential fixture now verifies 107 cases x 109 inputs =
11,663 records, adding f32/f64 vector negation/absolute value and signed
8/16/32/64-bit vector negation. Native arm64 results, raw-zero warmed Go
allocations and restored Context frames pass; generated code compiles for
both targets. Additional generated tests check aliased destinations and
three-lane vectors with tail padding, including signaling NaN bit patterns.
Those tests execute on arm64 and cross-compile for amd64. Default Go tests
and both-target compiler metadata checks pass.

Evidence: `.cache/upstream/arm64/simd-unary-scan-report.json`,
`.cache/numeric-conformance/` and TestSIMDUnarySnapshotsAndPadding.
Promotion reran all 4,466 M12 successes plus the candidate: **all 4,467
passed** native Rust and generated Go in 947.42 seconds, without failure or
timeout. The baseline contains 2,661 coretests, 1,478 alloctests, 326 internal
alloc cases and both auxiliary targets. Context, discovery and all previous
successes are retained; 193 cases were unselected and two remain ignored.
The report is `.cache/upstream/arm64/simd-unary-final-report.json`.
Native amd64 replay of this checkpoint remains unverified.

## Auto-trait-only object metadata (M12)

The pin_macro::unsize_coercion upstream case now passes native Rust and
generated Go. The exporter previously requested a vtable only when a trait
object had a principal trait. A dyn Send or dyn Send + Sync object has no
principal, but still needs Rust's actual drop/size/alignment metadata. The
exporter now asks rustc for the VTable with its optional principal intact;
existing Go layout, borrowing and drop lowering are reused.

The DST differential fixture now verifies thirteen cases x six inputs = 78
records. New cases cover shared auto-trait borrows, aligned Box<dyn Send>,
Box<dyn Send + Sync>, nested auto-trait DST tails, ordinary-trait to auto-trait
upcasts, zero-sized Send/Sync borrows and actual vtable-driven destruction.
Rust size/alignment 32/64 and exactly-once drops are checked. Both targets
compile; arm64 executes native comparisons and raw-zero Go-allocation/frame
checks. Default Go tests and both-target compiler metadata checks also pass.

Promotion reran all 4,465 M11 successes plus this candidate: **all 4,466
passed** native Rust and generated Go in 921.35 seconds, with no failure or
timeout. The baseline comprises 2,660 coretests, 1,478 alloctests, 326 internal
alloc cases and both auxiliary targets. Context, discovered IDs and all
previous successes are retained; 194 unselected cases and two ignored cases
are outside the passing set. Evidence:
`.cache/upstream/arm64/auto-trait-final-report.json`,
`.cache/upstream/arm64/auto-trait-scan-report.json` and
`.cache/dst-conformance/`. Native amd64 execution of this checkpoint remains
unverified.

## C math entries and algebraic float operations (M11)

A completed promotion passes **4,465 cases**, retaining all 4,453 M10
successes and recovering twelve previously failing f32/f64 core tests: acosh,
asinh, atanh, gamma, ln_gamma and to_algebraic. The C ABI table calls the
pinned modernc libc implementations for acosh/asinh/cosh/tgamma/lgamma_r,
including their f32 entry points. The reentrant log-gamma boundary passes
the original signed-int output pointer directly. Existing log1p lowering
also unblocks the atanh tests. No upstream test is modified.

Algebraic add/subtract/multiply/divide/remainder reuse the existing typed
scalar operations. Rust permits algebraic optimizations; this implementation
chooses ordinary per-operation IEEE rounding without enabling reassociation
or fusion. The generic numeric differential fixture now has 99 cases x 109
inputs = 10,791 records, and checks all five operations at both precisions.
Generated code compiles on amd64 and arm64; arm64 compares native Rust and
checks raw-zero Go allocations and restored automatic storage.

Runtime gamma probes verify both signs of zero, infinities, invalid integer
arguments and fractional values. Reentrant f32/f64 calls independently check
sign output and adjacent-byte guards. All ten C helpers have raw-zero warmed
Go allocations and unchanged Context frames. `go test ./...`, fresh libc
differential tests and both-target compiler metadata checks pass.

All 4,465 cases pass fresh native Rust, MIR export, Go build and execution in
877.02 seconds, with no failure or timeout. The passing set now comprises
2,659 coretests, 1,478 alloctests, 326 internal alloc cases and both auxiliary
targets. Context, discovery inventory and every earlier success are retained.
The 195 unselected cases are outside this verified set; both ignored cases
remain ignored. Evidence: `.cache/upstream/arm64/math-entry-final-report.json`,
`.cache/upstream/arm64/math-entry-cosh-scan-report.json` and
`.cache/numeric-conformance/`. Native amd64 replay of the new cases remains
unverified.

## Anonymous constant pooling (M10)

The completed M10 passing baseline contains **4,453 cases**: 2,647 coretests,
1,478 alloctests, 326 internal alloc cases and both auxiliary targets. A fresh
promotion reran every M9 success and the three remaining alloc execution failures:
`str::const_str_ptr`, `task::test_waker_will_wake_clone` and
`task::test_local_waker_will_wake_clone`. The exporter follows rustc's
pooling of fully initialized anonymous read-only allocations by their bytes
and provenance, with the maximum required alignment. Named statics and
mutable allocations retain separate identity. The generic allocator/runtime
alias format is reused; there is no test-specific pointer substitution.

All 4,453 passed native Rust and generated Go in 925.95 seconds, with no
failure or timeout. Every earlier success, context field and discovered ID is
preserved. The report marks 207 unselected cases as not_run and preserves two
ignored cases; it does not reclassify the four M9 unsized-FnOnce emit failures.

The numeric fixture now covers 89 cases x 109 inputs = 9,701 records. Native
probes check pooled constant pointers, shared alignment 16, distinct identical
named statics and independently writable mutable statics. Generated code for
both targets compiles; arm64 executes native comparisons and raw-zero
Go-allocation/frame checks. `go test ./...` and both-target compiler metadata
checks also pass with the pooling implementation.
Evidence: `.cache/upstream/arm64/constant-pool-probe-report.json`,
`.cache/upstream/arm64/constant-pool-final-report.json` and
`.cache/numeric-conformance/`. Native amd64 execution remains unverified.

## Intrinsic runtime bodies and math entries (M9)

A completed promotion verifies **4,450 passing cases**, preserving all 2,584
M8 successes and adding 1,866 cases. It executes 68 additional `coretests`
and all 1,808 main/internal alloc tests against native Rust and generated Go.
The run took 1,344.49 seconds and reports 4,450 passed, four emit failures,
three execution failures, 203 intentionally unselected core cases and two
upstream ignored cases. Its report is
`.cache/upstream/arm64/intrinsic-coverage-final-report.json`.
Earlier interrupted measurement reports are diagnostic evidence only.

| Suite | Passing cases |
| --- | ---: |
| coretests | 2,647 |
| alloctests | 1,475 |
| alloctests-internal | 326 |
| allocation-error auxiliary targets | 2 |

All alloc cases have measured outcomes after the allocator-alias and intrinsic
repairs. The four emit failures involve unsized Box<dyn FnOnce> values in
threaded Arc/linked-list tests and require broader lowering work. The three
execution failures concern constant pointer identity and are addressed by M10.

| Small repair | Additional verified core cases |
| --- | ---: |
| Runtime const_allocate/const_deallocate, using Rust's supplied MIR bodies | 2 |
| Carryless multiplication, using Rust's supplied MIR bodies | 12 |
| Funnel shifts, including zero shift and overflow panic, using Rust MIR | 20 |
| NaN-propagating minimum/maximum, using Rust MIR | 4 |
| float_to_int_unchecked for valid primitive inputs | 6 |
| Missing truncf64 dispatch | 3 |
| round_ties_even f32/f64 dispatch | 2 |
| exp/exp2/log10 dispatch | 16 |
| Fused f64 multiply-add, using math.FMA | 1 |
| C ldexp/ldexpf signatures and scaling | 2 |

The scalar math mappings preserve Rust's documented special values and
unspecified transcendental precision; they do not claim bit-identical
transcendental results from the native C math library. Expm1/log1p entries
also restore the two measured Zipf sort cases. No upstream assertion or input
size is changed. Required intrinsic bodies must be available from the pinned
compiler; there is no guessed body or compiler-version compatibility path.

The expanded numeric fixture verifies 85 cases x 109 inputs = 9,265 records,
including signaling/quiet NaNs, signed zero, subnormals, half-way rounding,
primitive conversion bounds, 8/16/32/64/128-bit funnel and polynomial
operations, fused f64 arithmetic and C scaling at extreme integer exponents.
The ldexp probes also compare errno after overflow/underflow and preserve
an existing errno on non-error paths. Both targets compile; arm64 executes
the native comparisons and raw-zero Go-allocation/frame checks. Current
source fingerprints and successful exports are in `.cache/numeric-conformance/`.

## Upstream standard-library regression repairs (M8)

At the completed M8 checkpoint, the Linux arm64 baseline contained
**2,584 passing cases**, up from
2,507. A fresh `promote --select ... --jobs 4` ran every previous success and
77 previously failing candidates through native Rust, fresh MIR export, Go
compilation and Go execution. All 2,584 passed in 483.68 seconds, with no
failures or timeouts. Compiler/source/dependency/adapter context and all 4,662
discovered IDs are unchanged; no previous passing entry was removed.

| Small repair | Recovered upstream cases |
| --- | ---: |
| Atomic min/max update direction and narrow signed argument truncation | 8 |
| Boolean ordering (`false < true`) | 1 |
| Guarded ZST pointer-distance paths: prevent Go compile-time division by zero | 9 |
| 128-bit bit counts, rotations, min/max, exact division and wide shift operands | 53 |
| Closure-to-function-pointer coercion: export rustc's FnOnce shim | 3 Unicode case-conversion tests |
| Compiler-marked allocator declarations: normalize inline/re-exported alloc paths | 3 |

The passing set comprises 2,579 `coretests`, two main `alloctests`, one
`alloctests-internal` and both allocation-error auxiliary targets. The allocator
alias blocker was resolved for the three selected alloc cases; the other 1,805
main/internal alloc cases had not been rerun at that checkpoint. The report
marks 2,076 unselected cases as `not_run` and preserves two ignored cases;
this checkpoint does not establish a new full-inventory failure count.

Validation also includes `go test ./...`, fresh Rust language/allocator/callback
gates and both-target compiler metadata checks. The numeric differential
fixture covers 48 cases × 109 inputs = 5,232 records, including bool ordering,
128-bit boundaries and a noncapturing closure returning an array. Generated
amd64 and arm64 code compiles; arm64 executes against native Rust and checks
raw zero Go allocations/bytes over warmed calls. Runtime tests additionally
check atomic old-value returns and adjacent bytes at all four widths, plus
128-bit counts/rotations against `math/big`. These helpers need no heap storage.
Native amd64 execution of these new upstream cases remains unverified.

Evidence: `.cache/upstream/arm64/simple-fixes-report.json`,
[passing baseline](fixtures/upstream/baseline-linux-arm64.json) and
`.cache/numeric-conformance/`. The implementation changes general compiler
metadata, lowering and runtime operations; upstream test bodies, assertions,
input sizes and the fixture adapter are unchanged.

## Static Go type API (M7)

The current change removes `oxide.Type`, `oxide.Value`, runtime field / variant
lookup and callback tables. Sized Rust types have nominal `Rust__Name` layouts
and `Value__Name` owners; `Ref__Name` / `Mut__Name` provide typed views, including
metadata-bearing views for unsized types. Private
zero-sized markers reject both implicit misuse and explicit cross-type or
borrow/owner conversions, with compile-time 8/16-byte handle size assertions.
True Rust aliases retain one Go identity. Fields, enum payloads, containers,
formatting, serde and iterator operations have concrete signatures and call
compiler-selected Rust instances directly.

| Gate | linux/arm64 | linux/amd64 |
| --- | --- | --- |
| Generic values / ownership / serde / replacement | 14 / 44 / 6 / 5 cases passed, 100 raw-zero calls per case | same 69 cases passed on native hardware |
| Function-pointer callbacks | RustCall versus ordinary tuple ABI: 16 cases × 100 calls passed | native Rust and generated Go passed the same 16 cases |
| External static type checking | one positive consumer and 17 negative consumers passed | the same 18 consumers passed |
| Renderer source and default vet | fresh compiler export, 973 roots, 402 generated Go files; passed | fresh export, 973 roots, 405 Go files; compile/link and default vet passed |
| Renderer API / allocations / ownership | all 74 native byte comparisons and 222 raw-zero allocation windows passed; Rust, libc, file-map and frame baselines restored | same 74 cases / 222 raw-zero windows and all three ledgers passed |
| Renderer images / lifecycle | 72 inputs / 144 native SVG/PNG files; 8 owned returns / 24 raw windows; 18 exact-allocation windows; 4 chaos epochs / 48 calls passed | same complete image/lifecycle matrix passed |
| CLI and documentation example | 20 process comparisons passed; tutorial SVG/PNG equal native bytes | same 20 process cases and native tutorial bytes passed |

The 85 small cases on each architecture use raw `MemStats.Mallocs` and
`TotalAlloc` deltas, not rounded averages. Independent zero-sized owners,
packed and align-64 values, private DST tails, `Box<dyn Trait>`, 256 KiB moves,
old-destructor panic, RHS mutation and real double-panic abort (134) remain
covered. Additional generated tests verify heap-slot versus payload ownership,
allocator mapping-failure rollback, niche wrap, signed/high-128-bit enum tags,
non-exhaustive variants and void `Default` alias forwarding.

A packed DST can have minimum alignment 1 but a vtable-required alignment 64.
Its shared field accessor now checks the actual child alignment. A native Rust
layout probe and a negative control that removes only this check demonstrate
the failure. Function pointers require compiler-provided `fn_spread_arg`: `-1`
for ordinary parameters or the final tuple index for RustCall. Missing metadata
is rejected. Callbacks use named `ABI__Name` / `Callback__Name` aliases without
embedding compiler IDs in client code. Scalar and reference boundaries avoid
extra Context frames and temporary fat-pointer allocations.

Final small-suite evidence: `.cache/static-final-gates/summary.json`.
ARM renderer evidence: `.cache/static-api/renderer-api-arm64/validation-verified.json`
and `.cache/static-api/arm64-final-results.json`. AMD evidence is
`.cache/static-api/amd64-final/gate-results.json`; the combined record is
`.cache/static-api/summary.json`. The final API suites take 5.732 / 18.832 s
on arm64 / amd64; the combined image/lifecycle batches take 221.073 /
633.178 s. API inventory mutation checks
are in `.cache/static-api/api-coverage-negative.log`. The default `go test ./...`
passes in `.cache/static-api/default-final-fresh.log`. The final renderer export
contains no public function-pointer types; a fresh export confirms the new ABI
metadata contract without relying on older interchange files. Final frontend
SHA256 is `c10f6db630dafc9a45a30418f4a78a3e4fa1301ad28e4fd6626a61073aec6ac8`
on arm64 and `368ac5749b75156e80eba6a145632b7a1e239037779e8f359300b62804afcb80`
on amd64. MIR SHA256 is
`fbf71d837ccb10dacab9a5aa4c82bc87c5ed11c79b903e9f9c6f2bd940337ba3` /
`040285883f901f1d80870192afc5ae41238a9e4eeef4242676d5a7cd6e3d2b02`, respectively.

Go cannot enforce linear ownership or all borrow lifetimes. Borrowed byte
slices preserve the caller's Rust mutability obligations. Go layout values have
at most alignment 8; typed storage uses the actual Rust alignment. Context
storage models addressable automatic storage, without claiming identical
physical native-stack placement or zero cold-start/Rust-heap allocation.
The standard-library scope remains the tested subsets listed below. M7 repeats
the direct-API and renderer acceptance with static interfaces and adds the
callback ABI gate; it does not claim all Rust language or library support.
Historical M6 evidence follows separately.

## Direct library API (M6)

Historical completed checkpoint: `e98931f`. The renderer façade is removed from the translation input. Go
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
| M7: static Go type API | verified subset on both native targets | concrete layout/view/owner types, 17 compile rejection cases, 85 differential cases, full renderer/API/CLI replay; no descriptor dispatch or compatibility API |
| M6: direct Go use of Rust libraries | verified subset at `e98931f` on both native targets | upstream manifest without façade; Go constructs/borrows/drops Rust values; complete renderer matrix and generic type/ownership/JSON/SIMD differential gates pass |
| M8: pinned upstream standard-library regressions | 2,584 passing cases on native linux/arm64 | all 2,507 previous successes retained; 77 small-failure cases recovered; complete inventory and native amd64 execution remain open |
| M9: intrinsic bodies and math entries | 4,450 passing cases on native linux/arm64 | all 1,808 alloc cases measured; 68 additional core cases and 1,798 additional alloc cases pass; four emit and three execution failures remain |
| M10: anonymous constant pooling | 4,453 passing cases on native linux/arm64 | all 4,450 M9 successes retained; three pointer-identity failures recovered; shared alignment and independent static/mutable identity verified; 9,701 numeric records pass |
| M11: C math and algebraic floating operations | 4,465 passing cases on native linux/arm64 | all 4,453 M10 successes retained; twelve failures recovered; actual libc math entries, reentrant sign-pointer guards, 10,791 numeric records and raw-zero helper allocations pass |
| M12: auto-trait-only objects | 4,466 passing cases on native linux/arm64 | all 4,465 M11 successes retained; auto-trait-only unsizing restored; 78 DST records, aligned/nested/ZST metadata and Box destruction verified |
| M13: SIMD unary operations | 4,467 passing cases on native linux/arm64 | all 4,466 M12 successes retained; SIMD neg/fabs restored; bit patterns, integer minimum values, aliasing/padding and 11,663 numeric records verified |
| M14: Coroutine discriminants | 4,470 passing cases on native linux/arm64 | all 4,467 M13 successes retained; three join failures restored; compiler-derived state tags, nested await, pending cancellation and 12,099 differential records verified |
| M15: C variadic formatting | 4,471 passing cases on native linux/arm64 | all 4,470 M14 successes retained; real printf/snprintf variadic calls, 27 libc records and raw-zero indirect argument storage verified |
| M16: single-rounding f32 FMA | 4,472 passing cases on native linux/arm64 | all 4,471 M15 successes retained; complete 4,662-case audit; f32 FMA restored; 12,644 numeric records and 52,820 exact finite comparisons verified; 180 generation failures, eight native issues and two ignored cases remain |
| M17: f16/f128 | 4,636 passing cases on native linux/arm64 | all 4,472 M16 successes retained plus 164 additions; exact layouts, arithmetic/FMA/sqrt/casts, 15,136 differential records and raw-zero allocations; correct native reference selector and validated context migration; 13 binary128 transcendental cases require an explicit math provider |
| M18: binary128 math | 4,649 passing cases on native linux/arm64 | all 4,636 M17 successes retained plus the 13 remaining math cases; built-in fixed-precision math, 15,612 MPFR records within one ULP, 1,800 method records and 15,136 core float records on both native targets, full-range trigonometric reduction and raw-zero Go allocations |

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
| floating point | verified subset | f16/f32/f64/f128 layouts, arithmetic and casts through i128/u128; single-rounding FMA for all four widths, including LLVM #98389; binary128 uses fixed-width software arithmetic and wider intermediates for built-in transcendental math; explicit per-MIR-operation rounding prevents unintended fusion; exhaustive IEEE/math conformance remains open |
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
| async/coroutines | verified manual-poll subset on arm64 | compiler discriminants/variant offsets and actual lowered MIR; three join tests plus completion/nested-await/cancellation Drop probes pass; general executor and native threading contracts remain unverified |
| SIMD | partial | compiler vector layouts and required lane operations lower; integer pack, multiply/add, shuffle, shift and carryless multiply helpers have unit tests; actual SSE compare/min/max/conversion/reciprocal helpers pass native amd64 tests; comprehensive SIMD semantics remain unverified |
| inline assembly | explicit templates only | complete stdarch CPUID template with checked operands/options invokes actual CPUID on amd64; fully parsed comment-only same-place identity in/out invokes a compiler barrier; unknown templates fail compilation |
| volatile memory accesses | unsupported | rejected explicitly |

## Standard library support

A row covers only the explicitly tested APIs; translating a generic body is
not evidence that an entire module is supported.

| Library/API family | Status | Tested subset / remaining boundary |
| --- | --- | --- |
| `core::mem` | verified subset | size_of, align_of, offset_of, equal-size transmute; size_of_val/align_of_val for aligned/nested/packed DSTs and packed ManuallyDrop tails; broader MaybeUninit/ManuallyDrop cases pending |
| `core::ptr` | verified subset | address-of, read/write, add, read_unaligned/write_unaligned and pointer/integer casts; guarded ZST pointer-distance paths compile and preserve Rust's checks; volatile unsupported |
| `core::num` / primitive integer methods | verified subset | wrapping operations, nonzero niches, 128-bit bit counts/rotations/min/max/exact division/remainder/casts and wide shift operands, signed/unsigned carrying_mul_add at 8/16/32/64/128 bits and disjoint_bitor |
| `core::f16`, `core::f128` | verified numeric subset | exact value ABI/storage, arithmetic/comparisons, FMA, sqrt, integral rounding, casts, sign operations and integer powers; real core MIR supplies remaining methods and formatting/parsing; both targets compile, native arm64 and exact-arithmetic references verify the covered subset |
| `std::f16`, `std::f128` | verified math subset | built-in exp/log, trig/inverse-trig, hyperbolic/inverse-hyperbolic, cbrt, hypot, powf, gamma/lgamma and erf/erfc; 30 method paths and 1,800 records pass on both native targets; 15,612 MPFR cases check binary128 precision and exceptional values; optional `Context.Binary128Math` overrides the defaults |
| `core::f32`, `core::f64` | verified numeric subset | fused multiply-add, integer conversions, explicit per-operation rounding and selected math entries; f32 FMA halfway/subnormal/overflow cases pass native arm64 differential and exact-arithmetic checks |
| `core::char` / `core::unicode` | verified upstream subset on arm64 | to_lowercase, to_uppercase and to_casefold tests pass unchanged, including full Unicode input traversal; broader API support is limited to the passing baseline |
| `core::option` | verified subset | NonZeroU64/U128 representation and match; Go named-variant construction and String ownership use actual compiler tags/niches; complete Option API not claimed |
| `core::result` | verified subset | catch results and dropping Err<Box<dyn Any + Send>> execute with native-equivalent payload destruction; direct Go Results preserve nested owners, borrowed results and serde errors; broader Result API coverage pending |
| `core::array`, `slice`, `str` | partial | array and slice storage/projections; broad iterator/UTF-8 suites pending |
| `core::ops`, `marker`, `convert`, `clone`, `cmp` | partial | rustc resolves concrete instances; bool ordering and 128-bit shift-operator cases pass the arm64 upstream gate; complete API coverage remains open |
| `core::fmt`, `core::error`, `core::any` | verified subset | HRTB/dyn metadata, TypeId equality, mixed string/u64 formatting callbacks and dynamic payload downcasts; generic Go Debug/Display execute compiler-resolved Rust formatting, including Box<dyn Debug>; complete formatting/error APIs remain unverified |
| `core::cell`, `pin`, `iter` | verified upstream subset on arm64 | the passing baseline includes 39 cell, seven pin/pin_macro and 289 iterator cases; complete APIs and native amd64 replay remain unverified |
| `core::borrow` / `alloc::borrow` | verified alloc-test subset on arm64 | six borrow and five cow_str cases pass; complete borrowing/lifetime API coverage is not claimed |
| `core::sync::atomic` | verified single-thread subset | AtomicUsize hook/drop counters and relaxed load/store/fetch operations; eight upstream signed/unsigned min/max cases pass on arm64, with runtime boundary/old-value/adjacent-byte checks at 1/2/4/8-byte widths; memory-order and multithreaded conformance pending |
| `core::arch` / CPU feature detection | verified runtime subset | actual CPUID/XGETBV and required SSE/AVX helpers pass on native amd64 hardware; Rust feature checks remain reachable, no hardcoded CPU capabilities; the complete translated renderer is checked separately |
| `alloc::alloc` | verified subset | allocator primitives and fresh Rust allocation call chains; transparent Alignment/NonNull ABI parameters; compiler-marked allocator declarations survive inline/re-exported paths, with three main/internal alloc cases passing on arm64 |
| `alloc::boxed`, `vec`, `string` | verified subset | Box creation/drop, Vec growth/push/reverse, String creation/push_str/drop; direct Go String/Vec construction, moves, field replacement, Box dereference, manual header allocation/free and in-place Rust Drop; both-target 14-case type API and 44-case ownership gates retain raw zero Go allocations |
| `alloc::collections` | verified upstream subset on arm64 | 262 internal collection cases, 41 main collection cases, 116 VecDeque and 438 sort cases pass; the linked-list threaded send test remains blocked by unsized FnOnce lowering |
| `alloc::rc` | verified upstream subset on arm64 | 65 Rc/Weak cases pass; LocalWaker constant identity is checked separately by M10 |
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
| `std::future`, `task`, async | verified polling subset on arm64 | three join cases and manual Pending/Ready/cancellation lifetime probes pass; no general executor acceptance or native multithreaded scheduling guarantee |
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
