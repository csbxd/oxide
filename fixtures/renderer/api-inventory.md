# Renderer public API acceptance inventory

This inventory covers `mermaid-rs-renderer` 0.3.1 at upstream commit
`3726ccbffe0e8032361eb9668694b24f77858060`, with its default `cli` and `png`
features retained and `scene` enabled. Translation targets the upstream library
and its dependencies directly. No fixture library or Rust observer supplies
the translated public API or seeds its generic instances.

[api-inventory.json](api-inventory.json) is the complete machine-readable
inventory, including every public alias, signature, generic parameter,
source location, trait identity, feature, and probe assignment.
[api.rs](api.rs) supplies only the native Rust reference observations.
The external Go tests call generated upstream roots and operate on Rust values
through compiler-derived type descriptors, including fields, enum payloads,
containers and real Rust destructors. Formatting, iteration and serde operations
also call compiler-resolved Rust implementations.
[check_api.py](check_api.py) checks export coverage without loading the large
MIR body file.

## Authority and scope

Two pinned `rustdoc` JSON builds were inspected: default features, and default
features plus `scene`. The compiler is `nightly-2026-09-15 (574ff7d98)` and the
rustdoc JSON format is 61. Public reachability follows module children and
`pub use` targets, including public types re-exported from private modules.
The JSON records both rustdoc input hashes. The compiler's separate public API
sidecar supplies resolved callable instances, inherited trait defaults,
canonical trait definitions, and actual export selection.

The public modules are `cli`, `config`, `error`, `ir`, `layout`, `layout_dump`,
`parser`, `render`, `scene`, `theme`, and `validator`. Root re-exports and
re-exports within these modules remain distinct public paths. The `benchmark`
feature only enables the Criterion benchmark target/dependency at this commit;
it adds no library source API. CLI process entry points are tested in isolated
processes because argument parsing, stdout, stderr and exit status are observable.

| Rustdoc callable definitions | Default features | Default + scene | Public paths with scene |
| --- | ---: | ---: | ---: |
| Free functions | 27 | 28 | 39 |
| Public inherent methods | 32 | 32 | 47 |
| Handwritten trait methods | 18 | 18 | 23 |
| Derive-generated trait methods | 406 | 430 | 575 |
| Callable tuple enum variant constructors | 14 | 15 | 18 |
| Public tuple struct constructors | 0 | 0 | 0 |

The scene build exposes 156 types and 70 public-use records, compared with 148
types and 61 public-use records in the default build. Unit variants and
named-field variants are values/constructions, not function constructors.

The compiler inventory has **1,089 upstream callable paths**: 39 free-function,
47 inherent-method, 18 constructor and 985 trait-method paths. The trait count
includes aliases and provided defaults, so it is not the rustdoc count of local
method definitions. Of these paths, 973 are monomorphic and 116 require concrete
type/const arguments. The complete export therefore has 973 selected
monomorphic roots and 1,089 inventory paths, all belonging to the upstream API.
Lifetimes alone do not prevent monomorphization. No handwritten free or inherent
method here requires a type/const argument; 55 local derive-generated method
definitions do. Blanket impls and synthetic auto-trait impls are not counted as
local callable definitions.

## API to probe mapping

The following tables list one shortest public path per definition, omitting
the `mermaid_rs_renderer::` prefix. All aliases remain in the JSON inventory.
Probe names refer to `api.rs::CASES`; `CLI processes` refers to the separate
process matrix below. A probe exercises the named Rust method but does not imply
exhaustive branch or input coverage.

### Free functions

| API | Probes | Feature gate |
| --- | --- | --- |
| `compute_layout` | `layout` | — |
| `compute_layout_with_metrics` | `layout_metrics`, `layout_metrics_sequence` | — |
| `config::load_config` | `config_none`, `config_file`, `config_json_error`, `config_missing` | — |
| `config::load_config_with_theme` | `config_theme`, `config_theme_error` | — |
| `config::parse_aspect_ratio_value` | `aspect_ratios` | — |
| `layout::flowchart_quality_metrics` | `quality`, `quality_other` | — |
| `layout::validate_layout_invariants` | `layout_valid`, `layout_invalid` | — |
| `layout_dump::write_layout_dump` | `layout_dump`, `layout_dump_error` | — |
| `measure` | `measure`, `measure_error` | — |
| `measure_svg_dimensions` | `measure_dimensions` | — |
| `measure_with_dimensions` | `measure_dimensions` | — |
| `merge_init_config` | `merge_config`, `merge_nonobject` | — |
| `parse_mermaid` | `parse`, `parse_error` | — |
| `parse_mermaid_strict` | `strict_parse`, `strict_directive`, `strict_unclosed`, `strict_end`, `strict_arrow`, `strict_click`, `strict_participant` | — |
| `render` | `render`, `render_error` | — |
| `render::arrowhead_inset` | `ir_methods` | — |
| `render::render_svg_with_dimensions` | `render_svg_dimensions` | — |
| `render_scene` | `scene`, `scene_class`, `scene_error` | `scene` |
| `render_strict` | `render_strict`, `render_strict_error` | — |
| `render_svg` | `render_svg` | — |
| `render_with_detailed_timing` | `detailed_timing`, `detailed_timing_error` | — |
| `render_with_options` | `render_options`, `render_init` | — |
| `render_with_timing` | `timing`, `timing_error` | — |
| `run` | CLI processes | `cli` |
| `validator::validate` | `validate`, `validate_error` | — |
| `write_layered_layout_dump` | `layered_dump`, `layered_dump_error` | — |
| `write_output_png` | `write_png`, `write_png_invalid`, `write_png_error` | `png` |
| `write_output_svg` | `write_svg`, `write_svg_error`; CLI processes | — |

### Inherent methods

| API | Probes | Feature gate |
| --- | --- | --- |
| `Direction::from_timeline_token` | `ir_methods` | — |
| `Direction::from_token` | `ir_methods` | — |
| `FlowchartLayoutEngine::as_str` | `ir_methods` | — |
| `Graph::ensure_node` | `ir_methods`, `serde_traits` | — |
| `Graph::new` | `ir_methods`, `serde_traits` | — |
| `LayoutConfig::class_label_line_height` | `config_defaults` | — |
| `LayoutStageMetrics::total_us` | `layout_metrics`, `timing_methods` | — |
| `RenderDetailedResult::total_ms` | `detailed_timing`, `timing_methods` | — |
| `RenderDetailedResult::total_us` | `detailed_timing`, `timing_methods` | — |
| `RenderOptions::mermaid_default` | `option_methods` | — |
| `RenderOptions::modern` | `option_methods` | — |
| `RenderOptions::with_node_spacing` | `option_methods` | — |
| `RenderOptions::with_preferred_aspect_ratio` | `option_methods` | — |
| `RenderOptions::with_preferred_aspect_ratio_parts` | `option_methods` | — |
| `RenderOptions::with_rank_spacing` | `option_methods` | — |
| `RenderResult::total_ms` | `timing`, `timing_methods` | — |
| `RenderResult::total_us` | `timing`, `timing_methods` | — |
| `SvgDimensions::aspect_ratio` | `dimension_methods` | — |
| `SvgDimensions::viewbox_aspect_ratio` | `dimension_methods` | — |
| `Theme::dark` | `theme_methods` | — |
| `Theme::forest` | `theme_methods` | — |
| `Theme::from_name` | `theme_methods` | — |
| `Theme::mermaid_default` | `theme_methods` | — |
| `Theme::modern` | `theme_methods` | — |
| `Theme::neutral` | `theme_methods` | — |
| `config::RequirementConfig::divider_offset` | `config_defaults` | — |
| `ir::ArchDir::from_char` | `ir_methods` | — |
| `ir::ArchDir::is_horizontal` | `ir_methods` | — |
| `ir::C4ShapeKind::as_str` | `ir_methods` | — |
| `layout::FlowchartQualityMetrics::geometry_debt_count` | `ir_methods`, `quality`, `quality_other` | — |
| `layout::FlowchartQualityMetrics::hard_violation_count` | `ir_methods`, `quality`, `quality_other` | — |
| `layout_dump::LayoutDump::from_layout` | `layout`, `layout_dump` | — |

### Handwritten trait methods

| API | Probes | Feature gate |
| --- | --- | --- |
| `<Config as core::default::Default>::default` | `config_defaults` | — |
| `<FlowchartLayoutEngine as core::convert::From<LayoutEngineArg>>::from` | `ir_methods` | `cli` |
| `<Graph as core::default::Default>::default` | `serde_traits` | — |
| `<LayoutConfig as core::default::Default>::default` | `config_defaults` | — |
| `<RenderConfig as core::default::Default>::default` | `config_defaults` | — |
| `<RenderOptions as core::default::Default>::default` | `config_defaults` | — |
| `<config::C4Config as core::default::Default>::default` | `config_defaults` | — |
| `<config::FlowchartAutoSpacingConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::FlowchartLayoutConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::FlowchartObjectiveConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::FlowchartRoutingConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::GitGraphConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::MindmapConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::PieConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::RequirementConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::TimelineConfig as core::default::Default>::default` | `config_defaults` | — |
| `<config::TreemapConfig as core::default::Default>::default` | `config_defaults` | — |
| `<layout::LayoutInvariantError as core::fmt::Display>::fmt` | `ir_methods` | — |

### Callable constructors

| API | Probes | Feature gate |
| --- | --- | --- |
| `Paint::Solid` | `construct_other` | `scene` |
| `RadarEntry::Named` | `construct_other` | — |
| `RadarEntry::Positional` | `construct_other` | — |
| `layout::DiagramData::C4` | `construct_c4` | — |
| `layout::DiagramData::Error` | `construct_error` | — |
| `layout::DiagramData::Gantt` | `construct_gantt` | — |
| `layout::DiagramData::GitGraph` | `construct_gitgraph` | — |
| `layout::DiagramData::Journey` | `construct_journey` | — |
| `layout::DiagramData::Pie` | `construct_pie` | — |
| `layout::DiagramData::Quadrant` | `construct_quadrant` | — |
| `layout::DiagramData::Radar` | `construct_radar` | — |
| `layout::DiagramData::Sankey` | `construct_sankey` | — |
| `layout::DiagramData::Sequence` | `construct_sequence` | — |
| `layout::DiagramData::Timeline` | `construct_timeline` | — |
| `layout::DiagramData::XYChart` | `construct_xychart` | — |

### Derived and provided trait methods

All 985 trait-method paths are recorded individually, with canonical identities
such as `core::clone::Clone`, `serde_core::ser::Serialize` and
`serde_core::de::Deserialize`. Every monomorphic path must be selected and have
a root symbol. Generic declarations are retained explicitly instead of being
silently dropped; concrete `serde_json` serialize/deserialize calls for `Theme`
and `LayoutConfig` are witnesses in `serde_traits`. Clone, default, debug and
comparison operations also execute in the Rust probes and constructor checks.

This does **not** individually test every generated `Debug`, `Clone`, `PartialEq`,
`Hash`, clap or serde method, every provided trait default, or every possible
generic instantiation. The JSON marks these as `not_individually_probed` or
`inherited_default_not_individually_probed`; a selected export must not be
reported as an individual behavioral pass. Generic input types not concretely
used by the probes remain outside this differential test matrix.

## Differential oracles

The 74 Rust cases accept an input string and a caller-owned temporary path.
The native-only `api::run` observer returns the full byte length and only copies
bytes when capacity is sufficient. Native acceptance executes each case once
for its length and twice for its complete bytes, rejecting nondeterminism.
Go independently composes the same operations from public library roots and
compares the full result; it does not call a translated `api::run` or other
fixture wrapper. Parser, layout, rendering and serialization remain actual
translated Rust implementations.

| Observable boundary | Oracle |
| --- | --- |
| Parsing, configuration, IR, dimensions and metrics | Complete deterministic Rust result presentation, including error results and invalid-input cases. Graph's nine `HashMap` fields are presented in sorted key order without dropping fields or values. |
| Rendering and scene | Complete SVG bytes; scene's complete Rust `Debug` representation includes all commands, paths and paints. PNG writers return their actual file bytes. |
| File operations | Each call initializes its own path state, reads the complete created file on success, and returns the error on failure. Only the caller-specific path is replaced with `<path>`. Directory-as-file and missing-file cases are intentional. |
| Timing | Actually calls both timed rendering APIs and layout-with-metrics. Checks exact sums and total-millisecond relationships; compares the resulting SVG and deterministic layout data. Measured durations cannot match across independent executions and are not compared as bytes. `timing_methods` separately tests supplied integer inputs, including values wider than `u64`. |
| Constructors | Native Rust calls tuple constructors through typed function pointers; Go calls their generated public roots. Both clone and check the reconstructed value. Diagram payloads come from actual Rust parsing/layout; scalar payload inputs are built in Go using compiler-derived fields and enum descriptors. |
| CLI | Original Rust `cli::run` executes in a separate process per case. Compare exit status, complete stdout, complete stderr and every file. Real timing stderr is retained and checked against its numeric schema and exact sums before comparison of its contract result. |

The CLI cases are: `help`, `version`, `unknown-flag`, `bad-ratio`, `empty-input`,
`missing-file`, `stdin-svg`, `implicit-stdin`, `svg-file`, `png-file`,
`png-missing-output`, `size`, `options`, `config`, `invalid-config`,
`layout-dumps`, `timing`, `markdown`, `markdown-size`, and `markdown-empty`.
The two stdout SVG cases exercise `write_output_svg` with no output path.
The markdown cases cover multiple diagrams, file naming and size-only output.
The CLI executable is given a fixed argv[0] so its usage text is comparable.

The Go acceptance package uses `memory.counters`. After a fixed warm call for
each API case, every measured invocation checks raw Go allocation object/byte
totals, Context restoration and three independent ownership records: live Rust
heap allocations, the full libc allocator snapshot, and file-backed mapping
bytes grouped by device/inode. Rust operations and their Drop calls remain
inside the allocation window. Input preparation and Go-side file comparison
are outside it. Successful writer outputs are removed after comparison so a
later invocation cannot pass by leaving a previous file untouched. These checks
do not imply coverage of every anonymous mapping or every possible API input.

## Reproduction and coverage gate

From the `oxide` directory, after building the pinned frontend, use the cache
for the machine running the native references (`arm64` below; use `amd64` on an
amd64 machine). `test.py` chooses that host architecture automatically:

```sh
python3 fixtures/renderer/test.py --stage native
python3 fixtures/renderer/test.py --stage export
python3 fixtures/renderer/check_api.py \
  .cache/renderer-direct/arm64/oxide.mir.api.json \
  --cases .cache/renderer-direct/arm64/reference/api/cases.json \
  --cli-results .cache/renderer-direct/arm64/cli/native/results.json
python3 fixtures/renderer/test.py --stage go
```

The full command is `python3 fixtures/renderer/test.py`. It preserves default
dependency features and enables scene. Native and exported builds use the same
pinned compiler, `-Zbuild-std=std,panic_unwind`, `-Zalways-encode-mir`,
`-Zmir-opt-level=0`, and `-Coverflow-checks=yes` in Cargo's dev profile. Run it
separately on Linux arm64 and amd64 to obtain references on the same target as
the translated executable. This supplements the existing 72-source SVG/PNG
matrix rather than replacing it.

[check_dependencies.py](check_dependencies.py) compares Cargo's resolved
upstream dependency closure in the native oracle build and in the direct
library build. The current closure has 96 packages; package versions, enabled
features and dependency edges must match. The gate runs before the stages and
writes `dependency-closure.json` into the architecture's direct cache. Native
oracle source is not part of the translated dependency closure.

The coverage checker imports as
`check_api.check_api(sidecar, native_cases, cli_results_path=cli_results)`.
It rejects missing or unexpected upstream paths, changed definitions or canonical
trait identities, unselected monomorphic APIs, missing root symbols, missing
rustdoc function/method/constructor aliases, stale probe source hashes, unknown
probe names, missing reference files, and changes to the declared CLI matrix.
It reads the small `.api.json` sidecar, never the full MIR. Both architecture
triples are accepted; the API and compiler pin must still match this inventory.

Current native/Go execution and ownership results are maintained in
[MILESTONES.md](../../MILESTONES.md), separately from this discovery/coverage
inventory. Passing the inventory gate alone does not establish execution or
individual behavioral coverage of every exported method.

## Rebuilding the inventory

[inventory.py](inventory.py) discovers APIs afresh from rustdoc JSON and checks
them against the compiler sidecar. [api-probes.json](api-probes.json) is the
explicit, manually maintained mapping from individual definitions to behavior
probes; it is not a discovery input. A new handwritten function, method or
constructor without a mapping fails generation, including a new method on an
already-tested type. New derive/default methods stay explicitly untested unless
assigned a probe. Removed definitions leave stale mappings and also fail.
`api-inventory.md` remains an edited explanation of the machine inventory.

From `oxide`, using the same upstream checkout as the fixture, rebuild the two
rustdoc authority files with the pinned compiler. This uses its own Cargo cache
and does not alter the renderer's MIR or generated Go:

```sh
OXIDE_API_SYSROOT="$(bin/oxide-rs --print-sysroot)"
mkdir -p .cache/renderer-api/tmp
(
  export TMPDIR="$PWD/.cache/renderer-api/tmp"
  export RUSTC="$OXIDE_API_SYSROOT/bin/rustc"
  export RUSTDOC="$OXIDE_API_SYSROOT/bin/rustdoc"
  export RUSTC_BOOTSTRAP=1 RUSTC_WRAPPER= RUSTC_WORKSPACE_WRAPPER=
  export RUSTFLAGS= CARGO_ENCODED_RUSTFLAGS= OXIDE_EXPORT= OXIDE_ROOTS=
  export CARGO_TARGET_DIR="$PWD/.cache/renderer-api/rustdoc-target"
  "$OXIDE_API_SYSROOT/bin/cargo" rustdoc -Zbuild-std=std,panic_unwind \
    --manifest-path ../mermaid-rs-renderer/Cargo.toml --lib --locked \
    --target aarch64-unknown-linux-gnu \
    -- -Zunstable-options --output-format json --document-private-items
  cp "$CARGO_TARGET_DIR/aarch64-unknown-linux-gnu/doc/mermaid_rs_renderer.json" \
    .cache/renderer-api/default.json
  "$OXIDE_API_SYSROOT/bin/cargo" rustdoc -Zbuild-std=std,panic_unwind \
    --manifest-path ../mermaid-rs-renderer/Cargo.toml --lib --locked \
    --features scene --target aarch64-unknown-linux-gnu \
    -- -Zunstable-options --output-format json --document-private-items
  cp "$CARGO_TARGET_DIR/aarch64-unknown-linux-gnu/doc/mermaid_rs_renderer.json" \
    .cache/renderer-api/scene.json
)
```

Private-item documentation permits resolving re-export targets; public
reachability determines which records belong in the inventory. Defaults stay
enabled in both builds. Raw artifact hashes are recorded in the output.

After generating current native references and a current `.api.json` sidecar
with `test.py --stage native` and `test.py --stage export`, rebuild the inventory:

```sh
python3 fixtures/renderer/inventory.py \
  --default .cache/renderer-api/default.json \
  --scene .cache/renderer-api/scene.json \
  --api .cache/renderer-direct/arm64/oxide.mir.api.json \
  --cases .cache/renderer-direct/arm64/reference/api/cases.json
```

Use `--check` to require exact equality with the existing JSON without writing
it, or `--output FILE` to review a proposed result. `--upstream` selects the
upstream source checkout; package provenance and its commit come from Cargo.toml
and Git. No generation step reads the existing inventory as a discovery source,
and none loads the large MIR file. After changing `api.rs`, rebuild native
references before updating its recorded hash.

The persistent [negative checks](test_check_api.py) start from a real completed
export and reject eight deliberate coverage corruptions. They also verify that
a new unassigned handwritten method fails and a new derived method is marked
untested:

```sh
python3 fixtures/renderer/test_check_api.py \
  --api .cache/renderer-direct/arm64/oxide.mir.api.json \
  --cases .cache/renderer-direct/arm64/reference/api/cases.json \
  --cli-results .cache/renderer-direct/arm64/cli/native/results.json
```
