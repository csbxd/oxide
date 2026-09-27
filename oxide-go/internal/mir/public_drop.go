package mir

import "sort"

func (g *generator) validatePublicDrops() {
	g.f = nil
	drops := append([]PublicDropType(nil), g.p.PublicDropTypes...)
	sort.Slice(drops, func(i, j int) bool { return drops[i].Type < drops[j].Type })
	for i, d := range drops {
		if i > 0 && drops[i-1].Type == d.Type {
			g.fail("duplicate public drop type %d", d.Type)
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
	}
}
