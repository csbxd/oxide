package rendererfixture_test

import (
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"math"
	. "oxide-renderer-conformance"
	"strconv"
	"strings"
)

func stageTotal(ctx *oxide.Context, v Ref__LayoutStageMetrics) {
	total := oxide.U128{}
	for _, value := range [...]oxide.U128{v.Field__LayeredLayoutUs().Get(), v.Field__PortAssignmentUs().Get(), v.Field__EdgeRoutingUs().Get(), v.Field__LabelPlacementUs().Get()} {
		total = oxide.U128AddValue(total, value)
	}
	if LayoutStageMetrics_TotalUs(ctx, v) != total {
		panic("stage timing sum differs")
	}
}
func checkTiming(parse, layout, render, total oxide.U128, ms float64) {
	sum := oxide.U128AddValue(oxide.U128AddValue(parse, layout), render)
	if total != sum || ms != oxide.U128ToF64(total)/1000 {
		panic("render timing relations differ")
	}
}
func appendU128(ctx *oxide.Context, dst []byte, n oxide.U128) []byte {
	value := New__U128(ctx)
	value.Mut().Set(n)
	return appendDebug(ctx, dst, value.Ref())
}
func directMore(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	switch name {
	case "timing", "timing_error":
		result := RenderWithTiming(ctx, Borrow__Str(source), Default__RenderOptions(ctx))
		defer result.Drop(ctx)
		if result.Ref().Variant().String() == "Err" {
			return appendError(ctx, dst, result.Ref().Field__Err__0(), path)
		}
		value := result.Ref().Field__Ok__0()
		checkTiming(value.Field__ParseUs().Get(), value.Field__LayoutUs().Get(), value.Field__RenderUs().Get(), RenderResult_TotalUs(ctx, value), RenderResult_TotalMs(ctx, value))
		dst = append(dst, "timing relations checked\n"...)
		return append(dst, value.Field__Svg().Bytes()...)
	case "detailed_timing", "detailed_timing_error":
		result := RenderWithDetailedTiming(ctx, Borrow__Str(source), Default__RenderOptions(ctx))
		defer result.Drop(ctx)
		if result.Ref().Variant().String() == "Err" {
			return appendError(ctx, dst, result.Ref().Field__Err__0(), path)
		}
		value := result.Ref().Field__Ok__0()
		checkTiming(value.Field__ParseUs().Get(), value.Field__LayoutUs().Get(), value.Field__RenderUs().Get(), RenderDetailedResult_TotalUs(ctx, value), RenderDetailedResult_TotalMs(ctx, value))
		stageTotal(ctx, value.Field__LayoutStages())
		dst = append(dst, "timing/stage relations checked\n"...)
		dst = append(dst, value.Field__Svg().Bytes()...)
		return appendJSON(ctx, dst, value.Field__LayoutStages().Field__LayeredLayout())
	case "layout_dump", "layout_dump_error":
		parsed, theme, config, layout := layoutValues(ctx, source)
		defer parsed.Drop(ctx)
		defer theme.Drop(ctx)
		defer config.Drop(ctx)
		defer layout.Drop(ctx)
		status := LayoutDump_WriteLayoutDump(ctx, Borrow__Std_Path_Path(path), layout.Ref(), apiOK(ctx, parsed).Field__Graph())
		defer status.Drop(ctx)
		if status.Ref().Variant().String() == "Err" {
			return appendError(ctx, dst, status.Ref().Field__Err__0(), path)
		}
		return dst
	case "layout_metrics", "layout_metrics_sequence", "layered_dump", "layered_dump_error":
		parsed := ParseMermaid(ctx, Borrow__Str(source))
		defer parsed.Drop(ctx)
		graph := apiOK(ctx, parsed).Field__Graph()
		theme := Theme_Modern(ctx)
		defer theme.Drop(ctx)
		config := Default__LayoutConfig(ctx)
		defer config.Drop(ctx)
		config.Mut().Field__Flowchart().Field__Engine().Replace(ctx, New__FlowchartLayoutEngine__Dagre(ctx))
		pair := ComputeLayoutWithMetrics(ctx, graph, theme.Ref(), config.Ref())
		defer pair.Drop(ctx)
		layout, metrics := pair.Ref().Field__0(), pair.Ref().Field__1()
		stageTotal(ctx, metrics)
		if strings.HasPrefix(name, "layered_dump") {
			snapshot := metrics.Field__LayeredLayout()
			if snapshot.Variant().String() != "Some" {
				panic("missing flowchart snapshot")
			}
			status := WriteLayeredLayoutDump(ctx, Borrow__Std_Path_Path(path), snapshot.Field__Some__0())
			defer status.Drop(ctx)
			if status.Ref().Variant().String() == "Err" {
				return appendError(ctx, dst, status.Ref().Field__Err__0(), path)
			}
			return dst
		}
		dump := LayoutDump_LayoutDump_FromLayout(ctx, layout, graph)
		defer dump.Drop(ctx)
		dst = appendJSON(ctx, dst, dump.Ref())
		return appendJSON(ctx, dst, metrics.Field__LayeredLayout())
	case "timing_methods":
		result := New__RenderResult(ctx)
		result.Mut().Field__Svg().Init(String__Alloc_String_String(ctx, ""))
		result.Mut().Field__ParseUs().Set(oxide.U128{Hi: 2})
		result.Mut().Field__LayoutUs().Set(oxide.U128{Lo: 7})
		result.Mut().Field__RenderUs().Set(oxide.U128{Lo: 9})
		defer result.Drop(ctx)
		detail := New__RenderDetailedResult(ctx)
		detail.Mut().Field__Svg().Init(String__Alloc_String_String(ctx, ""))
		detail.Mut().Field__ParseUs().Set(oxide.U128{Lo: 23})
		detail.Mut().Field__LayoutUs().Set(oxide.U128{Lo: 29})
		detail.Mut().Field__RenderUs().Set(oxide.U128{Lo: 31})
		defer detail.Drop(ctx)
		stages := detail.Mut().Field__LayoutStages()
		stages.Field__LayeredLayoutUs().Set(oxide.U128{Lo: 11})
		stages.Field__PortAssignmentUs().Set(oxide.U128{Lo: 13})
		stages.Field__EdgeRoutingUs().Set(oxide.U128{Lo: 17})
		stages.Field__LabelPlacementUs().Set(oxide.U128{Lo: 19})
		stages.Field__LayeredLayout().Init(New__Core_Option_Option__Of__LayeredLayoutSnapshot__End__None(ctx))
		dst = append(dst, '(')
		dst = appendU128(ctx, dst, RenderResult_TotalUs(ctx, result.Ref()))
		dst = append(dst, ',', ' ')
		dst = strconv.AppendUint(dst, math.Float64bits(RenderResult_TotalMs(ctx, result.Ref())), 10)
		dst = append(dst, ',', ' ')
		dst = appendU128(ctx, dst, RenderDetailedResult_TotalUs(ctx, detail.Ref()))
		dst = append(dst, ',', ' ')
		dst = strconv.AppendUint(dst, math.Float64bits(RenderDetailedResult_TotalMs(ctx, detail.Ref())), 10)
		dst = append(dst, ',', ' ')
		dst = appendU128(ctx, dst, LayoutStageMetrics_TotalUs(ctx, stages.Ref()))
		return append(dst, ')')
	case "config_defaults":
		config := Default__Config(ctx)
		options := Default__RenderOptions(ctx)
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, config.Ref())
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, options.Ref())
		dst = append(dst, ')')
		config.Drop(ctx)
		options.Drop(ctx)
		// Each default has a distinct Rust type and a concrete Go call site.
		{
			value := Default__LayoutConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__RenderConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_RequirementConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_MindmapConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_GitGraphConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_C4Config(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_PieConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_TreemapConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_TimelineConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_FlowchartLayoutConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_FlowchartObjectiveConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_FlowchartAutoSpacingConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		{
			value := Default__Config_FlowchartRoutingConfig(ctx)
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		cfg := Default__LayoutConfig(ctx)
		defer cfg.Drop(ctx)
		requirement := cfg.Ref().Field__Requirement()
		values := [3]float32{LayoutConfig_ClassLabelLineHeight(ctx, cfg.Ref()), Config_RequirementConfig_DividerOffset(ctx, requirement, 10, 12), Config_RequirementConfig_DividerOffset(ctx, requirement, 100, 120)}
		dst = append(dst, '(')
		for i, x := range values {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			value := New__F32(ctx)
			value.Mut().Set(x)
			dst = appendDebug(ctx, dst, value.Ref())
		}
		return append(dst, ')')
	default:
		return directConstructors(ctx, name, source, path, dst)
	}
}
