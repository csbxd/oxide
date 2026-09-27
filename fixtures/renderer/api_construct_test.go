package rendererfixture_test

import (
	"bytes"
	"math"
	. "oxide-renderer-conformance"
	"strconv"
	"strings"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

var diagramConstructors = [...]struct {
	name string
	call func(*oxide.Context, oxide.Value) oxide.Value
}{
	{"Sequence", Layout_DiagramData_Sequence}, {"Pie", Layout_DiagramData_Pie}, {"Quadrant", Layout_DiagramData_Quadrant}, {"Gantt", Layout_DiagramData_Gantt}, {"Sankey", Layout_DiagramData_Sankey}, {"GitGraph", Layout_DiagramData_GitGraph}, {"C4", Layout_DiagramData_C4}, {"XYChart", Layout_DiagramData_XYChart}, {"Timeline", Layout_DiagramData_Timeline}, {"Journey", Layout_DiagramData_Journey}, {"Radar", Layout_DiagramData_Radar}, {"Error", Layout_DiagramData_Error},
}

func directConstructors(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	if name == "ir_methods" {
		return directIR(ctx, dst)
	}
	if name == "construct_other" {
		option := RadarEntry_PositionalTypes.Params[0]
		a := RadarEntry_Positional(ctx, option.Enum(ctx, "None"))
		b := RadarEntry_Positional(ctx, someFloat(ctx, option, 1.25))
		label := RadarEntry_NamedTypes.Params[0].String(ctx, "axis")
		c := RadarEntry_Named(ctx, label, someFloat(ctx, RadarEntry_NamedTypes.Params[1], 3.5))
		color := TypeColor.Uninit(ctx)
		color.Field("r").SetUint(1)
		color.Field("g").SetUint(2)
		color.Field("b").SetUint(3)
		color.Field("a").SetFloat(0.5)
		d := Paint_Solid(ctx, color)
		dst = append(dst, '(')
		for i, v := range [...]oxide.Value{a, b, c, d} {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = appendDebug(ctx, dst, v)
			v.Drop(ctx)
		}
		return append(dst, ')')
	}
	if !strings.HasPrefix(name, "construct_") {
		panic("direct API probe is not implemented: " + name)
	}
	var value oxide.Value
	if name == "construct_error" {
		v := Layout_DiagramData_ErrorTypes.Params[0].Uninit(ctx)
		for i, field := range [...]string{"viewbox_width", "viewbox_height", "render_width", "render_height", "text_x", "text_y", "text_size", "version_x", "version_y", "version_size", "icon_scale", "icon_tx", "icon_ty"} {
			v.Field(field).SetFloat(float64(i + 1))
		}
		v.Field("message").Init(v.Field("message").Type.String(ctx, "error probe"))
		v.Field("version").Init(v.Field("version").Type.String(ctx, "version probe"))
		value = Layout_DiagramData_Error(ctx, v)
	} else {
		parsed, theme, config, layout := layoutValues(ctx, source)
		value = Layout_DiagramData_As_Core_Clone_Clone_Clone(ctx, layout.Field("diagram"))
		layout.Drop(ctx)
		config.Drop(ctx)
		theme.Drop(ctx)
		parsed.Drop(ctx)
	}
	original := value.Debug(ctx)
	defer original.Drop(ctx)
	variant := value.Variant()
	var construct func(*oxide.Context, oxide.Value) oxide.Value
	for _, constructor := range diagramConstructors {
		if constructor.name == variant {
			construct = constructor.call
			break
		}
	}
	if construct == nil {
		panic("unexpected constructor variant: " + variant)
	}
	// Move the sole tuple payload; the previous enum bits are no longer owned.
	value = construct(ctx, value.Field("0"))
	defer value.Drop(ctx)
	clone := Layout_DiagramData_As_Core_Clone_Clone_Clone(ctx, value)
	defer clone.Drop(ctx)
	text := clone.Debug(ctx)
	defer text.Drop(ctx)
	if !bytes.Equal(text.Bytes(), original.Bytes()) {
		panic("constructor/clone changed diagram payload")
	}
	return appendDebug(ctx, dst, value)
}

func directIR(ctx *oxide.Context, dst []byte) []byte {
	dst = append(dst, '[')
	for i, s := range [...]string{"TD", "TB", "BT", "LR", "RL", "lr", "?"} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		a := Direction_FromToken(ctx, ctx.CopyString(s))
		b := Direction_FromTimelineToken(ctx, ctx.CopyString(s))
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, a)
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, b)
		dst = append(dst, ')')
		a.Drop(ctx)
		b.Drop(ctx)
	}
	dst = append(dst, ']', '[')
	for i, c := range [...]rune{'L', 'R', 'T', 'B', 'l', 'r', 't', 'b', '?', '界'} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		v := Ir_ArchDir_FromChar(ctx, uint32(c))
		if v.Variant() == "None" {
			dst = append(dst, "None"...)
		} else {
			entry := v.Field("0")
			dst = append(dst, "Some(("...)
			dst = appendDebug(ctx, dst, entry)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendBool(dst, Ir_ArchDir_IsHorizontal(ctx, entry))
			dst = append(dst, "))"...)
		}
		v.Drop(ctx)
	}
	dst = append(dst, ']', '[')
	for i, name := range [...]string{"Person", "ExternalPerson", "System", "SystemDb", "SystemQueue", "ExternalSystem", "ExternalSystemDb", "ExternalSystemQueue", "Container", "ContainerDb", "ContainerQueue", "ExternalContainer", "ExternalContainerDb", "ExternalContainerQueue", "Component", "ComponentDb", "ComponentQueue", "ExternalComponent", "ExternalComponentDb", "ExternalComponentQueue"} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		shape := TypeIr_C4ShapeKind.Enum(ctx, name)
		text := Ir_C4ShapeKind_AsStr(ctx, shape)
		dst = appendDebug(ctx, dst, text)
	}
	dst = append(dst, ']')
	for _, name := range [...]string{"Flowchart", "Class", "State", "Sequence", "Er", "Pie", "Mindmap", "Journey", "Timeline", "Gantt", "Requirement", "GitGraph", "C4", "Sankey", "Quadrant", "ZenUML", "Block", "Packet", "Kanban", "Architecture", "Radar", "Treemap", "XYChart"} {
		kind := TypeDiagramKind.Enum(ctx, name)
		for _, head := range [...]string{"", "OpenTriangle", "ClassDependency"} {
			typ := Render_ArrowheadInsetTypes.Params[1]
			var option oxide.Value
			if head == "" {
				option = typ.Enum(ctx, "None")
			} else {
				option = typ.Enum(ctx, "Some", variantField(typ, "Some", 0).Enum(ctx, head))
			}
			n := Render_ArrowheadInset(ctx, kind, option)
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(n)), 10)
		}
	}
	for _, name := range [...]string{"Current", "Dagre", "Auto"} {
		arg := TypeCli_LayoutEngineArg.Enum(ctx, name)
		engine := FlowchartLayoutEngine_As_Core_Convert_From_Of_Cli_LayoutEngineArg_End_From(ctx, arg)
		text := FlowchartLayoutEngine_AsStr(ctx, engine)
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, engine)
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, text)
		dst = append(dst, ')')
	}
	graph := Graph_New(ctx)
	defer graph.Drop(ctx)
	labelType, shapeType := Graph_EnsureNodeTypes.Params[2], Graph_EnsureNodeTypes.Params[3]
	Graph_EnsureNode(ctx, graph, ctx.CopyString("B"), labelType.Enum(ctx, "None"), shapeType.Enum(ctx, "None"))
	Graph_EnsureNode(ctx, graph, ctx.CopyString("A"), labelType.Enum(ctx, "Some", variantField(labelType, "Some", 0).String(ctx, "Alpha")), shapeType.Enum(ctx, "Some", variantField(shapeType, "Some", 0).Enum(ctx, "Circle")))
	Graph_EnsureNode(ctx, graph, ctx.CopyString("B"), labelType.Enum(ctx, "Some", variantField(labelType, "Some", 0).String(ctx, "Beta")), shapeType.Enum(ctx, "Some", variantField(shapeType, "Some", 0).Enum(ctx, "Diamond")))
	Graph_EnsureNode(ctx, graph, ctx.CopyString("B"), labelType.Enum(ctx, "None"), shapeType.Enum(ctx, "None"))
	dst = appendGraph(ctx, dst, graph)
	quality := TypeLayout_FlowchartQualityMetrics.Default(ctx)
	defer quality.Drop(ctx)
	for i, field := range [...]string{"bad_source_exits", "bad_target_entries", "endpoint_node_intrusions", "non_endpoint_node_hits", "endpoint_node_reentries"} {
		quality.Field(field).SetUint([5]uint64{1, 2, 3, 5, 7}[i])
	}
	dst = append(dst, '(')
	dst = strconv.AppendUint(dst, uint64(Layout_FlowchartQualityMetrics_HardViolationCount(ctx, quality)), 10)
	dst = append(dst, ',', ' ')
	dst = strconv.AppendUint(dst, uint64(Layout_FlowchartQualityMetrics_GeometryDebtCount(ctx, quality)), 10)
	dst = append(dst, ')')
	err := TypeLayout_LayoutInvariantError.Uninit(ctx)
	err.Field("path").Init(err.Field("path").Type.String(ctx, "probe.path"))
	err.Field("message").Init(err.Field("message").Type.String(ctx, "probe message"))
	defer err.Drop(ctx)
	message := err.Display(ctx)
	defer message.Drop(ctx)
	dst = appendDebug(ctx, dst, message)
	return dst
}
