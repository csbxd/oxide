package rendererfixture_test

import (
	"bytes"
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"math"
	. "oxide-renderer-conformance"
	"strconv"
	"strings"
)

func directConstructors(ctx *oxide.Context, name string, source, path oxide.Span, dst []byte) []byte {
	if name == "ir_methods" {
		return directIR(ctx, dst)
	}
	if name == "construct_other" {
		a := RadarEntry_Positional(ctx, New__Core_Option_Option__Of__F32__End__None(ctx))
		b := RadarEntry_Positional(ctx, someFloat(ctx, 1.25))
		c := RadarEntry_Named(ctx, String__Alloc_String_String(ctx, "axis"), someFloat(ctx, 3.5))
		color := New__Color(ctx)
		color.Mut().Field__R().Set(1)
		color.Mut().Field__G().Set(2)
		color.Mut().Field__B().Set(3)
		color.Mut().Field__A().Set(0.5)
		d := Paint_Solid(ctx, color)
		dst = append(dst, '(')
		for i, value := range [...]Value__RadarEntry{a, b, c} {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = appendDebug(ctx, dst, value.Ref())
			value.Drop(ctx)
		}
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, d.Ref())
		d.Drop(ctx)
		return append(dst, ')')
	}
	if !strings.HasPrefix(name, "construct_") {
		panic("direct API probe is not implemented: " + name)
	}
	var value Value__Layout_DiagramData
	if name == "construct_error" {
		item := New__Layout_ErrorLayout(ctx)
		item.Mut().Field__ViewboxWidth().Set(1)
		item.Mut().Field__ViewboxHeight().Set(2)
		item.Mut().Field__RenderWidth().Set(3)
		item.Mut().Field__RenderHeight().Set(4)
		item.Mut().Field__TextX().Set(5)
		item.Mut().Field__TextY().Set(6)
		item.Mut().Field__TextSize().Set(7)
		item.Mut().Field__VersionX().Set(8)
		item.Mut().Field__VersionY().Set(9)
		item.Mut().Field__VersionSize().Set(10)
		item.Mut().Field__IconScale().Set(11)
		item.Mut().Field__IconTx().Set(12)
		item.Mut().Field__IconTy().Set(13)
		item.Mut().Field__Message().Init(String__Alloc_String_String(ctx, "error probe"))
		item.Mut().Field__Version().Init(String__Alloc_String_String(ctx, "version probe"))
		value = Layout_DiagramData_Error(ctx, item)
	} else {
		parsed, theme, config, layout := layoutValues(ctx, source)
		value = Layout_DiagramData_As_Core_Clone_Clone_Clone(ctx, layout.Ref().Field__Diagram())
		layout.Drop(ctx)
		config.Drop(ctx)
		theme.Drop(ctx)
		parsed.Drop(ctx)
	}
	original := value.Ref().Debug(ctx)
	defer original.Drop(ctx)
	// Every branch moves its own statically typed payload. The previous enum
	// owner is uninitialized after Move and is never dropped or reused.
	switch value.Ref().Variant() {
	case Variant__Layout_DiagramData__Sequence:
		value = Layout_DiagramData_Sequence(ctx, value.Mut().Field__Sequence__0().Move(ctx))
	case Variant__Layout_DiagramData__Pie:
		value = Layout_DiagramData_Pie(ctx, value.Mut().Field__Pie__0().Move(ctx))
	case Variant__Layout_DiagramData__Quadrant:
		value = Layout_DiagramData_Quadrant(ctx, value.Mut().Field__Quadrant__0().Move(ctx))
	case Variant__Layout_DiagramData__Gantt:
		value = Layout_DiagramData_Gantt(ctx, value.Mut().Field__Gantt__0().Move(ctx))
	case Variant__Layout_DiagramData__Sankey:
		value = Layout_DiagramData_Sankey(ctx, value.Mut().Field__Sankey__0().Move(ctx))
	case Variant__Layout_DiagramData__GitGraph:
		value = Layout_DiagramData_GitGraph(ctx, value.Mut().Field__GitGraph__0().Move(ctx))
	case Variant__Layout_DiagramData__C4:
		value = Layout_DiagramData_C4(ctx, value.Mut().Field__C4__0().Move(ctx))
	case Variant__Layout_DiagramData__XYChart:
		value = Layout_DiagramData_XYChart(ctx, value.Mut().Field__XYChart__0().Move(ctx))
	case Variant__Layout_DiagramData__Timeline:
		value = Layout_DiagramData_Timeline(ctx, value.Mut().Field__Timeline__0().Move(ctx))
	case Variant__Layout_DiagramData__Journey:
		value = Layout_DiagramData_Journey(ctx, value.Mut().Field__Journey__0().Move(ctx))
	case Variant__Layout_DiagramData__Radar:
		value = Layout_DiagramData_Radar(ctx, value.Mut().Field__Radar__0().Move(ctx))
	case Variant__Layout_DiagramData__Error:
		value = Layout_DiagramData_Error(ctx, value.Mut().Field__Error__0().Move(ctx))
	default:
		panic("unexpected constructor variant: " + value.Ref().Variant().String())
	}
	defer value.Drop(ctx)
	clone := Layout_DiagramData_As_Core_Clone_Clone_Clone(ctx, value.Ref())
	defer clone.Drop(ctx)
	text := clone.Ref().Debug(ctx)
	defer text.Drop(ctx)
	if !bytes.Equal(text.Ref().Bytes(), original.Ref().Bytes()) {
		panic("constructor/clone changed diagram payload")
	}
	return appendDebug(ctx, dst, value.Ref())
}

func directIR(ctx *oxide.Context, dst []byte) []byte {
	dst = append(dst, '[')
	for i, s := range [...]string{"TD", "TB", "BT", "LR", "RL", "lr", "?"} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		a := Direction_FromToken(ctx, Borrow__Str(ctx.CopyString(s)))
		b := Direction_FromTimelineToken(ctx, Borrow__Str(ctx.CopyString(s)))
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, a.Ref())
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, b.Ref())
		dst = append(dst, ')')
		a.Drop(ctx)
		b.Drop(ctx)
	}
	dst = append(dst, ']', '[')
	for i, c := range [...]rune{'L', 'R', 'T', 'B', 'l', 'r', 't', 'b', '?', '界'} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		value := Ir_ArchDir_FromChar(ctx, uint32(c))
		if value.Ref().Variant().String() == "None" {
			dst = append(dst, "None"...)
		} else {
			entry := value.Ref().Field__Some__0()
			dst = append(dst, "Some(("...)
			dst = appendDebug(ctx, dst, entry)
			dst = append(dst, ',', ' ')
			dst = strconv.AppendBool(dst, Ir_ArchDir_IsHorizontal(ctx, Ir_ArchDir_As_Core_Clone_Clone_Clone(ctx, entry)))
			dst = append(dst, "))"...)
		}
		value.Drop(ctx)
	}
	dst = append(dst, ']', '[')
	for i, construct := range [...]func(*oxide.Context) Value__Ir_C4ShapeKind{New__Ir_C4ShapeKind__Person, New__Ir_C4ShapeKind__ExternalPerson, New__Ir_C4ShapeKind__System, New__Ir_C4ShapeKind__SystemDb, New__Ir_C4ShapeKind__SystemQueue, New__Ir_C4ShapeKind__ExternalSystem, New__Ir_C4ShapeKind__ExternalSystemDb, New__Ir_C4ShapeKind__ExternalSystemQueue, New__Ir_C4ShapeKind__Container, New__Ir_C4ShapeKind__ContainerDb, New__Ir_C4ShapeKind__ContainerQueue, New__Ir_C4ShapeKind__ExternalContainer, New__Ir_C4ShapeKind__ExternalContainerDb, New__Ir_C4ShapeKind__ExternalContainerQueue, New__Ir_C4ShapeKind__Component, New__Ir_C4ShapeKind__ComponentDb, New__Ir_C4ShapeKind__ComponentQueue, New__Ir_C4ShapeKind__ExternalComponent, New__Ir_C4ShapeKind__ExternalComponentDb, New__Ir_C4ShapeKind__ExternalComponentQueue} {
		if i > 0 {
			dst = append(dst, ',', ' ')
		}
		shape := construct(ctx)
		text := Ir_C4ShapeKind_AsStr(ctx, shape.Ref())
		dst = appendDebug(ctx, dst, text)
		shape.Drop(ctx)
	}
	dst = append(dst, ']')
	for _, construct := range [...]func(*oxide.Context) Value__DiagramKind{New__DiagramKind__Flowchart, New__DiagramKind__Class, New__DiagramKind__State, New__DiagramKind__Sequence, New__DiagramKind__Er, New__DiagramKind__Pie, New__DiagramKind__Mindmap, New__DiagramKind__Journey, New__DiagramKind__Timeline, New__DiagramKind__Gantt, New__DiagramKind__Requirement, New__DiagramKind__GitGraph, New__DiagramKind__C4, New__DiagramKind__Sankey, New__DiagramKind__Quadrant, New__DiagramKind__ZenUML, New__DiagramKind__Block, New__DiagramKind__Packet, New__DiagramKind__Kanban, New__DiagramKind__Architecture, New__DiagramKind__Radar, New__DiagramKind__Treemap, New__DiagramKind__XYChart} {
		kind := construct(ctx)
		for head := 0; head < 3; head++ {
			var option Value__Core_Option_Option__Of__EdgeArrowhead__End
			switch head {
			case 0:
				option = New__Core_Option_Option__Of__EdgeArrowhead__End__None(ctx)
			case 1:
				option = New__Core_Option_Option__Of__EdgeArrowhead__End__Some(ctx, New__EdgeArrowhead__OpenTriangle(ctx))
			case 2:
				option = New__Core_Option_Option__Of__EdgeArrowhead__End__Some(ctx, New__EdgeArrowhead__ClassDependency(ctx))
			}
			number := Render_ArrowheadInset(ctx, DiagramKind_As_Core_Clone_Clone_Clone(ctx, kind.Ref()), option)
			dst = strconv.AppendUint(dst, uint64(math.Float32bits(number)), 10)
		}
		kind.Drop(ctx)
	}
	for _, construct := range [...]func(*oxide.Context) Value__Cli_LayoutEngineArg{New__Cli_LayoutEngineArg__Current, New__Cli_LayoutEngineArg__Dagre, New__Cli_LayoutEngineArg__Auto} {
		engine := FlowchartLayoutEngine_As_Core_Convert_From_Of_Cli_LayoutEngineArg_End_From(ctx, construct(ctx))
		text := FlowchartLayoutEngine_AsStr(ctx, FlowchartLayoutEngine_As_Core_Clone_Clone_Clone(ctx, engine.Ref()))
		dst = append(dst, '(')
		dst = appendDebug(ctx, dst, engine.Ref())
		dst = append(dst, ',', ' ')
		dst = appendDebug(ctx, dst, text)
		dst = append(dst, ')')
		engine.Drop(ctx)
	}
	graph := Graph_New(ctx)
	defer graph.Drop(ctx)
	Graph_EnsureNode(ctx, graph.Mut(), Borrow__Str(ctx.CopyString("B")), New__Core_Option_Option__Of__Alloc_String_String__End__None(ctx), New__Core_Option_Option__Of__NodeShape__End__None(ctx))
	Graph_EnsureNode(ctx, graph.Mut(), Borrow__Str(ctx.CopyString("A")), New__Core_Option_Option__Of__Alloc_String_String__End__Some(ctx, String__Alloc_String_String(ctx, "Alpha")), New__Core_Option_Option__Of__NodeShape__End__Some(ctx, New__NodeShape__Circle(ctx)))
	Graph_EnsureNode(ctx, graph.Mut(), Borrow__Str(ctx.CopyString("B")), New__Core_Option_Option__Of__Alloc_String_String__End__Some(ctx, String__Alloc_String_String(ctx, "Beta")), New__Core_Option_Option__Of__NodeShape__End__Some(ctx, New__NodeShape__Diamond(ctx)))
	Graph_EnsureNode(ctx, graph.Mut(), Borrow__Str(ctx.CopyString("B")), New__Core_Option_Option__Of__Alloc_String_String__End__None(ctx), New__Core_Option_Option__Of__NodeShape__End__None(ctx))
	dst = appendGraph(ctx, dst, graph.Ref())
	quality := Default__Layout_FlowchartQualityMetrics(ctx)
	defer quality.Drop(ctx)
	quality.Mut().Field__BadSourceExits().Set(1)
	quality.Mut().Field__BadTargetEntries().Set(2)
	quality.Mut().Field__EndpointNodeIntrusions().Set(3)
	quality.Mut().Field__NonEndpointNodeHits().Set(5)
	quality.Mut().Field__EndpointNodeReentries().Set(7)
	dst = append(dst, '(')
	dst = strconv.AppendUint(dst, uint64(Layout_FlowchartQualityMetrics_HardViolationCount(ctx, quality.Ref())), 10)
	dst = append(dst, ',', ' ')
	dst = strconv.AppendUint(dst, uint64(Layout_FlowchartQualityMetrics_GeometryDebtCount(ctx, quality.Ref())), 10)
	dst = append(dst, ')')
	err := New__Layout_LayoutInvariantError(ctx)
	err.Mut().Field__Path().Init(String__Alloc_String_String(ctx, "probe.path"))
	err.Mut().Field__Message().Init(String__Alloc_String_String(ctx, "probe message"))
	defer err.Drop(ctx)
	message := err.Ref().Display(ctx)
	defer message.Drop(ctx)
	return appendDebug(ctx, dst, message.Ref())
}
