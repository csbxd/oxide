//! Public-library API probes. All semantic operations call the original Rust
//! library. Only unordered-map presentation and caller-specific paths are normalized.

use mermaid_rs_renderer as r;
use std::collections::BTreeMap;
use std::fmt::{Debug, Display, Write};
use std::path::Path;

const FLOW: &str = "flowchart LR\n A[Alpha] -->|go| B{Beta}\n";
const SEQUENCE: &str = "sequenceDiagram\n participant A\n participant B\n A->>B: Hello\n";
const SVG: &str = r##"<svg xmlns="http://www.w3.org/2000/svg" width="16" height="12"><rect width="16" height="12" fill="#f00"/></svg>"##;

pub const CASES: &[(&str, &str)] = &[
    ("parse", FLOW),
    ("parse_error", "not a diagram"),
    ("strict_parse", FLOW),
    ("strict_directive", "%%{init: {broken}}%%\nflowchart TD\n A"),
    ("strict_unclosed", "flowchart TD\nsubgraph group\n A"),
    ("strict_end", "flowchart TD\n end"),
    ("strict_arrow", "flowchart TD\n --> B"),
    ("strict_click", "flowchart TD\nA\nclick A \"unterminated"),
    (
        "strict_participant",
        "sequenceDiagram\nparticipant Alice\nAlice->>Nobody: hello",
    ),
    ("validate", FLOW),
    ("validate_error", "flowchart TD\n --> B"),
    ("render", FLOW),
    ("render_error", "not a diagram"),
    ("render_options", FLOW),
    (
        "render_init",
        "%%{init: {'theme':'dark','flowchart':{'nodeSpacing':31,'rankSpacing':61}}}%%\nflowchart TD\n A --> B",
    ),
    ("render_strict", FLOW),
    ("render_strict_error", "flowchart TD\nsubgraph open\n A"),
    ("timing", FLOW),
    ("timing_error", "not a diagram"),
    ("detailed_timing", FLOW),
    ("detailed_timing_error", "not a diagram"),
    ("layout", FLOW),
    ("layout_metrics", FLOW),
    ("layout_metrics_sequence", SEQUENCE),
    ("layout_valid", FLOW),
    ("layout_invalid", FLOW),
    ("quality", FLOW),
    ("quality_other", SEQUENCE),
    ("layout_dump", FLOW),
    ("layout_dump_error", FLOW),
    ("layered_dump", FLOW),
    ("layered_dump_error", FLOW),
    ("render_svg", FLOW),
    ("render_svg_dimensions", FLOW),
    ("measure", FLOW),
    ("measure_dimensions", FLOW),
    ("measure_error", "not a diagram"),
    ("dimension_methods", ""),
    (
        "theme_methods",
        "modern|default|mermaid|base|dark|forest|neutral| MODERN |unknown",
    ),
    ("option_methods", ""),
    ("ir_methods", ""),
    ("config_defaults", ""),
    ("config_none", ""),
    (
        "config_file",
        r##"{"theme":"dark","themeVariables":{"primaryColor":"#123456","fontSize":17},"flowchart":{"nodeSpacing":37,"rankSpacing":59}}"##,
    ),
    ("config_json_error", "{ broken"),
    ("config_missing", ""),
    (
        "config_theme",
        r##"{"theme":"dark","themeVariables":{"primaryColor":"#123456"}}"##,
    ),
    ("config_theme_error", ""),
    (
        "merge_config",
        r##"{"theme":"forest","themeVariables":{"fontSize":18,"lineColor":"#010203"},"flowchart":{"nodeSpacing":38,"rankSpacing":62,"layoutEngine":"dagre"}}"##,
    ),
    ("merge_nonobject", "null"),
    (
        "aspect_ratios",
        "16:9| 4 / 3 |1.25||zero|0|-1|NaN|inf|a:2|2:b|1:0|inf:1|2:3:4",
    ),
    ("write_svg", SVG),
    ("write_svg_error", SVG),
    ("write_png", SVG),
    ("write_png_invalid", "not svg"),
    ("write_png_error", SVG),
    ("scene", FLOW),
    ("scene_class", include_str!("cases/class.mmd")),
    ("scene_error", "not a diagram"),
    ("serde_traits", FLOW),
    ("timing_methods", ""),
    (
        "construct_sequence",
        include_str!("cases/upstream/sequence/basic.mmd"),
    ),
    (
        "construct_pie",
        include_str!("cases/upstream/pie/basic.mmd"),
    ),
    (
        "construct_quadrant",
        include_str!("cases/upstream/quadrant/basic.mmd"),
    ),
    (
        "construct_gantt",
        include_str!("cases/upstream/gantt/basic.mmd"),
    ),
    (
        "construct_sankey",
        include_str!("cases/upstream/sankey/basic.mmd"),
    ),
    (
        "construct_gitgraph",
        include_str!("cases/upstream/gitgraph/basic.mmd"),
    ),
    ("construct_c4", include_str!("cases/upstream/c4/basic.mmd")),
    (
        "construct_xychart",
        include_str!("cases/upstream/xychart/basic.mmd"),
    ),
    (
        "construct_timeline",
        include_str!("cases/upstream/timeline/basic.mmd"),
    ),
    (
        "construct_journey",
        include_str!("cases/upstream/journey/basic.mmd"),
    ),
    (
        "construct_radar",
        include_str!("cases/upstream/radar/basic.mmd"),
    ),
    ("construct_error", ""),
    ("construct_other", ""),
];

fn debug(value: impl Debug) -> Vec<u8> {
    format!("{value:?}").into_bytes()
}

fn error(value: impl Display, path: &str) -> Vec<u8> {
    format!("error:{value}")
        .replace(path, "<path>")
        .into_bytes()
}

fn checked<T: Debug, E: Display>(value: Result<T, E>, path: &str) -> Vec<u8> {
    match value {
        Ok(value) => debug(value),
        Err(value) => error(value, path),
    }
}

fn svg_result<E: Display>(value: Result<String, E>, path: &str) -> Vec<u8> {
    match value {
        Ok(value) => value.into_bytes(),
        Err(value) => error(value, path),
    }
}

// Graph only has these nine HashMap fields. Preserve every field and value,
// but present map entries in key order; native HashMap iteration is randomized.
fn graph_bytes(graph: &r::Graph) -> Vec<u8> {
    let mut graph = graph.clone();
    let mut out = String::new();
    macro_rules! map {
        ($field:ident) => {
            writeln!(
                &mut out,
                "{}={:?}",
                stringify!($field),
                std::mem::take(&mut graph.$field)
                    .into_iter()
                    .collect::<BTreeMap<_, _>>()
            )
            .unwrap();
        };
    }
    map!(node_order);
    map!(class_defs);
    map!(node_classes);
    map!(node_styles);
    map!(subgraph_styles);
    map!(subgraph_classes);
    map!(node_links);
    map!(edge_styles);
    map!(arch_edge_ports);
    writeln!(&mut out, "{graph:?}").unwrap();
    out.into_bytes()
}

fn parsed(value: r::parser::ParseOutput) -> Vec<u8> {
    let mut out = graph_bytes(&value.graph);
    out.extend(debug(value.init_config));
    out
}

fn layout(source: &str) -> (r::Graph, r::Layout) {
    let graph = r::parse_mermaid(source).unwrap().graph;
    let layout = r::compute_layout(&graph, &r::Theme::modern(), &r::LayoutConfig::default());
    (graph, layout)
}

fn dump(graph: &r::Graph, layout: &r::Layout) -> Vec<u8> {
    serde_json::to_vec(&r::layout_dump::LayoutDump::from_layout(layout, graph)).unwrap()
}

fn stage_metrics(value: &r::LayoutStageMetrics) {
    assert_eq!(
        value.total_us(),
        value.layered_layout_us
            + value.port_assignment_us
            + value.edge_routing_us
            + value.label_placement_us
    );
}

fn reset_path(path: &Path) {
    match std::fs::symlink_metadata(path) {
        Ok(meta) if meta.is_dir() => std::fs::remove_dir_all(path).unwrap(),
        Ok(_) => std::fs::remove_file(path).unwrap(),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => {}
        Err(err) => panic!("reset path: {err}"),
    }
}

fn execute(name: &str, source: &str, path: &str) -> Vec<u8> {
    let file = Path::new(path);
    match name {
        "parse" | "parse_error" => match r::parser::parse_mermaid(source) {
            Ok(value) => parsed(value),
            Err(value) => error(value, path),
        },
        name if name.starts_with("strict_") => match r::parse_mermaid_strict(source) {
            Ok(value) => parsed(value),
            Err(value) => debug((
                format!("{value:?}"),
                value.to_string(),
                std::error::Error::source(&value).is_some(),
            )),
        },
        "validate" | "validate_error" => checked(r::validator::validate(source), path),
        "render" | "render_error" => svg_result(r::render(source), path),
        "render_options" | "render_init" => svg_result(
            r::render_with_options(
                source,
                r::RenderOptions::mermaid_default()
                    .with_node_spacing(37.0)
                    .with_rank_spacing(63.0)
                    .with_preferred_aspect_ratio(1.5),
            ),
            path,
        ),
        "render_strict" | "render_strict_error" => {
            svg_result(r::render_strict(source, r::RenderOptions::modern()), path)
        }
        "timing" | "timing_error" => {
            match r::render_with_timing(source, r::RenderOptions::default()) {
                Ok(value) => {
                    assert_eq!(
                        value.total_us(),
                        value.parse_us + value.layout_us + value.render_us
                    );
                    assert_eq!(value.total_ms(), value.total_us() as f64 / 1000.0);
                    let mut out = b"timing relations checked\n".to_vec();
                    out.extend(value.svg.as_bytes());
                    out
                }
                Err(value) => error(value, path),
            }
        }
        "detailed_timing" | "detailed_timing_error" => {
            match r::render_with_detailed_timing(source, r::RenderOptions::default()) {
                Ok(value) => {
                    assert_eq!(
                        value.total_us(),
                        value.parse_us + value.layout_us + value.render_us
                    );
                    assert_eq!(value.total_ms(), value.total_us() as f64 / 1000.0);
                    stage_metrics(&value.layout_stages);
                    let mut out = b"timing/stage relations checked\n".to_vec();
                    out.extend(value.svg.as_bytes());
                    out.extend(serde_json::to_vec(&value.layout_stages.layered_layout).unwrap());
                    out
                }
                Err(value) => error(value, path),
            }
        }
        "layout" => {
            let (graph, value) = layout(source);
            dump(&graph, &value)
        }
        "layout_metrics" | "layout_metrics_sequence" | "layered_dump" | "layered_dump_error" => {
            let graph = r::parse_mermaid(source).unwrap().graph;
            let mut config = r::LayoutConfig::default();
            config.flowchart.engine = r::FlowchartLayoutEngine::Dagre;
            let (value, metrics) =
                r::compute_layout_with_metrics(&graph, &r::Theme::modern(), &config);
            stage_metrics(&metrics);
            if name.starts_with("layered_dump") {
                let snapshot = metrics.layered_layout.as_ref().expect("flowchart snapshot");
                reset_path(file);
                if name.ends_with("error") {
                    std::fs::create_dir(file).unwrap();
                }
                return match r::write_layered_layout_dump(file, snapshot) {
                    Ok(()) => std::fs::read(file).unwrap(),
                    Err(value) => error(value, path),
                };
            }
            let mut out = dump(&graph, &value);
            out.extend(serde_json::to_vec(&metrics.layered_layout).unwrap());
            out
        }
        "layout_valid" | "layout_invalid" => {
            let (_, mut value) = layout(source);
            if name.ends_with("invalid") {
                value.width = f32::NAN;
                value.height = -1.0;
                value.nodes.values_mut().next().unwrap().width = 0.0;
                value.edges[0].points.clear();
            }
            match r::layout::validate_layout_invariants(&value) {
                Ok(()) => b"valid".to_vec(),
                Err(errors) => debug(
                    errors
                        .iter()
                        .map(|e| (e.path.clone(), e.message.clone(), e.to_string()))
                        .collect::<Vec<_>>(),
                ),
            }
        }
        "quality" | "quality_other" => {
            let (_, value) = layout(source);
            debug(r::layout::flowchart_quality_metrics(&value).map(|value| {
                (
                    value,
                    value.hard_violation_count(),
                    value.geometry_debt_count(),
                )
            }))
        }
        "layout_dump" | "layout_dump_error" => {
            let (graph, value) = layout(source);
            reset_path(file);
            if name.ends_with("error") {
                std::fs::create_dir(file).unwrap();
            }
            match r::layout_dump::write_layout_dump(file, &value, &graph) {
                Ok(()) => std::fs::read(file).unwrap(),
                Err(value) => error(value, path),
            }
        }
        "render_svg" | "render_svg_dimensions" => {
            let (_, value) = layout(source);
            if name.ends_with("dimensions") {
                r::render::render_svg_with_dimensions(
                    &value,
                    &r::Theme::modern(),
                    &r::LayoutConfig::default(),
                    Some((640.0, 480.0)),
                )
                .into_bytes()
            } else {
                r::render_svg(&value, &r::Theme::modern(), &r::LayoutConfig::default()).into_bytes()
            }
        }
        "measure" | "measure_error" => {
            checked(r::measure(source, r::RenderOptions::default()), path)
        }
        "measure_dimensions" => {
            let (_, value) = layout(source);
            let measured = r::measure_with_dimensions(
                source,
                r::RenderOptions::default(),
                Some((321.0, 123.0)),
            )
            .unwrap();
            let direct = r::measure_svg_dimensions(
                &value,
                &r::LayoutConfig::default(),
                Some((321.0, 123.0)),
            );
            assert_eq!(measured, direct);
            serde_json::to_vec(&measured).unwrap()
        }
        "dimension_methods" => {
            let values = [0.0, -1.0, 0.5, 12.0].map(|height| {
                let d = r::SvgDimensions {
                    width: 24.0,
                    height,
                    viewbox_x: 0.0,
                    viewbox_y: 0.0,
                    viewbox_width: 36.0,
                    viewbox_height: height,
                };
                (
                    d.aspect_ratio().to_bits(),
                    d.viewbox_aspect_ratio().to_bits(),
                )
            });
            debug(values)
        }
        "theme_methods" => {
            let presets = [
                r::Theme::modern(),
                r::Theme::mermaid_default(),
                r::Theme::dark(),
                r::Theme::forest(),
                r::Theme::neutral(),
            ];
            serde_json::to_vec(&serde_json::json!({"presets":presets,"lookups":source.split('|').map(r::Theme::from_name).collect::<Vec<_>>()})).unwrap()
        }
        "option_methods" => {
            let mut result = Vec::new();
            for ratio in [2.0, 0.0, -1.0, f32::NAN, f32::INFINITY] {
                result.push(
                    r::RenderOptions::modern()
                        .with_node_spacing(33.0)
                        .with_rank_spacing(55.0)
                        .with_preferred_aspect_ratio(ratio),
                );
                result.push(
                    r::RenderOptions::mermaid_default()
                        .with_preferred_aspect_ratio_parts(ratio, 3.0),
                );
                result.push(
                    r::RenderOptions::default().with_preferred_aspect_ratio_parts(3.0, ratio),
                );
            }
            debug(result)
        }
        "ir_methods" => ir_methods(),
        "config_defaults" => config_defaults(),
        "config_none" => checked(r::config::load_config(None), path),
        "config_file" | "config_json_error" | "config_theme" | "config_missing" => {
            reset_path(file);
            if name != "config_missing" {
                std::fs::write(file, source).unwrap();
            }
            if name == "config_theme" {
                checked(
                    r::config::load_config_with_theme(Some(file), Some("neutral")),
                    path,
                )
            } else {
                checked(r::config::load_config(Some(file)), path)
            }
        }
        "config_theme_error" => checked(
            r::config::load_config_with_theme(None, Some("unrecognized-theme")),
            path,
        ),
        "merge_config" | "merge_nonobject" => debug(r::merge_init_config(
            r::Config::default(),
            serde_json::from_str(source).unwrap(),
        )),
        "aspect_ratios" => debug(
            source
                .split('|')
                .map(r::config::parse_aspect_ratio_value)
                .collect::<Vec<_>>(),
        ),
        "write_svg" | "write_svg_error" | "write_png" | "write_png_invalid" | "write_png_error" => {
            reset_path(file);
            if name.ends_with("error") {
                std::fs::create_dir(file).unwrap();
            }
            let result = if name.starts_with("write_svg") {
                r::write_output_svg(source, Some(file))
            } else {
                r::write_output_png(
                    source,
                    file,
                    &r::RenderConfig::default(),
                    &r::Theme::modern(),
                )
            };
            match result {
                Ok(()) => std::fs::read(file).unwrap(),
                Err(value) => error(value, path),
            }
        }
        "scene" | "scene_class" | "scene_error" => {
            checked(r::render_scene(source, r::RenderOptions::modern()), path)
        }
        "serde_traits" => {
            let theme = r::Theme::forest();
            let bytes = serde_json::to_vec(&theme).unwrap();
            let decoded: r::Theme = serde_json::from_slice(&bytes).unwrap();
            assert_eq!(bytes, serde_json::to_vec(&decoded).unwrap());
            let config = r::LayoutConfig::default();
            let encoded = serde_json::to_vec(&config).unwrap();
            let decoded: r::LayoutConfig = serde_json::from_slice(&encoded).unwrap();
            assert_eq!(encoded, serde_json::to_vec(&decoded).unwrap());
            let mut out = bytes;
            out.extend(encoded);
            out.extend(debug(config.clone()));
            let graph = r::Graph::default();
            out.extend(graph_bytes(&graph));
            out
        }
        "timing_methods" => {
            let result = r::RenderResult {
                svg: String::new(),
                parse_us: 1u128 << 65,
                layout_us: 7,
                render_us: 9,
            };
            let stages = r::LayoutStageMetrics {
                layered_layout_us: 11,
                port_assignment_us: 13,
                edge_routing_us: 17,
                label_placement_us: 19,
                layered_layout: None,
            };
            let detailed = r::RenderDetailedResult {
                svg: String::new(),
                parse_us: 23,
                layout_us: 29,
                render_us: 31,
                layout_stages: stages,
            };
            debug((
                result.total_us(),
                result.total_ms().to_bits(),
                detailed.total_us(),
                detailed.total_ms().to_bits(),
                detailed.layout_stages.total_us(),
            ))
        }
        name if name.starts_with("construct_") => constructors(name, source),
        _ => panic!("unknown API probe {name}"),
    }
}

fn constructors(name: &str, source: &str) -> Vec<u8> {
    use r::layout::{DiagramData as D, ErrorLayout};
    if name == "construct_other" {
        let positional: fn(Option<f32>) -> r::RadarEntry = r::RadarEntry::Positional;
        let named: fn(String, Option<f32>) -> r::RadarEntry = r::RadarEntry::Named;
        let solid: fn(r::Color) -> r::Paint = r::Paint::Solid;
        return debug((
            positional(None),
            positional(Some(1.25)),
            named("axis".into(), Some(3.5)),
            solid(r::Color {
                r: 1,
                g: 2,
                b: 3,
                a: 0.5,
            }),
        ));
    }
    let value = if name == "construct_error" {
        let ctor: fn(ErrorLayout) -> D = D::Error;
        ctor(ErrorLayout {
            viewbox_width: 1.0,
            viewbox_height: 2.0,
            render_width: 3.0,
            render_height: 4.0,
            message: "error probe".into(),
            version: "version probe".into(),
            text_x: 5.0,
            text_y: 6.0,
            text_size: 7.0,
            version_x: 8.0,
            version_y: 9.0,
            version_size: 10.0,
            icon_scale: 11.0,
            icon_tx: 12.0,
            icon_ty: 13.0,
        })
    } else {
        layout(source).1.diagram
    };
    let original = debug(&value);
    macro_rules! ctor {
        ($variant:ident,$value:ident) => {{
            let constructor: fn(_) -> D = D::$variant;
            constructor($value)
        }};
    }
    let value = match value {
        D::Sequence(value) => ctor!(Sequence, value),
        D::Pie(value) => ctor!(Pie, value),
        D::Quadrant(value) => ctor!(Quadrant, value),
        D::Gantt(value) => ctor!(Gantt, value),
        D::Sankey(value) => ctor!(Sankey, value),
        D::GitGraph(value) => ctor!(GitGraph, value),
        D::C4(value) => ctor!(C4, value),
        D::XYChart(value) => ctor!(XYChart, value),
        D::Timeline(value) => ctor!(Timeline, value),
        D::Journey(value) => ctor!(Journey, value),
        D::Radar(value) => ctor!(Radar, value),
        D::Error(value) => ctor!(Error, value),
        D::Graph { .. } => panic!("constructor probe produced graph variant"),
    };
    assert_eq!(original, debug(value.clone()));
    debug(value)
}

fn config_defaults() -> Vec<u8> {
    use r::config::*;
    let mut result = debug((Config::default(), r::RenderOptions::default()));
    macro_rules! value {
        ($t:ty) => {
            result.extend(debug(<$t>::default()));
        };
    }
    value!(LayoutConfig);
    value!(RenderConfig);
    value!(RequirementConfig);
    value!(MindmapConfig);
    value!(GitGraphConfig);
    value!(C4Config);
    value!(PieConfig);
    value!(TreemapConfig);
    value!(TimelineConfig);
    value!(FlowchartLayoutConfig);
    value!(FlowchartObjectiveConfig);
    value!(FlowchartAutoSpacingConfig);
    value!(FlowchartRoutingConfig);
    let cfg = LayoutConfig::default();
    result.extend(debug((
        cfg.class_label_line_height(),
        cfg.requirement.divider_offset(10.0, 12.0),
        cfg.requirement.divider_offset(100.0, 120.0),
    )));
    result
}

fn ir_methods() -> Vec<u8> {
    use r::ir::{ArchDir, C4ShapeKind, DiagramKind, EdgeArrowhead};
    let mut out = debug(["TD", "TB", "BT", "LR", "RL", "lr", "?"].map(|s| {
        (
            r::Direction::from_token(s),
            r::Direction::from_timeline_token(s),
        )
    }));
    out.extend(debug(
        ['L', 'R', 'T', 'B', 'l', 'r', 't', 'b', '?', '界']
            .map(|c| ArchDir::from_char(c).map(|v| (v, v.is_horizontal()))),
    ));
    use C4ShapeKind::*;
    out.extend(debug(
        [
            Person,
            ExternalPerson,
            System,
            SystemDb,
            SystemQueue,
            ExternalSystem,
            ExternalSystemDb,
            ExternalSystemQueue,
            Container,
            ContainerDb,
            ContainerQueue,
            ExternalContainer,
            ExternalContainerDb,
            ExternalContainerQueue,
            Component,
            ComponentDb,
            ComponentQueue,
            ExternalComponent,
            ExternalComponentDb,
            ExternalComponentQueue,
        ]
        .map(|v| v.as_str()),
    ));
    use DiagramKind::*;
    for kind in [
        Flowchart,
        Class,
        State,
        Sequence,
        Er,
        Pie,
        Mindmap,
        Journey,
        Timeline,
        Gantt,
        Requirement,
        GitGraph,
        C4,
        Sankey,
        Quadrant,
        ZenUML,
        Block,
        Packet,
        Kanban,
        Architecture,
        Radar,
        Treemap,
        XYChart,
    ] {
        for head in [
            None,
            Some(EdgeArrowhead::OpenTriangle),
            Some(EdgeArrowhead::ClassDependency),
        ] {
            out.extend(debug(r::render::arrowhead_inset(kind, head).to_bits()));
        }
    }
    for v in [
        r::cli::LayoutEngineArg::Current,
        r::cli::LayoutEngineArg::Dagre,
        r::cli::LayoutEngineArg::Auto,
    ] {
        let engine: r::FlowchartLayoutEngine = v.into();
        out.extend(debug((engine, engine.as_str())));
    }
    let mut graph = r::Graph::new();
    graph.ensure_node("B", None, None);
    graph.ensure_node("A", Some("Alpha".into()), Some(r::NodeShape::Circle));
    graph.ensure_node("B", Some("Beta".into()), Some(r::NodeShape::Diamond));
    graph.ensure_node("B", None, None);
    out.extend(graph_bytes(&graph));
    let q = r::layout::FlowchartQualityMetrics {
        bad_source_exits: 1,
        bad_target_entries: 2,
        endpoint_node_intrusions: 3,
        non_endpoint_node_hits: 5,
        endpoint_node_reentries: 7,
        ..Default::default()
    };
    out.extend(debug((q.hard_violation_count(), q.geometry_debt_count())));
    out.extend(debug(
        r::layout::LayoutInvariantError {
            path: "probe.path".into(),
            message: "probe message".into(),
        }
        .to_string(),
    ));
    out
}

/// Scalar test ABI. Source/path must name readable UTF-8 bytes; output must
/// name `capacity` writable bytes when capacity is nonzero. Inputs and output
/// must not overlap. Insufficient capacity only returns the required size.
pub unsafe fn run(
    case: usize,
    source: *const u8,
    source_len: usize,
    path: *const u8,
    path_len: usize,
    output: *mut u8,
    capacity: usize,
) -> usize {
    let source =
        std::str::from_utf8(unsafe { std::slice::from_raw_parts(source, source_len) }).unwrap();
    let path = std::str::from_utf8(unsafe { std::slice::from_raw_parts(path, path_len) }).unwrap();
    let bytes = execute(CASES[case].0, source, path);
    if capacity != 0 && capacity >= bytes.len() {
        unsafe { std::ptr::copy_nonoverlapping(bytes.as_ptr(), output, bytes.len()) };
    }
    bytes.len()
}
