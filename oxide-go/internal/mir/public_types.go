package mir

import (
	"fmt"
	"math/big"
	"sort"
)

func (g *generator) apiType(id int) *Type {
	if t := g.apiTypes[id]; t != nil {
		return t
	}
	return g.typ(id)
}

func (g *generator) apiIdentity(id int) int {
	seen := make(map[int]bool)
	for {
		if seen[id] {
			g.fail("cyclic Rust type identity %d", id)
		}
		seen[id] = true
		t := g.apiType(id)
		if t.Canonical == nil || *t.Canonical == id {
			return id
		}
		id = *t.Canonical
	}
}

func (g *generator) collectAPITypes() []int {
	g.apiTypes = make(map[int]*Type)
	for i := range g.p.APITypes {
		t := &g.p.APITypes[i]
		g.apiTypes[t.ID] = t
	}
	seen := make(map[int]bool)
	var visit func(int)
	visit = func(id int) {
		id = g.apiIdentity(id)
		if seen[id] {
			return
		}
		seen[id] = true
		t := g.apiType(id)
		switch t.Kind {
		case "pointer":
			visit(t.Pointee)
		case "array", "slice":
			visit(t.Element)
		case "fnptr":
			for _, param := range t.FnInputs {
				visit(param)
			}
			visit(t.FnOutput)
		}
		if c := t.Container; c != nil {
			switch c.Kind {
			case "option", "result":
			case "hash_map", "btree_map":
				visit(c.Key)
				visit(c.Value)
			default:
				visit(c.Element)
			}
		}
		if t.DisplaySymbol != "" {
			_, ret := g.signature(g.functions[t.DisplaySymbol])
			visit(ret)
		}
		if t.Debug != nil {
			visit(t.Debug.StringType)
		}
		for _, it := range []*IterationAPI{t.Iteration, t.MutableIteration} {
			if it != nil {
				visit(it.IteratorType)
				visit(it.ItemType)
			}
		}
		if j := t.JSON; j != nil {
			for _, symbol := range []string{j.SerializeSymbol, j.DeserializeSymbol, j.ValueSymbol} {
				if symbol != "" {
					f := g.functions[symbol]
					if f == nil {
						g.fail("missing JSON function %s", symbol)
					}
					_, ret := g.signature(f)
					visit(ret)
				}
			}
		}
		for _, fields := range t.Members {
			for _, f := range fields {
				if f.Public || !g.apiType(f.Type).Sized {
					visit(f.Type)
				}
			}
		}
	}
	for _, t := range g.p.APITypes {
		visit(t.ID)
	}
	for _, t := range g.p.PublicTypes {
		visit(t.Type)
	}
	for _, r := range g.p.Roots {
		f := g.functions[r.Symbol]
		if f == nil {
			g.fail("missing root %s", r.Symbol)
		}
		params, ret := g.publicSignature(r, f)
		visit(ret)
		for _, id := range params {
			visit(id)
		}
	}
	for _, d := range g.p.PublicDropTypes {
		visit(d.Type)
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func apiBits(value string) string {
	x, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic(generationError("invalid Rust discriminant " + value))
	}
	x.Mod(x, new(big.Int).Lsh(big.NewInt(1), 128))
	low := x.Uint64()
	high := x.Rsh(x, 64).Uint64()
	return fmt.Sprintf("oxide.U128{Lo:%d,Hi:%d}", low, high)
}

func (g *generator) emitPublicTypes() {
	g.f = nil
	ids := g.collectAPITypes()
	g.initAPINames(ids)
	for _, id := range ids {
		g.emitLayoutType(id)
		g.emitStaticStorage(id)
		g.emitStaticFields(id)
		g.emitStaticContainer(id)
		g.emitStaticOperations(id)
	}
	next := make(map[int]string)
	for _, id := range ids {
		t := g.apiType(id)
		for _, it := range []*IterationAPI{t.Iteration, t.MutableIteration} {
			if it == nil {
				continue
			}
			iid := g.apiIdentity(it.IteratorType)
			if old, ok := next[iid]; ok {
				if old != it.NextSymbol {
					g.fail("ambiguous iterator next for %s", g.apiName(iid))
				}
				continue
			}
			next[iid] = it.NextSymbol
			g.emitStaticMethod("Next", iid, it.NextSymbol, true, true)
		}
	}
	g.emitStaticCallbacks(ids)
	g.emitStaticAliases()
}
