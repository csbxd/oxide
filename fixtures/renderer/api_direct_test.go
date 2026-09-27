package rendererfixture_test

import (
	"bytes"
	"cmp"
	"math"
	. "oxide-renderer-conformance"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func variantField(t *oxide.Type, name string, index int) *oxide.Type {
	for _, v := range t.Variants {
		if v.Name == name {
			return v.Fields[index].Type
		}
	}
	panic("missing Rust variant: " + name)
}

func someFloat(ctx *oxide.Context, t *oxide.Type, n float64) oxide.Value {
	v := variantField(t, "Some", 0).Uninit(ctx)
	v.SetFloat(n)
	return t.Enum(ctx, "Some", v)
}

func dimensions(ctx *oxide.Context, t *oxide.Type, width, height float64) oxide.Value {
	v := variantField(t, "Some", 0).Uninit(ctx)
	v.Field("0").SetFloat(width)
	v.Field("1").SetFloat(height)
	return t.Enum(ctx, "Some", v)
}

func optionalSpan(ctx *oxide.Context, t *oxide.Type, span oxide.Span, some bool) oxide.Value {
	if !some {
		return t.Enum(ctx, "None")
	}
	p := variantField(t, "Some", 0).Uninit(ctx)
	p.SetRef(oxide.Value{Addr: span.Data, Meta: span.Len, Type: p.Type.Elem})
	return t.Enum(ctx, "Some", p)
}

func appendJSON(ctx *oxide.Context, dst []byte, value oxide.Value) []byte {
	result := value.JSON(ctx)
	defer result.Drop(ctx)
	return append(dst, resultOK(ctx, result).Bytes()...)
}

// json! first serializes into serde_json::Value: object keys are ordered and
// f32 numbers are widened before final JSON formatting. Keep that Rust path.
func appendJSONValue(ctx *oxide.Context, dst []byte, value oxide.Value) []byte {
	result := value.JSONValue(ctx)
	defer result.Drop(ctx)
	return appendJSON(ctx, dst, resultOK(ctx, result))
}

type observedEntry struct{ key, value oxide.Value }

func mapEntries(ctx *oxide.Context, value oxide.Value) []observedEntry {
	mark := ctx.Mark()
	it := value.Iterator(ctx)
	count := 0
	for {
		m := ctx.Mark()
		next := it.Next(ctx)
		done := next.Variant() == "None"
		next.Drop(ctx)
		ctx.Restore(m)
		if done {
			break
		}
		count++
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	p := ctx.Alloc(uintptr(count)*unsafe.Sizeof(observedEntry{}), unsafe.Alignof(observedEntry{}))
	entries := unsafe.Slice((*observedEntry)(unsafe.Pointer(p)), count)
	mark = ctx.Mark()
	it = value.Iterator(ctx)
	for i := 0; i < count; i++ {
		m := ctx.Mark()
		next := it.Next(ctx)
		pair := next.Field("0")
		entries[i] = observedEntry{pair.Field("0").Deref(), pair.Field("1").Deref()}
		next.Drop(ctx)
		ctx.Restore(m)
	}
	it.Drop(ctx)
	ctx.Restore(mark)
	slices.SortFunc(entries, func(a, b observedEntry) int {
		if a.key.Type.Kind == "usize" {
			return cmp.Compare(a.key.Uint(), b.key.Uint())
		}
		return strings.Compare(a.key.String(), b.key.String())
	})
	return entries
}

func appendGraph(ctx *oxide.Context, dst []byte, graph oxide.Value) []byte {
	graph = Graph_As_Core_Clone_Clone_Clone(ctx, graph)
	defer graph.Drop(ctx)
	for _, name := range [...]string{"node_order", "class_defs", "node_classes", "node_styles", "subgraph_styles", "subgraph_classes", "node_links", "edge_styles", "arch_edge_ports"} {
		field := graph.Field(name)
		entries := mapEntries(ctx, field)
		dst = append(dst, name...)
		dst = append(dst, '=', '{')
		for i, e := range entries {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = appendDebug(ctx, dst, e.key)
			dst = append(dst, ':', ' ')
			dst = appendDebug(ctx, dst, e.value)
		}
		dst = append(dst, '}', '\n')
		field.Replace(ctx, field.Type.Default(ctx))
	}
	dst = appendDebug(ctx, dst, graph)
	return append(dst, '\n')
}

func appendParsed(ctx *oxide.Context, dst []byte, result oxide.Value, path oxide.Span) []byte {
	defer result.Drop(ctx)
	if result.Variant() == "Err" {
		return appendError(ctx, dst, result.Field("0"), path)
	}
	v := result.Field("0")
	dst = appendGraph(ctx, dst, v.Field("graph"))
	return appendDebug(ctx, dst, v.Field("init_config"))
}

func layoutValues(ctx *oxide.Context, source oxide.Span) (oxide.Value, oxide.Value, oxide.Value, oxide.Value) {
	parsed := ParseMermaid(ctx, source)
	graph := resultOK(ctx, parsed).Field("graph")
	theme := Theme_Modern(ctx)
	config := TypeLayoutConfig.Default(ctx)
	layout := ComputeLayout(ctx, graph, theme, config)
	return parsed, theme, config, layout
}

func appendDebug(ctx *oxide.Context, dst []byte, value oxide.Value) []byte {
	text := value.Debug(ctx)
	dst = append(dst, text.Bytes()...)
	text.Drop(ctx)
	return dst
}

func appendError(ctx *oxide.Context, dst []byte, value oxide.Value, path oxide.Span) []byte {
	text := value.Display(ctx)
	defer text.Drop(ctx)
	dst = append(dst, "error:"...)
	b := text.Bytes()
	for {
		i := bytes.Index(b, path.Bytes())
		if i < 0 {
			return append(dst, b...)
		}
		dst = append(dst, b[:i]...)
		dst = append(dst, "<path>"...)
		b = b[i+int(path.Len):]
	}
}

func appendResult(ctx *oxide.Context, dst []byte, result oxide.Value, path oxide.Span, stringOK bool) []byte {
	defer result.Drop(ctx)
	value := result.Field("0")
	if result.Variant() == "Err" {
		return appendError(ctx, dst, value, path)
	}
	if stringOK {
		return append(dst, value.Bytes()...)
	}
	return appendDebug(ctx, dst, value)
}

func directAPI(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	switch name {
	case "parse", "parse_error", "strict_parse":
		if name == "strict_parse" {
			return appendParsed(ctx, dst, ParseMermaidStrict(ctx, source), path)
		}
		return appendParsed(ctx, dst, ParseMermaid(ctx, source), path)
	case "strict_directive", "strict_unclosed", "strict_end", "strict_arrow", "strict_click", "strict_participant":
		result := ParseMermaidStrict(ctx, source)
		defer result.Drop(ctx)
		if result.Variant() != "Err" {
			panic("strict diagnostic unexpectedly succeeded")
		}
		value := result.Field("0")
		debug := value.Debug(ctx)
		defer debug.Drop(ctx)
		display := value.Display(ctx)
		defer display.Drop(ctx)
		cause := ParseError_As_Core_Error_Error_Source(ctx, value)
		defer cause.Drop(ctx)
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, debug)
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, display)
		dst = append(dst, ',', ' ')
		dst = strconv.AppendBool(dst, cause.Variant() == "Some")
		return append(dst, ')')
	case "render", "render_error":
		return appendResult(ctx, dst, Render(ctx, source), path, true)
	case "render_options", "render_init":
		options := RenderOptions_MermaidDefault(ctx)
		// Replace an owned field, then mutate nested scalars and an enum using
		// compiler type information. Rust receives the same options as the oracle.
		options.Field("theme").Replace(ctx, Theme_MermaidDefault(ctx))
		layout := options.Field("layout")
		layout.Field("node_spacing").SetFloat(37)
		layout.Field("rank_spacing").SetFloat(63)
		ratio := layout.Field("preferred_aspect_ratio")
		ratio.Replace(ctx, someFloat(ctx, ratio.Type, 1.5))
		return appendResult(ctx, dst, RenderWithOptions(ctx, source, options), path, true)
	case "render_strict", "render_strict_error":
		return appendResult(ctx, dst, RenderStrict(ctx, source, RenderOptions_Modern(ctx)), path, true)
	case "validate", "validate_error":
		return appendResult(ctx, dst, Validator_Validate(ctx, source), path, false)
	case "measure", "measure_error":
		return appendResult(ctx, dst, Measure(ctx, source, TypeRenderOptions.Default(ctx)), path, false)
	case "scene", "scene_class", "scene_error":
		result := RenderScene(ctx, source, RenderOptions_Modern(ctx))
		defer result.Drop(ctx)
		if result.Variant() == "Err" {
			return appendError(ctx, dst, result.Field("0"), path)
		}
		scene := result.Field("0")
		rebuilt := TypeScene.Uninit(ctx)
		rebuilt.Field("width").SetFloat(scene.Field("width").Float())
		rebuilt.Field("height").SetFloat(scene.Field("height").Float())
		commands := scene.Field("commands")
		rebuilt.Field("commands").Init(commands.Type.Vec(ctx, commands.Len()))
		defer rebuilt.Drop(ctx)
		output := rebuilt.Field("commands")
		for i := uintptr(0); i < commands.Len(); i++ {
			element := commands.Index(i)
			switch element.Variant() {
			case "FillPath", "PushClip", "PopClip", "PushLayer", "PopLayer":
			default:
				panic("unknown scene command")
			}
			output.InitAt(i, SceneCommand_As_Core_Clone_Clone_Clone(ctx, element))
			output.SetLen(i + 1)
		}
		return appendDebug(ctx, dst, rebuilt)
	case "write_svg", "write_svg_error", "write_png", "write_png_invalid", "write_png_error":
		var result oxide.Value
		if strings.HasPrefix(name, "write_svg") {
			result = WriteOutputSvg(ctx, source, optionalSpan(ctx, WriteOutputSvgTypes.Params[1], path, true))
		} else {
			config := TypeRenderConfig.Default(ctx)
			defer config.Drop(ctx)
			theme := Theme_Modern(ctx)
			defer theme.Drop(ctx)
			result = WriteOutputPng(ctx, source, path, config, theme)
		}
		defer result.Drop(ctx)
		if result.Variant() == "Err" {
			return appendError(ctx, dst, result.Field("0"), path)
		}
		return dst
	case "config_none", "config_file", "config_json_error", "config_missing", "config_theme", "config_theme_error":
		var result oxide.Value
		if name == "config_theme" || name == "config_theme_error" {
			theme := "neutral"
			some := true
			if name == "config_theme_error" {
				theme = "unrecognized-theme"
				some = false
			}
			result = Config_LoadConfigWithTheme(ctx, optionalSpan(ctx, Config_LoadConfigWithThemeTypes.Params[0], path, some), optionalSpan(ctx, Config_LoadConfigWithThemeTypes.Params[1], ctx.CopyString(theme), true))
		} else {
			result = Config_LoadConfig(ctx, optionalSpan(ctx, Config_LoadConfigTypes.Params[0], path, name != "config_none"))
		}
		return appendResult(ctx, dst, result, path, false)
	case "merge_config", "merge_nonobject":
		json := MergeInitConfigTypes.Params[1].FromJSON(ctx, source)
		if json.Variant() != "Ok" {
			panic("native merge input failed to deserialize")
		}
		// Move the payload out; the Result owner must not drop it again.
		value := MergeInitConfig(ctx, TypeConfig.Default(ctx), json.Field("0"))
		defer value.Drop(ctx)
		return appendDebug(ctx, dst, value)
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
			v := Config_ParseAspectRatioValue(ctx, ctx.CopyString(token))
			dst = appendDebug(ctx, dst, v)
			v.Drop(ctx)
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
			v := Theme_FromName(ctx, ctx.CopyString(token))
			dst = appendJSONValue(ctx, dst, v)
			v.Drop(ctx)
			if !more {
				break
			}
			rest = next
		}
		dst = append(dst, "],\"presets\":["...)
		for i, f := range [...]func(*oxide.Context) oxide.Value{Theme_Modern, Theme_MermaidDefault, Theme_Dark, Theme_Forest, Theme_Neutral} {
			if i > 0 {
				dst = append(dst, ',')
			}
			v := f(ctx)
			dst = appendJSONValue(ctx, dst, v)
			v.Drop(ctx)
		}
		return append(dst, ']', '}')
	case "serde_traits":
		theme := Theme_Forest(ctx)
		defer theme.Drop(ctx)
		encoded := theme.JSON(ctx)
		defer encoded.Drop(ctx)
		decoded := theme.Type.FromJSON(ctx, resultOK(ctx, encoded).Span())
		defer decoded.Drop(ctx)
		roundtrip := resultOK(ctx, decoded).JSON(ctx)
		defer roundtrip.Drop(ctx)
		if !bytes.Equal(resultOK(ctx, encoded).Bytes(), resultOK(ctx, roundtrip).Bytes()) {
			panic("Theme JSON changed")
		}
		dst = append(dst, resultOK(ctx, encoded).Bytes()...)
		config := TypeLayoutConfig.Default(ctx)
		defer config.Drop(ctx)
		configJSON := config.JSON(ctx)
		defer configJSON.Drop(ctx)
		configDecoded := config.Type.FromJSON(ctx, resultOK(ctx, configJSON).Span())
		defer configDecoded.Drop(ctx)
		configAgain := resultOK(ctx, configDecoded).JSON(ctx)
		defer configAgain.Drop(ctx)
		if !bytes.Equal(resultOK(ctx, configJSON).Bytes(), resultOK(ctx, configAgain).Bytes()) {
			panic("LayoutConfig JSON changed")
		}
		dst = append(dst, resultOK(ctx, configJSON).Bytes()...)
		clone := LayoutConfig_As_Core_Clone_Clone_Clone(ctx, config)
		dst = appendDebug(ctx, dst, clone)
		clone.Drop(ctx)
		graph := TypeGraph.Default(ctx)
		defer graph.Drop(ctx)
		return appendGraph(ctx, dst, graph)
	case "render_svg", "render_svg_dimensions", "layout", "layout_valid", "layout_invalid", "quality", "quality_other":
		parsed, theme, config, layout := layoutValues(ctx, source)
		defer parsed.Drop(ctx)
		defer theme.Drop(ctx)
		defer config.Drop(ctx)
		defer layout.Drop(ctx)
		switch name {
		case "render_svg", "render_svg_dimensions":
			var value oxide.Value
			if name == "render_svg" {
				value = RenderSvg(ctx, layout, theme, config)
			} else {
				value = Render_RenderSvgWithDimensions(ctx, layout, theme, config, dimensions(ctx, Render_RenderSvgWithDimensionsTypes.Params[3], 640, 480))
			}
			defer value.Drop(ctx)
			return append(dst, value.Bytes()...)
		case "layout":
			dump := LayoutDump_LayoutDump_FromLayout(ctx, layout, parsed.Field("0").Field("graph"))
			defer dump.Drop(ctx)
			return appendJSON(ctx, dst, dump)
		case "layout_valid", "layout_invalid":
			if name == "layout_invalid" {
				layout.Field("width").SetFloat(math.NaN())
				layout.Field("height").SetFloat(-1)
				it := layout.Field("nodes").IteratorMut(ctx)
				first := it.Next(ctx)
				first.Field("0").Field("1").Deref().Field("width").SetFloat(0)
				first.Drop(ctx)
				it.Drop(ctx)
				points := layout.Field("edges").Index(0).Field("points")
				if points.Type.Container.Element.NeedsDrop {
					panic("layout points gained owned elements")
				}
				points.SetLen(0)
			}
			result := Layout_ValidateLayoutInvariants(ctx, layout)
			defer result.Drop(ctx)
			if result.Variant() == "Ok" {
				return append(dst, "valid"...)
			}
			items := result.Field("0")
			dst = append(dst, '[')
			for i := uintptr(0); i < items.Len(); i++ {
				if i != 0 {
					dst = append(dst, ',', ' ')
				}
				v := items.Index(i)
				message := v.Display(ctx)
				dst = append(dst, '(')
				dst = appendDebug(ctx, dst, v.Field("path"))
				dst = append(dst, ',', ' ')
				dst = appendDebug(ctx, dst, v.Field("message"))
				dst = append(dst, ',', ' ')
				dst = appendDebug(ctx, dst, message)
				dst = append(dst, ')')
				message.Drop(ctx)
			}
			return append(dst, ']')
		case "quality", "quality_other":
			result := Layout_FlowchartQualityMetrics(ctx, layout)
			defer result.Drop(ctx)
			if result.Variant() == "None" {
				return append(dst, "None"...)
			}
			value := result.Field("0")
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
		measured := MeasureWithDimensions(ctx, source, TypeRenderOptions.Default(ctx), dimensions(ctx, MeasureWithDimensionsTypes.Params[2], 321, 123))
		defer measured.Drop(ctx)
		direct := MeasureSvgDimensions(ctx, layout, config, dimensions(ctx, MeasureSvgDimensionsTypes.Params[2], 321, 123))
		defer direct.Drop(ctx)
		a, b := resultOK(ctx, measured), direct
		for _, field := range a.Type.Fields {
			if a.Field(field.Name).Float() != b.Field(field.Name).Float() {
				panic("dimension APIs disagree")
			}
		}
		return appendJSON(ctx, dst, a)
	case "dimension_methods":
		dst = append(dst, '[')
		for i, height := range [...]float64{0, -1, 0.5, 12} {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			v := TypeSvgDimensions.Uninit(ctx)
			v.Field("width").SetFloat(24)
			v.Field("height").SetFloat(height)
			v.Field("viewbox_x").SetFloat(0)
			v.Field("viewbox_y").SetFloat(0)
			v.Field("viewbox_width").SetFloat(36)
			v.Field("viewbox_height").SetFloat(height)
			dst = append(dst, '(')
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(SvgDimensions_AspectRatio(ctx, v))), 10)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(SvgDimensions_ViewboxAspectRatio(ctx, v))), 10)
			dst = append(dst, ')')
		}
		return append(dst, ']')
	case "option_methods":
		dst = append(dst, '[')
		first := true
		for _, ratio := range [...]float32{2, 0, -1, float32(math.NaN()), float32(math.Inf(1))} {
			values := [3]oxide.Value{RenderOptions_WithPreferredAspectRatio(ctx, RenderOptions_WithRankSpacing(ctx, RenderOptions_WithNodeSpacing(ctx, RenderOptions_Modern(ctx), 33), 55), ratio), RenderOptions_WithPreferredAspectRatioParts(ctx, RenderOptions_MermaidDefault(ctx), ratio, 3), RenderOptions_WithPreferredAspectRatioParts(ctx, TypeRenderOptions.Default(ctx), 3, ratio)}
			for _, v := range values {
				if !first {
					dst = append(dst, ',', ' ')
				}
				first = false
				dst = appendDebug(ctx, dst, v)
				v.Drop(ctx)
			}
		}
		return append(dst, ']')
	default:
		return directMore(ctx, name, source, path, dst)
	}
	panic("unhandled direct API state: " + name)
}
