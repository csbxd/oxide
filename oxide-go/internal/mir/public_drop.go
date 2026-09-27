package mir

import (
	"fmt"
	"sort"
)

func (g *generator) emitPublicDrops() {
	g.f = nil
	drops := append([]PublicDropType(nil), g.p.PublicDropTypes...)
	sort.Slice(drops, func(i, j int) bool { return drops[i].Type < drops[j].Type })
	roots := make(map[string]bool, len(g.p.Roots))
	for _, r := range g.p.Roots {
		name, err := ExportName(r.Name)
		if err != nil {
			g.fail("%s", err)
		}
		roots[name] = true
	}
	for i, d := range drops {
		name := fmt.Sprintf("DropT%d", d.Type)
		if i > 0 && drops[i-1].Type == d.Type {
			g.fail("duplicate public drop type %d", d.Type)
		}
		if roots[name] {
			g.fail("public root collides with generated destructor %s", name)
		}
		t := g.types[d.Type]
		if t == nil || !t.Sized || t.Align == 0 || t.Align&(t.Align-1) != 0 {
			g.fail("public drop type %d has no sized Rust layout", d.Type)
		}
		f := g.functions[d.Symbol]
		if f == nil {
			g.fail("public drop type %d has missing function %q", d.Type, d.Symbol)
		}
		params, ret := g.signature(f)
		if len(params) != 1 || f.TrackCaller || (f.Signature != nil && f.Signature.Variadic) {
			g.fail("public drop type %d has invalid destructor signature", d.Type)
		}
		pointer := g.typ(params[0])
		if pointer.Kind != "pointer" || pointer.Pointee != d.Type || g.typ(ret).Size != 0 {
			g.fail("public drop type %d has invalid destructor signature", d.Type)
		}
		g.line("// %s consumes one owned Rust value of type %s.", name, d.Name)
		g.line("// The value and any bitwise copies must not be used or dropped again.")
		if g.indirectValue(d.Type) {
			g.line("// value points to %d initialized bytes with Rust alignment %d.", t.Size, t.Align)
		}
		g.line("func %s(ctx *oxide.Context, value %s) {", name, g.argumentType(d.Type))
		pointerExpr := "value"
		if !g.indirectValue(d.Type) {
			g.line("mark:=ctx.Mark(); defer ctx.Restore(mark)")
			g.line("p:=ctx.Alloc(%d,%d)", max(t.Size, 1), t.Align)
			g.storeValue(g.storageLocation("unsafe.Pointer(p)", d.Type), "value")
			pointerExpr = "p"
		}
		g.line("%s(ctx,%s)", g.names[d.Symbol], pointerExpr)
		g.line("if ctx.Failed() { panic(ctx.TakePanic()) }")
		g.line("}")
	}
}
