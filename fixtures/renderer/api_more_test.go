package rendererfixture_test

import (
	"math"
	. "oxide-renderer-conformance"
	"strconv"
	"strings"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func stageTotal(ctx *oxide.Context, v oxide.Value) {
	total := oxide.U128{}
	for _, field := range [...]string{"layered_layout_us", "port_assignment_us", "edge_routing_us", "label_placement_us"} {
		total = oxide.U128AddValue(total, v.Field(field).Uint128())
	}
	if LayoutStageMetrics_TotalUs(ctx, v) != total {
		panic("stage timing sum differs")
	}
}

func appendU128(ctx *oxide.Context, dst []byte, n oxide.U128) []byte {
	v := RenderResult_TotalUsTypes.Result.Uninit(ctx)
	v.SetUint128(n)
	return appendDebug(ctx, dst, v)
}

func directMore(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	switch name {
	case "timing", "timing_error", "detailed_timing", "detailed_timing_error":
		detailed := strings.HasPrefix(name, "detailed_")
		var result oxide.Value
		if detailed {
			result = RenderWithDetailedTiming(ctx, source, TypeRenderOptions.Default(ctx))
		} else {
			result = RenderWithTiming(ctx, source, TypeRenderOptions.Default(ctx))
		}
		defer result.Drop(ctx)
		if result.Variant() == "Err" {
			return appendError(ctx, dst, result.Field("0"), path)
		}
		v := result.Field("0")
		sum := oxide.U128AddValue(oxide.U128AddValue(v.Field("parse_us").Uint128(), v.Field("layout_us").Uint128()), v.Field("render_us").Uint128())
		var total oxide.U128
		var ms float64
		if detailed {
			total = RenderDetailedResult_TotalUs(ctx, v)
			ms = RenderDetailedResult_TotalMs(ctx, v)
		} else {
			total = RenderResult_TotalUs(ctx, v)
			ms = RenderResult_TotalMs(ctx, v)
		}
		if total != sum || ms != oxide.U128ToF64(total)/1000 {
			panic("render timing relations differ")
		}
		if detailed {
			stageTotal(ctx, v.Field("layout_stages"))
			dst = append(dst, "timing/stage relations checked\n"...)
		} else {
			dst = append(dst, "timing relations checked\n"...)
		}
		dst = append(dst, v.Field("svg").Bytes()...)
		if detailed {
			dst = appendJSON(ctx, dst, v.Field("layout_stages").Field("layered_layout"))
		}
		return dst
	case "layout_dump", "layout_dump_error":
		parsed, theme, config, layout := layoutValues(ctx, source)
		defer parsed.Drop(ctx)
		defer theme.Drop(ctx)
		defer config.Drop(ctx)
		defer layout.Drop(ctx)
		status := LayoutDump_WriteLayoutDump(ctx, path, layout, parsed.Field("0").Field("graph"))
		defer status.Drop(ctx)
		if status.Variant() == "Err" {
			return appendError(ctx, dst, status.Field("0"), path)
		}
		return dst
	case "layout_metrics", "layout_metrics_sequence", "layered_dump", "layered_dump_error":
		parsed := ParseMermaid(ctx, source)
		defer parsed.Drop(ctx)
		graph := resultOK(ctx, parsed).Field("graph")
		theme := Theme_Modern(ctx)
		defer theme.Drop(ctx)
		config := TypeLayoutConfig.Default(ctx)
		defer config.Drop(ctx)
		engine := config.Field("flowchart").Field("engine")
		engine.Replace(ctx, engine.Type.Enum(ctx, "Dagre"))
		pair := ComputeLayoutWithMetrics(ctx, graph, theme, config)
		defer pair.Drop(ctx)
		layout, metrics := pair.Field("0"), pair.Field("1")
		stageTotal(ctx, metrics)
		if strings.HasPrefix(name, "layered_dump") {
			snapshot := metrics.Field("layered_layout")
			if snapshot.Variant() != "Some" {
				panic("missing flowchart snapshot")
			}
			status := WriteLayeredLayoutDump(ctx, path, snapshot.Field("0"))
			defer status.Drop(ctx)
			if status.Variant() == "Err" {
				return appendError(ctx, dst, status.Field("0"), path)
			}
			return dst
		}
		dump := LayoutDump_LayoutDump_FromLayout(ctx, layout, graph)
		defer dump.Drop(ctx)
		dst = appendJSON(ctx, dst, dump)
		return appendJSON(ctx, dst, metrics.Field("layered_layout"))
	case "timing_methods":
		result := TypeRenderResult.Uninit(ctx)
		result.Field("svg").Init(result.Field("svg").Type.String(ctx, ""))
		result.Field("parse_us").SetUint128(oxide.U128{Hi: 2})
		result.Field("layout_us").SetUint128(oxide.U128{Lo: 7})
		result.Field("render_us").SetUint128(oxide.U128{Lo: 9})
		defer result.Drop(ctx)
		detail := TypeRenderDetailedResult.Uninit(ctx)
		detail.Field("svg").Init(detail.Field("svg").Type.String(ctx, ""))
		detail.Field("parse_us").SetUint128(oxide.U128{Lo: 23})
		detail.Field("layout_us").SetUint128(oxide.U128{Lo: 29})
		detail.Field("render_us").SetUint128(oxide.U128{Lo: 31})
		defer detail.Drop(ctx)
		stages := detail.Field("layout_stages")
		for i, field := range [...]string{"layered_layout_us", "port_assignment_us", "edge_routing_us", "label_placement_us"} {
			stages.Field(field).SetUint128(oxide.U128{Lo: [4]uint64{11, 13, 17, 19}[i]})
		}
		stages.Field("layered_layout").Init(stages.Field("layered_layout").Type.Enum(ctx, "None"))
		dst = append(dst, '(')
		dst = appendU128(ctx, dst, RenderResult_TotalUs(ctx, result))
		dst = append(dst, ',', ' ')
		dst = strconv.AppendUint(dst, math.Float64bits(RenderResult_TotalMs(ctx, result)), 10)
		dst = append(dst, ',', ' ')
		dst = appendU128(ctx, dst, RenderDetailedResult_TotalUs(ctx, detail))
		dst = append(dst, ',', ' ')
		dst = strconv.AppendUint(dst, math.Float64bits(RenderDetailedResult_TotalMs(ctx, detail)), 10)
		dst = append(dst, ',', ' ')
		dst = appendU128(ctx, dst, LayoutStageMetrics_TotalUs(ctx, stages))
		return append(dst, ')')
	case "config_defaults":
		config := TypeConfig.Default(ctx)
		options := TypeRenderOptions.Default(ctx)
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, config)
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, options)
		dst = append(dst, ')')
		config.Drop(ctx)
		options.Drop(ctx)
		for _, typ := range [...]*oxide.Type{TypeLayoutConfig, TypeRenderConfig, TypeConfig_RequirementConfig, TypeConfig_MindmapConfig, TypeConfig_GitGraphConfig, TypeConfig_C4Config, TypeConfig_PieConfig, TypeConfig_TreemapConfig, TypeConfig_TimelineConfig, TypeConfig_FlowchartLayoutConfig, TypeConfig_FlowchartObjectiveConfig, TypeConfig_FlowchartAutoSpacingConfig, TypeConfig_FlowchartRoutingConfig} {
			v := typ.Default(ctx)
			dst = appendDebug(ctx, dst, v)
			v.Drop(ctx)
		}
		cfg := TypeLayoutConfig.Default(ctx)
		defer cfg.Drop(ctx)
		requirement := cfg.Field("requirement")
		values := [3]float32{LayoutConfig_ClassLabelLineHeight(ctx, cfg), Config_RequirementConfig_DividerOffset(ctx, requirement, 10, 12), Config_RequirementConfig_DividerOffset(ctx, requirement, 100, 120)}
		dst = append(dst, '(')
		for i, x := range values {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			v := LayoutConfig_ClassLabelLineHeightTypes.Result.Uninit(ctx)
			v.SetFloat(float64(x))
			dst = appendDebug(ctx, dst, v)
		}
		return append(dst, ')')
	default:
		return directConstructors(ctx, name, source, path, dst)
	}
}
