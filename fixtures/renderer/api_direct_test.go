package rendererfixture_test

import (
	"bytes"
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"math"
	. "oxide-renderer-conformance"
	"strconv"
	"strings"
)

func directAPI(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	switch name {
	case "parse", "parse_error", "strict_parse":
		if name == "strict_parse" {
			return appendParsed(ctx, dst, ParseMermaidStrict(ctx, Borrow__Str(source)), path)
		}
		return appendParsed(ctx, dst, ParseMermaid(ctx, Borrow__Str(source)), path)
	case "strict_directive", "strict_unclosed", "strict_end", "strict_arrow", "strict_click", "strict_participant":
		result := ParseMermaidStrict(ctx, Borrow__Str(source))
		defer result.Drop(ctx)
		if result.Ref().Variant().String() != "Err" {
			panic("strict diagnostic unexpectedly succeeded")
		}
		value := result.Ref().Field__Err__0()
		debug := value.Debug(ctx)
		defer debug.Drop(ctx)
		display := value.Display(ctx)
		defer display.Drop(ctx)
		cause := ParseError_As_Core_Error_Error_Source(ctx, value)
		defer cause.Drop(ctx)
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, debug.Ref())
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, display.Ref())
		dst = append(dst, ',', ' ')
		dst = strconv.AppendBool(dst, cause.Ref().Variant().String() == "Some")
		return append(dst, ')')
	case "render", "render_error":
		return appendStringResult(ctx, dst, Render(ctx, Borrow__Str(source)), path)
	case "render_options", "render_init":
		options := RenderOptions_MermaidDefault(ctx)
		options.Mut().Field__Theme().Replace(ctx, Theme_MermaidDefault(ctx))
		layout := options.Mut().Field__Layout()
		layout.Field__NodeSpacing().Set(37)
		layout.Field__RankSpacing().Set(63)
		layout.Field__PreferredAspectRatio().Replace(ctx, someFloat(ctx, 1.5))
		return appendStringResult(ctx, dst, RenderWithOptions(ctx, Borrow__Str(source), options), path)
	case "render_strict", "render_strict_error":
		return appendStringResult(ctx, dst, RenderStrict(ctx, Borrow__Str(source), RenderOptions_Modern(ctx)), path)
	case "validate", "validate_error":
		return appendDebugResult(ctx, dst, Validator_Validate(ctx, Borrow__Str(source)), path)
	case "measure", "measure_error":
		return appendDebugResult(ctx, dst, Measure(ctx, Borrow__Str(source), Default__RenderOptions(ctx)), path)
	case "scene", "scene_class", "scene_error":
		result := RenderScene(ctx, Borrow__Str(source), RenderOptions_Modern(ctx))
		defer result.Drop(ctx)
		if result.Ref().Variant().String() == "Err" {
			return appendError(ctx, dst, result.Ref().Field__Err__0(), path)
		}
		scene := result.Ref().Field__Ok__0()
		rebuilt := New__Scene(ctx)
		rebuilt.Mut().Field__Width().Set(scene.Field__Width().Get())
		rebuilt.Mut().Field__Height().Set(scene.Field__Height().Get())
		commands := scene.Field__Commands()
		rebuilt.Mut().Field__Commands().Init(Vec__Alloc_Vec_Vec__Of__SceneCommand__End(ctx, commands.Len()))
		defer rebuilt.Drop(ctx)
		output := rebuilt.Mut().Field__Commands()
		for i := uintptr(0); i < commands.Len(); i++ {
			element := commands.Index(i)
			switch element.Variant() {
			case Variant__SceneCommand__FillPath, Variant__SceneCommand__PushClip, Variant__SceneCommand__PopClip, Variant__SceneCommand__PushLayer, Variant__SceneCommand__PopLayer:
			default:
				panic("unknown scene command")
			}
			output.InitAt(i, SceneCommand_As_Core_Clone_Clone_Clone(ctx, element))
			output.SetLen(i + 1)
		}
		return appendDebug(ctx, dst, rebuilt.Ref())
	case "write_svg", "write_svg_error", "write_png", "write_png_invalid", "write_png_error":
		var result Value__Core_Result_Result__Of__Unit__And__Anyhow_Error__End
		if strings.HasPrefix(name, "write_svg") {
			result = WriteOutputSvg(ctx, Borrow__Str(source), optionalPath(ctx, path, true))
		} else {
			config := Default__RenderConfig(ctx)
			defer config.Drop(ctx)
			theme := Theme_Modern(ctx)
			defer theme.Drop(ctx)
			result = WriteOutputPng(ctx, Borrow__Str(source), Borrow__Std_Path_Path(path), config.Ref(), theme.Ref())
		}
		defer result.Drop(ctx)
		if result.Ref().Variant().String() == "Err" {
			return appendError(ctx, dst, result.Ref().Field__Err__0(), path)
		}
		return dst
	case "config_none", "config_file", "config_json_error", "config_missing", "config_theme", "config_theme_error":
		var result Value__Core_Result_Result__Of__Config__And__Anyhow_Error__End
		if name == "config_theme" || name == "config_theme_error" {
			theme, some := "neutral", true
			if name == "config_theme_error" {
				theme, some = "unrecognized-theme", false
			}
			result = Config_LoadConfigWithTheme(ctx, optionalPath(ctx, path, some), optionalStr(ctx, ctx.CopyString(theme), true))
		} else {
			result = Config_LoadConfig(ctx, optionalPath(ctx, path, name != "config_none"))
		}
		return appendDebugResult(ctx, dst, result, path)
	case "merge_config", "merge_nonobject":
		json := FromJSON__SerdeJson_Value_Value(ctx, Borrow__Str(source))
		if json.Ref().Variant().String() != "Ok" {
			panic("native merge input failed to deserialize")
		}
		// Moving the payload leaves this Result uninitialized; do not drop its old bits.
		value := MergeInitConfig(ctx, Default__Config(ctx), json.Mut().Field__Ok__0().Move(ctx))
		defer value.Drop(ctx)
		return appendDebug(ctx, dst, value.Ref())
	case "aspect_ratios":
		dst = append(dst, '[')
		rest := source.String()
		first := true
		for {
			token, next, more := strings.Cut(rest, "|")
			if !first {
				dst = append(dst, ',', ' ')
			}
			first = false
			value := Config_ParseAspectRatioValue(ctx, Borrow__Str(ctx.CopyString(token)))
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
			if !more {
				break
			}
			rest = next
		}
		return append(dst, ']')
	case "theme_methods":
		dst = append(dst, "{\"lookups\":["...)
		rest := source.String()
		first := true
		for {
			token, next, more := strings.Cut(rest, "|")
			if !first {
				dst = append(dst, ',')
			}
			first = false
			value := Theme_FromName(ctx, Borrow__Str(ctx.CopyString(token)))
			dst = appendJSONValue(ctx, dst, value.Ref())
			value.Drop(ctx)
			if !more {
				break
			}
			rest = next
		}
		dst = append(dst, "],\"presets\":["...)
		for i, f := range [...]func(*oxide.Context) Value__Theme{Theme_Modern, Theme_MermaidDefault, Theme_Dark, Theme_Forest, Theme_Neutral} {
			if i > 0 {
				dst = append(dst, ',')
			}
			value := f(ctx)
			dst = appendJSONValue(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		return append(dst, ']', '}')
	case "serde_traits":
		theme := Theme_Forest(ctx)
		defer theme.Drop(ctx)
		encoded := theme.Ref().JSON(ctx)
		defer encoded.Drop(ctx)
		decoded := FromJSON__Theme(ctx, apiOK(ctx, encoded).Borrow())
		defer decoded.Drop(ctx)
		roundtrip := apiOK(ctx, decoded).JSON(ctx)
		defer roundtrip.Drop(ctx)
		if !bytes.Equal(apiOK(ctx, encoded).Bytes(), apiOK(ctx, roundtrip).Bytes()) {
			panic("Theme JSON changed")
		}
		dst = append(dst, apiOK(ctx, encoded).Bytes()...)
		config := Default__LayoutConfig(ctx)
		defer config.Drop(ctx)
		configJSON := config.Ref().JSON(ctx)
		defer configJSON.Drop(ctx)
		configDecoded := FromJSON__LayoutConfig(ctx, apiOK(ctx, configJSON).Borrow())
		defer configDecoded.Drop(ctx)
		configAgain := apiOK(ctx, configDecoded).JSON(ctx)
		defer configAgain.Drop(ctx)
		if !bytes.Equal(apiOK(ctx, configJSON).Bytes(), apiOK(ctx, configAgain).Bytes()) {
			panic("LayoutConfig JSON changed")
		}
		dst = append(dst, apiOK(ctx, configJSON).Bytes()...)
		clone := LayoutConfig_As_Core_Clone_Clone_Clone(ctx, config.Ref())
		dst = appendDebug(ctx, dst, clone.Ref())
		clone.Drop(ctx)
		graph := Default__Graph(ctx)
		defer graph.Drop(ctx)
		return appendGraph(ctx, dst, graph.Ref())
	case "render_svg", "render_svg_dimensions", "layout", "layout_valid", "layout_invalid", "quality", "quality_other":
		parsed, theme, config, layout := layoutValues(ctx, source)
		defer parsed.Drop(ctx)
		defer theme.Drop(ctx)
		defer config.Drop(ctx)
		defer layout.Drop(ctx)
		switch name {
		case "render_svg", "render_svg_dimensions":
			var value Value__Alloc_String_String
			if name == "render_svg" {
				value = RenderSvg(ctx, layout.Ref(), theme.Ref(), config.Ref())
			} else {
				value = Render_RenderSvgWithDimensions(ctx, layout.Ref(), theme.Ref(), config.Ref(), dimensions(ctx, 640, 480))
			}
			defer value.Drop(ctx)
			return append(dst, value.Ref().Bytes()...)
		case "layout":
			dump := LayoutDump_LayoutDump_FromLayout(ctx, layout.Ref(), apiOK(ctx, parsed).Field__Graph())
			defer dump.Drop(ctx)
			return appendJSON(ctx, dst, dump.Ref())
		case "layout_valid", "layout_invalid":
			if name == "layout_invalid" {
				layout.Mut().Field__Width().Set(float32(math.NaN()))
				layout.Mut().Field__Height().Set(-1)
				it := layout.Mut().Field__Nodes().Iter(ctx)
				first := it.Mut().Next(ctx)
				first.Mut().Field__Some__0().Field__1().Deref().Field__Width().Set(0)
				first.Drop(ctx)
				it.Drop(ctx)
				// The concrete tuple contains only two f32 scalars; truncation owns no drops.
				var points Mut__Alloc_Vec_Vec__Of__Tuple__Of__F32__And__F32__End__End = layout.Mut().Field__Edges().Index(0).Field__Points()
				points.SetLen(0)
			}
			result := Layout_ValidateLayoutInvariants(ctx, layout.Ref())
			defer result.Drop(ctx)
			if result.Ref().Variant().String() == "Ok" {
				return append(dst, "valid"...)
			}
			items := result.Ref().Field__Err__0()
			dst = append(dst, '[')
			for i := uintptr(0); i < items.Len(); i++ {
				if i != 0 {
					dst = append(dst, ',', ' ')
				}
				value := items.Index(i)
				message := value.Display(ctx)
				dst = append(dst, '(')
				dst = appendDebug(ctx, dst, value.Field__Path())
				dst = append(dst, ',', ' ')
				dst = appendDebug(ctx, dst, value.Field__Message())
				dst = append(dst, ',', ' ')
				dst = appendDebug(ctx, dst, message.Ref())
				dst = append(dst, ')')
				message.Drop(ctx)
			}
			return append(dst, ']')
		case "quality", "quality_other":
			result := Layout_FlowchartQualityMetrics(ctx, layout.Ref())
			defer result.Drop(ctx)
			if result.Ref().Variant().String() == "None" {
				return append(dst, "None"...)
			}
			value := result.Ref().Field__Some__0()
			hard := Layout_FlowchartQualityMetrics_HardViolationCount(ctx, value)
			debt := Layout_FlowchartQualityMetrics_GeometryDebtCount(ctx, value)
			dst = append(dst, "Some(("...)
			dst = appendDebug(ctx, dst, value)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendUint(dst, uint64(hard), 10)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendUint(dst, uint64(debt), 10)
			return append(dst, "))"...)
		}
	case "measure_dimensions":
		parsed, theme, config, layout := layoutValues(ctx, source)
		defer parsed.Drop(ctx)
		defer theme.Drop(ctx)
		defer config.Drop(ctx)
		defer layout.Drop(ctx)
		measured := MeasureWithDimensions(ctx, Borrow__Str(source), Default__RenderOptions(ctx), dimensions(ctx, 321, 123))
		defer measured.Drop(ctx)
		direct := MeasureSvgDimensions(ctx, layout.Ref(), config.Ref(), dimensions(ctx, 321, 123))
		defer direct.Drop(ctx)
		a, b := apiOK(ctx, measured), direct.Ref()
		if a.Field__Width().Get() != b.Field__Width().Get() || a.Field__Height().Get() != b.Field__Height().Get() || a.Field__ViewboxX().Get() != b.Field__ViewboxX().Get() || a.Field__ViewboxY().Get() != b.Field__ViewboxY().Get() || a.Field__ViewboxWidth().Get() != b.Field__ViewboxWidth().Get() || a.Field__ViewboxHeight().Get() != b.Field__ViewboxHeight().Get() {
			panic("dimension APIs disagree")
		}
		return appendJSON(ctx, dst, a)
	case "dimension_methods":
		dst = append(dst, '[')
		for i, height := range [...]float32{0, -1, 0.5, 12} {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			value := New__SvgDimensions(ctx)
			value.Mut().Field__Width().Set(24)
			value.Mut().Field__Height().Set(height)
			value.Mut().Field__ViewboxX().Set(0)
			value.Mut().Field__ViewboxY().Set(0)
			value.Mut().Field__ViewboxWidth().Set(36)
			value.Mut().Field__ViewboxHeight().Set(height)
			dst = append(dst, '(')
			// SvgDimensions is Copy in Rust; both methods take self by value.
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(SvgDimensions_AspectRatio(ctx, value))), 10)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(SvgDimensions_ViewboxAspectRatio(ctx, value))), 10)
			dst = append(dst, ')')
		}
		return append(dst, ']')
	case "option_methods":
		dst = append(dst, '[')
		first := true
		for _, ratio := range [...]float32{2, 0, -1, float32(math.NaN()), float32(math.Inf(1))} {
			values := [3]Value__RenderOptions{RenderOptions_WithPreferredAspectRatio(ctx, RenderOptions_WithRankSpacing(ctx, RenderOptions_WithNodeSpacing(ctx, RenderOptions_Modern(ctx), 33), 55), ratio), RenderOptions_WithPreferredAspectRatioParts(ctx, RenderOptions_MermaidDefault(ctx), ratio, 3), RenderOptions_WithPreferredAspectRatioParts(ctx, Default__RenderOptions(ctx), 3, ratio)}
			for _, value := range values {
				if !first {
					dst = append(dst, ',', ' ')
				}
				first = false
				dst = appendDebug(ctx, dst, value.Ref())
				value.Drop(ctx)
			}
		}
		return append(dst, ']')
	default:
		return directMore(ctx, name, source, path, dst)
	}
	panic("unhandled direct API state: " + name)
}
