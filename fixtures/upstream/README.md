# Upstream Rust regression tests

This fixture runs the pinned Rust `coretests` and `alloctests` through Oxide and
records successful cases as a regression baseline. A previously successful case
must continue to pass when Oxide changes. Unknown failures remain visible in
scan reports without weakening the existing baseline.

The source staging and dependency locking follow
[rustc_codegen_jvm's standard-library harness](https://github.com/IntegralPilot/rustc_codegen_jvm/blob/55442bb720fd1450b4a386ba0fe218e3cc5f3bba/stdlib_test_harness.py).
The fixture uses the `rust-src` installed with Oxide's compiler, pinned by
[`upstream.lock.json`](upstream.lock.json) to nightly `2026-09-15`, Rust commit
`574ff7d98bd6d037e5236a8453029173b32631fd`. It does not fetch the changing Rust
`main` branch during a test run.

## Current passing baseline (2026-10-01)

M18 records **4,649 passing cases**, retaining every M17 success and adding
the 13 remaining binary128 math cases. They now use built-in pure Go math
with fixed-size wider intermediates. The completed promotion takes
1,787.90 seconds, with no regression or timeout. Its report is
`.cache/upstream/arm64/f128-math-promotion-report.json`; context and discovery
are unchanged from M17.

| Suite | Discovered | Passed | Not selected | Ignored |
| --- | ---: | ---: | ---: | ---: |
| coretests | 2,852 | 2,843 | 7 | 2 |
| alloctests | 1,481 | 1,478 | 3 | 0 |
| alloctests-internal | 327 | 326 | 1 | 0 |
| c-str-alloc-error | 1 | 1 | 0 | 0 |
| vec-deque-alloc-error | 1 | 1 | 0 | 0 |
| **Total** | **4,662** | **4,649** | **11** | **2** |

The unselected cases are the four previously diagnosed by-value unsized FnOnce
failures and seven native slice timeouts. All 177 f16/f128 candidates from
M16 now pass. Their bodies, assertions and input sizes remain unchanged.
Dedicated float suites additionally execute on native amd64, with the same
pinned compiler, checking core operations, every math-method path and zero Go
allocations. A 15,612-record MPFR corpus verifies binary128 math within one ULP.

### Completed M17 measurement

M17 records **4,636 passing cases**, retaining all 4,472 M16 successes and
adding 164 f16/f128 cases. Every selected case executes native Rust, fresh
export, Go build and generated Go. The completed run takes 1,763.22 seconds,
with no regression or timeout. Its report is
`.cache/upstream/arm64/soft-float-promotion-report.json`.

| Suite | Discovered | Passed | Not selected | Ignored |
| --- | ---: | ---: | ---: | ---: |
| coretests | 2,852 | 2,830 | 20 | 2 |
| alloctests | 1,481 | 1,478 | 3 | 0 |
| alloctests-internal | 327 | 326 | 1 | 0 |
| c-str-alloc-error | 1 | 1 | 0 | 0 |
| vec-deque-alloc-error | 1 | 1 | 0 | 0 |
| **Total** | **4,662** | **4,636** | **24** | **2** |

The separate 177-case float scan passes 164 and identifies 13 cases requiring
`Context.Binary128Math`: binary128 transcendental math has no built-in
implementation and fails explicitly without a provider. The four by-value
unsized FnOnce failures and seven native slice timeouts from M16 were not
selected in M17. Source bodies, assertions and input sizes are unchanged.
Evidence is `.cache/upstream/arm64/soft-float-scan-report.json`.

Native reference builds now add `-Cllvm-args=-global-isel=0` to avoid the pinned
AArch64 debug backend's f16 FMA double rounding (LLVM #98389) and f16-to-i128
narrowing. Debug assertions and MIR optimization settings remain unchanged.
The changed flags are recorded in baseline context. The migration preserves
all prior successes and identical discovery, after replaying every retained
case; `.cache/upstream/arm64/soft-float-baseline-migration.json` records the
before/after hashes and exact context change. Compiler/source/dependency
upgrades still require explicit baseline migration.

The dedicated float fixture compares 15,136 records against SelectionDAG debug
and optimized native references, plus independent exact-arithmetic tests and
raw-zero warmed Go allocations. Native execution is arm64; generated code and
metadata compile for both targets. Native amd64 replay remains unverified.

### Completed M16 measurement

The completed M16 promotion measures **all 4,662 cases**, records **4,472**
successes and retains every M15 success. The only added passing case is
num::floats::mul_add::test_f32, restored with a single-rounding f32 FMA helper.
All passing cases execute native Rust, fresh export, Go build and generated Go.
The run takes 8,069.71 seconds; context and discovery are unchanged. Its report
is `.cache/upstream/arm64/fma32-full-report.json`, with no unselected cases.

| Suite | Discovered | Passed | Generation failed | Native failed | Native timeout | Ignored |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| coretests | 2,852 | 2,666 | 176 | 1 | 7 | 2 |
| alloctests | 1,481 | 1,478 | 3 | 0 | 0 | 0 |
| alloctests-internal | 327 | 326 | 1 | 0 | 0 | 0 |
| c-str-alloc-error | 1 | 1 | 0 | 0 | 0 | 0 |
| vec-deque-alloc-error | 1 | 1 | 0 | 0 | 0 | 0 |
| **Total** | **4,662** | **4,472** | **180** | **1** | **7** | **2** |

All 176 core generation failures require f16/f128 ABI support; the four alloc
failures require by-value unsized FnOnce support. The native f16 FMA failure
and seven native slice timeouts remain as described in the historical full
measurement below. No generated-Go execution failure or baseline regression
occurs. Source bodies, assertions and input sizes remain unchanged.
Native execution is arm64; compiler metadata and generated-code compilation
cover both targets. The numeric fixture verifies 12,644 records, including
FMA halfway/subnormal/overflow cases and raw-zero warmed Go allocations.
Native amd64 replay of the new checkpoints remains unverified.

### Completed M15 measurement

The completed M15 promotion records **4,471** passing cases: 2,665 coretests,
1,478 main alloctests, 326 internal alloc cases and both auxiliary targets.
It retains every M14 success and recovers ptr::test_variadic_fnptr using a
real libc printf boundary. All pass native Rust, fresh export, Go build and
execution in 995.99 seconds, without failure or timeout. Context and inventory
are unchanged; the report is `.cache/upstream/arm64/printf-final-report.json`.
It marks 189 cases unselected and preserves two ignored cases. Native execution
is arm64; metadata/generated-code compilation cover both targets. Actual
variadic formatting and zero Go allocations are checked by 27 libc records.

### Completed M14 measurement

The completed M14 promotion records **4,470** passing cases: 2,664 coretests,
1,478 main alloctests, 326 internal alloc cases and both auxiliary targets.
It retains every M13 success and restores three join tests by exporting the
compiler's Coroutine discriminant values. All pass native Rust, fresh export,
Go build and execution in 944.43 seconds, without failure or timeout. The
report is `.cache/upstream/arm64/coroutine-tag-final-report.json`; it preserves
the context and inventory and marks 190 cases unselected plus two ignored.
Native execution is arm64; metadata/generated-code compilation cover both
targets. The numeric/coroutine fixture verifies 12,099 records and raw-zero
warmed Go allocations, including nested await and cancellation Drop traces.

### Completed M13 measurement

The completed M13 promotion records **4,467** passing cases: 2,661 coretests,
1,478 main alloctests, 326 internal alloc cases and both auxiliary targets.
It retains every M12 success and recovers coretests/simd::testing by lowering
vector negation and floating absolute value. All pass native Rust, fresh export,
Go build and execution in 947.42 seconds. Context and inventory are unchanged.
The report is `.cache/upstream/arm64/simd-unary-final-report.json`; 193 cases
remain unselected and two remain ignored. Native execution is arm64; metadata
and generated-code compilation cover both targets. The corresponding numeric
suite has 11,663 records and raw-zero warmed Go allocations.

### Completed M12 measurement

The completed M12 promotion records **4,466** passing cases: 2,660 coretests,
1,478 main alloctests, 326 internal alloc cases and both auxiliary targets.
It retains every M11 success and recovers pin_macro::unsize_coercion using
compiler-provided auto-trait-only vtables. All pass native Rust, fresh export,
Go build and execution in 921.35 seconds, with unchanged context and discovery
inventory. The report is `.cache/upstream/arm64/auto-trait-final-report.json`;
it marks 194 unselected cases not_run and preserves two ignored cases.
Native execution is arm64; metadata and generated-code compilation cover
both targets. The associated 78-record DST differential suite checks alignment,
size, destruction and raw-zero warmed Go allocations.

### Completed M11 measurement

The completed M11 promotion records **4,465** passing cases: 2,659 coretests,
1,478 main alloctests, 326 internal alloc cases and both auxiliary targets.
It retains every M10 success and adds twelve C-math/algebraic-float cases.
All pass native Rust, fresh export, Go build and execution in 877.02 seconds,
with unchanged context and discovery inventory. The report is
`.cache/upstream/arm64/math-entry-final-report.json`; it marks 195 unselected
cases not_run and preserves two ignored cases. Native execution is arm64;
compiler metadata and generated-code compilation cover both targets.

### Completed M10 measurement

The completed M10 promotion records **4,453** passing cases: 2,647 `coretests`,
1,478 main `alloctests`, 326 internal alloc cases and both auxiliary targets.
All 4,450 M9 successes are retained and three constant pointer-identity cases
are recovered by pooling anonymous read-only allocations. Every recorded case
passes fresh native Rust, MIR export, Go build and execution in 925.95 seconds.
The report is `.cache/upstream/arm64/constant-pool-final-report.json`; its 207
not_run cases comprise the four known M9 generation failures and 203 core
cases outside this selection. Two upstream ignored cases remain ignored.
Context and discovery inventory are unchanged. Native amd64 execution of
this checkpoint remains unverified; compiler metadata and generated-code
compilation cover both targets.

### Completed M9 measurement

The completed M9 promotion records **4,450** passing cases: 2,647 `coretests`,
1,475 main `alloctests`, 326 internal alloc cases and both auxiliary targets.
It retains every one of M8's 2,584 successes, with unchanged compiler/source/
dependency/adapter context and discovery inventory. All 1,808 main/internal
alloc cases were measured, leaving four emit failures involving unsized FnOnce
values and three constant pointer-identity execution failures. The run took
1,344.49 seconds; its report is
`.cache/upstream/arm64/intrinsic-coverage-final-report.json`. Its 203 `not_run`
cases are outside the selected core set, and both upstream ignored cases remain
ignored. M10 subsequently recovers all three pointer-identity cases and reruns
every success. See
[MILESTONES.md](../../MILESTONES.md) for repair counts, numeric/allocation checks
and native amd64 verification limits.

### Earlier M8 checkpoint

At M8, the [Linux arm64 baseline](baseline-linux-arm64.json) recorded **2,584**
passing cases: 2,579 `coretests`, two main `alloctests`, one internal alloc case
and both allocation-error auxiliary targets. A fresh selected `promote --jobs 4`
reran all 2,507 previous successes plus 77 failing candidates; **all 2,584
passed** native Rust and generated Go in **483.68 seconds**. Baseline history
validation confirms that every previous success and the entire context and
discovery inventory are preserved. The report is
`.cache/upstream/arm64/simple-fixes-report.json`.

The small fixes cover atomic min/max, bool ordering, guarded ZST
pointer-distance paths, 128-bit integer operations, closure function-pointer
reification and compiler-marked allocator aliases. The latter resolves the
shared support blocker for the three selected main/internal alloc cases.
Their other 1,805 cases remain unmeasured after the repair. The promotion
reports 2,076 unselected cases as `not_run` and preserves two upstream ignored
cases; no new full-inventory failure count is claimed. See
[MILESTONES.md](../../MILESTONES.md) for the repair counts and separate numeric
and allocation checks. Native execution here is Linux arm64; the additional
amd64 evidence is compiler metadata and generated-code compilation.

## Scope and historical full measurement (2026-09-29)

Five test targets are discovered and reported separately:

| Case ID prefix | Upstream entry point |
| --- | --- |
| `coretests/` | `library/coretests/tests/lib.rs` |
| `alloctests/` | `library/alloctests/tests/lib.rs` |
| `alloctests-internal/` | `library/alloctests/lib.rs`, including tests in `library/alloc` |
| `c-str-alloc-error/` | `library/alloctests/tests/c_str_alloc_error.rs` |
| `vec-deque-alloc-error/` | `library/alloctests/tests/vec_deque_alloc_error.rs` |

The runner supports the host Linux `arm64` or `amd64` target in the debug
profile, with overflow checks enabled. Native Rust and generated Go execute on
the same host. Baselines are separate files, `baseline-linux-arm64.json` and
`baseline-linux-amd64.json`; results from one architecture do not establish
support on the other. Compiler discovery lists 4,662 cases on Linux arm64 across
these targets. This is the discovery total, not a passing count. Linux amd64
and release execution are not claimed as verified.

The full measurement completed on **2026-09-29**, using Oxide
implementation commit `10820d7` and the Rust source pin above. Every discovered
case has a recorded outcome; there are no `not_run` entries. Of the 4,662 cases,
2,507 passed both native Rust and generated Go:

| Target | Discovered | Passed | Blocked | Other results |
| --- | ---: | ---: | ---: | ---: |
| `coretests` | 2,852 | 2,505 | 0 | 347 |
| `alloctests` | 1,481 | 0 | 1,481 | 0 |
| `alloctests-internal` | 327 | 0 | 327 | 0 |
| `c-str-alloc-error` | 1 | 1 | 0 | 0 |
| `vec-deque-alloc-error` | 1 | 1 | 0 | 0 |
| **Total** | **4,662** | **2,507** | **1,808** | **347** |

The remaining 347 `coretests` outcomes are 296 `emit_failed`, 30
`go-build_failed`, 11 execution `failed`, 1 `native_failed`, 7 `native_timeout`
and 2 upstream `ignored`. The execution failures comprise eight atomic min/max
cases and three Unicode case-conversion cases. Main `alloctests` and
`alloctests-internal` have no passing cases: their common support code blocks
Go generation for all 1,808 cases, with evidence recorded as described below.
Both separate allocation-error targets pass. A completed measurement does not
mean that every case reached generated-Go execution.

The native failure is `coretests/num::floats::mul_add::test_f16`, whose native
result differs from the expected value by one representable step
(`0x3000` versus `0x3001`). The seven native timeouts comprise six
`slice::split_off[_mut]` cases involving `usize::MAX` zero-sized elements and
`slice::select_nth_unstable`. These retain the upstream assertions and input
sizes and exceeded the 30-second limit in the unoptimized debug profile.
Native-reference failures and timeouts are recorded separately from Oxide
execution failures.

The full report is `.cache/upstream/arm64/full-corrected-report.json`. Its
2,507 successful case IDs retain all 1,180 previously recorded baseline cases
and all 1,261 successes observed earlier. At that checkpoint, the
[Linux arm64 baseline](baseline-linux-arm64.json) contained exactly those
2,507 successes. A subsequent independent `check --jobs 4` rebuilt that
frontend and translator, rediscovered all five targets, and reran every
recorded case: **all 2,507 passed**, with exit status 0, in **524.9 seconds
(about 8.7 minutes)**. Both runs executed native Rust and generated Go on Linux
arm64. The verification report is
`.cache/upstream/arm64/full-verification-report.json`; its `not_run` entries are
the unselected cases outside the passing baseline.

The fixture changes only test integration files. Staged copies retain every upstream
test body, assertion and test size. The fixture replaces test attribute tokens
with a local procedural macro that preserves the original libtest test and
adds a private export function. It does not apply the JVM harness's reduction
of sort test sizes. Benches and doctests are outside these five test targets.

## Run

Run these commands from the `oxide` directory. Requirements match the main
project: Linux arm64/amd64, Go 1.27.1, Python 3.11 or newer, and the pinned Rust
compiler with its development components and sources.

```sh
./oxide-rs/build.sh
python3 fixtures/upstream/run.py list
python3 fixtures/upstream/run.py check
```

`check` rediscovers all five targets and reruns every case in the host's recorded
passing set against both native Rust and Oxide. It fails if the baseline is
absent, empty or invalid, or if any recorded success disappears, becomes
ignored, has no result, times out, fails to build, fails its native reference or
fails in generated Go. A recorded success reported as `blocked` also fails the
gate. A change to the baseline context also fails. The runner
incrementally rebuilds the Rust frontend and Go translator from the current
checkout and exports fresh MIR for each invocation; it does
not accept a previous report as proof that a test still passes.
An explicit `OXIDE_FRONTEND` override uses that executable instead; keep it
current when testing changes to a separately built frontend.

The ordinary Go test suite leaves this expensive gate disabled. Enable it
explicitly with:

```sh
(cd oxide-go && OXIDE_RUST_TESTS=1 go test ./internal/mir \
  -run '^TestRustUpstreamRegression$' -timeout 2h -v)
```

CI can invoke `python3 fixtures/upstream/run.py check` directly; that command
does not require `OXIDE_RUST_TESTS`. Choose an outer job timeout suitable for the
size of the recorded baseline.

The [GitHub Actions workflow](../../.github/workflows/upstream-tests.yml) runs
automatically for every push and pull request, and can also be started manually.
It uses `ubuntu-24.04-arm`, Go 1.27.1 and Python 3.13 with a 120-minute job timeout. It
runs the fixture unit tests, pinned Rust adapter tests and the arm64 `check`
gate. CI never promotes a baseline. Reports and diagnostic logs are uploaded
after success or failure, without native binaries, generated sources or Cargo
build caches. CI keeps the default single worker. After the pinned frontend is
built, it removes only downloaded `.cache/*.tar.xz` archives to reclaim disk
space on the hosted runner; the installed sysroot and other cache files remain.
This cleanup does not change the compiler, test inputs or test semantics.
The workflow is configured but has not yet run remotely on GitHub Actions;
the measurements above were obtained locally.

CI also compares the checked-in baseline with the pull request's base commit
or the previous commit of a push. The current baseline must retain every
previously passing case and the same context, so deleting entries cannot hide
a regression from `check`. Initial creation is allowed only when the previous
commit has no such baseline, or an initial push has no previous commit. Missing
Git objects and other Git errors fail the job. A manual run validates the
baseline against its own commit, then reruns all recorded successes; it does
not establish a comparison with an earlier commit. The current arm64 baseline
must exist in every case.

Repository administrators must separately configure a required status check
to block merges; adding the workflow does not change branch protection. The
check name is `Upstream Rust regression (linux/arm64)`. Before requiring it
for every pull request, ensure it has completed successfully in the repository.
The workflow has no path filters, so unrelated changes also receive the check.
See [GitHub's required-check guidance](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks).

## Discover failures and extend coverage

```sh
# Measure every discovered case; upstream ignored cases remain ignored.
python3 fixtures/upstream/run.py scan

# Diagnose a subset using a substring of the full suite/test case ID.
python3 fixtures/upstream/run.py scan --filter 'coretests/option::'
python3 fixtures/upstream/run.py scan --filter 'alloctests-internal/' --keep-generated
python3 fixtures/upstream/run.py scan --select /path/to/candidate-ids.json

# Add candidates while rerunning every already recorded success.
python3 fixtures/upstream/run.py promote --filter 'alloctests/vec::'
python3 fixtures/upstream/run.py promote --select /path/to/candidate-ids.json

# Rerun the full inventory and record new successes.
python3 fixtures/upstream/run.py promote

# Optional: use four workers on a local machine with sufficient resources.
GOMAXPROCS=2 python3 fixtures/upstream/run.py promote --jobs 4
```

`scan` produces a diagnostic report and leaves the baseline unchanged. A
completed scan can exit successfully while individual tests fail; use `check`
for the regression gate. Selected scans report unselected cases as `not_run`.
`--filter` matches a substring of the full case ID. `--select FILE` reads a JSON
array of exact discovered case ID strings, such as IDs copied from `list` or
a report. For example:

```json
[
  "alloctests/borrow::cow_const",
  "alloctests/borrow::test_from_cow_slice"
]
```

The two selection options are mutually exclusive and are available for `scan`
and `promote`; `check` rejects both options.

`promote` without a selection attempts the full discovered inventory. With a
filter or ID file, it attempts those candidates plus every case already in
the passing baseline, even if the selection excludes them. A passing outcome
requires fresh native execution, MIR export, Go compilation and Go execution.
A candidate ID list chooses work; it cannot import passing results
from an earlier report.

Promotion creates a missing host baseline or adds new successes only after
every existing success still passes. New candidates that fail remain in the
report and are not added to the passing set. A missing selected result,
interrupted run, removed discovery entry, changed context or empty passing set
prevents the baseline from being written. Intentionally unselected cases can
remain `not_run`; existing passing cases can never be skipped or removed to
accommodate a regression. Review and commit the resulting baseline file with
the change that establishes the new coverage, then run `check` against it.

The source lock verifies the compiler commit and SHA-256 digests of `coretests`,
`alloctests`, `alloc` and the upstream library lockfile. The baseline context
also records the host target, debug compiler flags, fixture dependency lockfile
and source/procedural-macro adapter digests. A compiler, source, dependency or
adapter upgrade cannot silently replace an existing baseline. Review the new
pins and adapter, then measure into an explicitly chosen new file with
`promote --baseline PATH`; review coverage changes before replacing the checked-in
baseline. Use `check --baseline PATH` when validating such a candidate.
The CI history guard also rejects replacement with a different context; moving
a candidate into the active baseline requires a separately reviewed migration
of that guard, not ordinary promotion.

## Execution and reports

Rustc/libtest discovers tests after macro expansion and `cfg` evaluation. Its
descriptor inventory is checked against the native libtest listing. The source
adapter leaves strings, character literals and comments untouched and rejects
unsupported conditional test creation instead of silently omitting it.
The procedural macro also recognizes attributes forwarded through `macro_rules!`
opaque fragments, including plain, message-matching and legacy `should_panic`
forms. It inspects a flattened view of those fragments while returning the
original test tokens unchanged.

An independent audit on 2026-09-29 also compiled byte-for-byte copies of the
original Rust sources and package manifests without instrumentation. All five
targets' test names and ignored flags matched: 4,662 tests and 2 ignored, with
no missing or extra entries. The internal alloc target used its original
`cargo test --lib` entry. This audit listed tests without executing their bodies.

Each selected case first runs through the native libtest oracle. Oxide exports
and compiles cases in batches. A fixture wrapper requests metadata-only output
for explicit MIR exports, avoiding another native executable for each batch;
native reference and discovery builds keep their normal compiler output modes.
Cargo must still exit successfully, and each export must produce fresh MIR.
The wrapper is pinned in the baseline's adapter hashes.
Compiler diagnostics guide smaller retry groups
where possible; otherwise the runner bisects a failed batch to isolate the
unsupported cases. Each retry exports and compiles its selected roots again,
and successfully compiled cases execute separately before they can pass.

Resource failures such as a full disk, exhausted file handles or unavailable
memory abort the run and cancel other workers. They do not become candidate
test failures and cannot produce a promoted baseline. Go's build cache lives
under `.cache/upstream/<arch>/go-cache`, and temporary build files use the
individual run directory. Each export also removes only the newly created
test-package build directories, preserving existing native references and
dependency artifacts, so recursive retries do not accumulate native binaries.

There is one exception to individual retries: a non-timeout `emit` failure
can block the whole batch when the exported MIR proves that the failing
function belongs to support code required by every selected root. The proof
must identify one failing function and a path of direct MIR call/assert-call
edges from the mandatory process-exit function or the return-value API shared
by all parameterless `u8` test roots. Its root set must exactly match the
batch. The runner saves that evidence in `shared-failure.log` and records
every affected case as `blocked`. It does not modify MIR, generated code or
the test body, and `blocked` never counts as passed. Without this proof,
ordinary retry grouping and bisection still apply; timeouts never establish
shared failure. Any previously passing case that becomes blocked fails the
regression gate.

The historical alloctests support failure involved an `__rust_alloc` `Item`
exposed through inline/re-exported `alloc` paths. Oxide's frontend now uses
rustc's internal-symbol marker to normalize those allocator declarations.
Three selected main/internal alloc cases pass after this repair; the original
full-scan diagnostics are retained. The fixture does not change allocator
semantics, MIR or generated code to bypass a failure.

Every compiled generated test executes in a separate process. The Rust adapter
preserves successful `Termination` results,
`should_panic`, and `should_panic(expected = ...)` string matching. A compiler
failure, Go runtime fault or abnormal process exit cannot satisfy an expected
Rust panic.

The default report is `.cache/upstream/<arch>/latest.json`. It includes the
discovered IDs, per-case status and panic/ignore metadata, failure details,
counts, context and the run's log directory. `--report PATH` chooses another
report destination. Logs are retained under
`.cache/upstream/<arch>/run-*/`; `--keep-generated` additionally retains MIR and
generated Go for diagnosis. An early setup failure may occur before a new
report is written, so use the current command's exit status and printed log
paths when assessing a run.

Full scans and promotion can take substantial time and disk space, especially
when failed compilation batches need repeated bisection. Cargo dependencies
are cached in `.cache/upstream/<arch>/cargo-target`; completed generated
binaries and MIR are normally removed, while logs and staged sources remain.
`--jobs` accepts 1 through 4 workers and defaults to 1. Additional workers run
batches concurrently using separate writable Cargo caches under
`.cache/upstream/<arch>/cargo-target-worker-*`. These caches can increase disk
usage substantially, and concurrent compiler/test processes require more CPU
and memory. The local four-worker example above reduces each Go process's
`GOMAXPROCS`; CI retains one worker to fit the hosted runner's resources.
`--batch-size` defaults to 128, `--timeout` to 30 seconds per test execution, and
`--build-timeout` to 600 seconds per build command. Slow tests remain reported
as timeouts unless a longer budget is supplied; their inputs are not reduced.
The runner defaults to `GOMAXPROCS=4`, `GOGC=50` and `GOMEMLIMIT=8GiB` when these
variables are unset. These are resource controls, not passing guarantees.

## Test the fixture itself

The Python checks use only the standard library:

```sh
python3 -m unittest discover -s fixtures/upstream -p 'test_*.py' -v
```

Run the procedural macro's native semantic tests with the same pinned compiler:

```sh
upstream_sysroot="$(bin/oxide-rs --print-sysroot)"
RUSTC="$upstream_sysroot/bin/rustc" \
  "$upstream_sysroot/bin/cargo" test --locked \
  --manifest-path fixtures/upstream/macros/Cargo.toml \
  --target-dir .cache/upstream-macro-tests
```

These checks cover source instrumentation, monotonic baseline validation,
shared-support failure proofs, conditional attributes, test return values, and
expected/unexpected panic handling. They do not substitute for the upstream
`check` command.
