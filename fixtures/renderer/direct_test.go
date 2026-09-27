package rendererfixture_test

import (
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	. "oxide-renderer-conformance"
)

// These Go test helpers compose only public upstream roots and generic Rust
// value operations. They contain no parsing, layout, rendering or PNG logic.
func resultOK(ctx *oxide.Context, result oxide.Value) oxide.Value {
	if result.Variant() == "Ok" {
		return result.Field("0")
	}
	message := result.Field("0").Display(ctx)
	text := message.StringCopy()
	message.Drop(ctx)
	panic(text)
}

func renderSVG(ctx *oxide.Context, source oxide.Span, output []byte) uintptr {
	mark := ctx.Mark()
	defer ctx.Restore(mark)
	result := Render(ctx, source)
	defer result.Drop(ctx)
	svg := resultOK(ctx, result)
	copy(output, svg.Bytes())
	return svg.Len()
}

func writePNG(ctx *oxide.Context, source, path oxide.Span) {
	mark := ctx.Mark()
	defer ctx.Restore(mark)
	result := Render(ctx, source)
	defer result.Drop(ctx)
	svg := resultOK(ctx, result)
	config := TypeRenderConfig.Default(ctx)
	defer config.Drop(ctx)
	theme := Theme_Modern(ctx)
	defer theme.Drop(ctx)
	status := WriteOutputPng(ctx, svg.Span(), path, config, theme)
	defer status.Drop(ctx)
	resultOK(ctx, status)
}
