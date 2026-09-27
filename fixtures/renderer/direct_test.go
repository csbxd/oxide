package rendererfixture_test

import (
	"strings"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	. "oxide-renderer-conformance"
)

// These Go test helpers compose only public upstream roots and generic Rust
// value operations. They contain no parsing, layout, rendering or PNG logic.
type resultTag interface{ String() string }
type displayValue interface {
	Display(*oxide.Context) Value__Alloc_String_String
}
type resultView[O any, E displayValue, T resultTag] interface {
	Variant() T
	Field__Ok__0() O
	Field__Err__0() E
}

// Generic constraints retain the concrete payload type at every call. These
// test observers cannot accept a different Result or synthesize Rust behavior.
func resultOK[O any, E displayValue, T resultTag, R resultView[O, E, T], V interface{ Ref() R }](ctx *oxide.Context, result V) O {
	view := result.Ref()
	if view.Variant().String() == "Ok" {
		return view.Field__Ok__0()
	}
	message := view.Field__Err__0().Display(ctx)
	text := strings.Clone(message.Ref().String())
	message.Drop(ctx)
	panic(text)
}

func renderSVG(ctx *oxide.Context, source oxide.Span, output []byte) uintptr {
	mark := ctx.Mark()
	defer ctx.Restore(mark)
	result := Render(ctx, Borrow__Str(source))
	defer result.Drop(ctx)
	svg := resultOK(ctx, result)
	copy(output, svg.Bytes())
	return svg.Len()
}

func writePNG(ctx *oxide.Context, source, path oxide.Span) {
	mark := ctx.Mark()
	defer ctx.Restore(mark)
	result := Render(ctx, Borrow__Str(source))
	defer result.Drop(ctx)
	svg := resultOK(ctx, result)
	config := Default__RenderConfig(ctx)
	defer config.Drop(ctx)
	theme := Theme_Modern(ctx)
	defer theme.Drop(ctx)
	status := WriteOutputPng(ctx, svg.Borrow(), Borrow__Std_Path_Path(path), config.Ref(), theme.Ref())
	defer status.Drop(ctx)
	resultOK(ctx, status)
}
