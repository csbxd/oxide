package mir

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
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

func (g *generator) apiTypeExpr(id int) string {
	return fmt.Sprintf("&oxideType%d", g.apiIdentity(id))
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

func (g *generator) apiFields(fields []TypeMember) {
	g.line("[]oxide.Field{")
	for _, f := range fields {
		typ := "nil"
		if f.Public || !g.apiType(f.Type).Sized {
			typ = g.apiTypeExpr(f.Type)
		}
		g.line("{Name:%q,Offset:%d,Type:%s,Public:%t},", f.Name, f.Offset, typ, f.Public)
	}
	g.line("},")
}

func (g *generator) emitPublicTypes() {
	g.f = nil
	ids := g.collectAPITypes()
	drops := make(map[int]string)
	for _, d := range g.p.PublicDropTypes {
		id := g.apiIdentity(d.Type)
		if previous := drops[id]; previous != "" && previous != d.Symbol {
			g.fail("ambiguous Rust destructor identity %d", id)
		}
		drops[id] = d.Symbol
	}
	for _, id := range ids {
		g.line("var oxideType%d oxide.Type", id)
	}
	g.line("func init() {")
	for _, id := range ids {
		t := g.apiType(id)
		g.line("oxideType%d = oxide.Type{ID:%d,Name:%q,Kind:%q,Size:%d,Align:%d,Pack:%d,Sized:%t,NeedsDrop:%t,PointerKind:%q,Mutable:%t,SingleVariant:%d,", id, id, t.Name, t.Kind, t.Size, t.Align, t.Pack, t.Sized, t.NeedsDrop, t.PointerKind, t.Mutable, t.Variant)
		switch t.Kind {
		case "pointer":
			g.line("Elem:%s,", g.apiTypeExpr(t.Pointee))
		case "array", "slice":
			g.line("Elem:%s,Length:%d,", g.apiTypeExpr(t.Element), t.Length)
		}
		if t.Container != nil {
			c := t.Container
			element := "nil"
			if c.Kind != "option" && c.Kind != "result" && c.Kind != "hash_map" && c.Kind != "btree_map" {
				element = g.apiTypeExpr(c.Element)
			}
			key, value := "nil", "nil"
			if c.Kind == "hash_map" || c.Kind == "btree_map" {
				key = g.apiTypeExpr(c.Key)
				value = g.apiTypeExpr(c.Value)
			}
			g.line("Container:&oxide.Container{Kind:%q,Element:%s,Key:%s,Value:%s,DataOffset:%d,LenOffset:%d,CapacityOffset:%d,GlobalAllocator:%t,MetaOffset:%d,Metadata:%q},", c.Kind, element, key, value, c.DataOffset, c.LenOffset, c.CapacityOffset, c.GlobalAllocator, c.MetaOffset, c.Metadata)
		}
		if t.AdtKind == "Enum" {
			g.line("Variants:[]oxide.Variant{")
			for i, name := range t.VariantNames {
				uninhabited := false
				if i < len(t.VariantsInfo) {
					uninhabited = !t.VariantsInfo[i].Inhabited
				}
				g.line("{Name:%q,Uninhabited:%t,Discriminant:%s,Fields:", name, uninhabited, apiBits(t.Discriminants[i]))
				var fields []TypeMember
				if i < len(t.Members) {
					fields = t.Members[i]
				}
				g.apiFields(fields)
				g.line("},")
			}
			g.line("},")
		} else if len(t.Members) > 0 {
			g.line("Fields:")
			g.apiFields(t.Members[0])
		}
		if tag := t.Tag; tag != nil {
			start := "0"
			if tag.Start != "" {
				start = tag.Start
			}
			g.line("Tag:&oxide.EnumTag{Offset:%d,Size:%d,Encoding:%q,Start:%s,First:%d,Last:%d,Untagged:%d},", tag.Offset, tag.Size, tag.Encoding, apiBits(start), tag.First, tag.Last, tag.Untagged)
		}
		symbol := t.DropSymbol
		if symbol == "" {
			symbol = drops[id]
		}
		if symbol != "" {
			if g.names[symbol] == "" {
				g.fail("missing destructor %s", symbol)
			}
			g.line("Drop:func(ctx *oxide.Context,p uintptr){ %s(ctx,p); if ctx.Failed(){panic(ctx.TakePanic())} },", g.names[symbol])
		}
		if t.DefaultSymbol != "" {
			g.emitTypeMethod("Default", id, t.DefaultSymbol, false)
		}
		if t.DisplaySymbol != "" {
			g.emitTypeMethod("Display", id, t.DisplaySymbol, true)
		}
		if t.Debug != nil {
			g.emitDebugMethod(id, t.Debug)
		}
		if t.Iteration != nil {
			g.emitIteration("Iterate", id, t.Iteration)
		}
		if t.MutableIteration != nil {
			g.emitIteration("IterateMut", id, t.MutableIteration)
		}
		if j := t.JSON; j != nil {
			if j.SerializeSymbol != "" {
				g.emitTypeMethod("JSON", id, j.SerializeSymbol, true)
			}
			if j.DeserializeSymbol != "" {
				g.emitFromJSON(j.DeserializeSymbol)
			}
			if j.ValueSymbol != "" {
				g.emitTypeMethod("JSONValue", id, j.ValueSymbol, true)
			}
		}
		g.line("}")
	}
	g.line("}")
	publicNames := make(map[string]int)
	for _, p := range g.p.PublicTypes {
		name, err := ExportName(p.Name)
		if err != nil {
			g.fail("public type: %s", err)
		}
		name = "Type" + name
		id := g.apiIdentity(p.Type)
		if old, ok := publicNames[name]; ok && old != id {
			g.fail("ambiguous Go public type name %s", name)
		}
		publicNames[name] = id
	}
	keys := make([]string, 0, len(publicNames))
	for name := range publicNames {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		g.line("var %s = %s", name, g.apiTypeExpr(publicNames[name]))
	}
	lookup := make(map[string]int)
	add := func(name string, id int) {
		if name == "" {
			return
		}
		id = g.apiIdentity(id)
		if old, ok := lookup[name]; ok && old != id {
			g.fail("ambiguous Rust type name %q", name)
		}
		lookup[name] = id
	}
	for _, id := range ids {
		add(g.apiType(id).Name, id)
	}
	for _, p := range g.p.PublicTypes {
		add(p.Name, p.Type)
		if _, short, ok := strings.Cut(p.Name, "::"); ok {
			add(short, p.Type)
		}
	}
	keys = keys[:0]
	for name := range lookup {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	g.line("// RustType resolves a compiler-recorded Rust name or public type alias.")
	g.line("func RustType(name string) *oxide.Type { switch name {")
	for _, name := range keys {
		g.line("case %s: return %s", strconv.Quote(name), g.apiTypeExpr(lookup[name]))
	}
	g.line("}; panic(\"oxide: unknown Rust type\") }")
}

func (g *generator) emitTypeMethod(field string, id int, symbol string, receiver bool) {
	decl := "ctx *oxide.Context"
	if receiver {
		decl += ",v oxide.Value"
	}
	g.line("%s:func(%s) oxide.Value {", field, decl)
	g.emitTypeMethodBody(field, id, symbol, receiver)
	g.line("return out },")
}

func (g *generator) emitTypeMethodBody(field string, id int, symbol string, receiver bool) {
	f := g.functions[symbol]
	if f == nil {
		g.fail("missing %s method %s", field, symbol)
	}
	params, ret := g.signature(f)
	if receiver {
		g.checkPublicValue("v", id)
	}
	g.line("entry:=ctx.Mark(); restore:=entry; defer func(){ctx.Restore(restore)}()")
	t := g.apiType(ret)
	g.line("out:=oxide.Value{Addr:ctx.Alloc(%d,%d),Type:%s}; retained:=ctx.Mark()", t.Size, t.Align, g.apiTypeExpr(ret))
	args := []string{"ctx"}
	if receiver {
		if len(params) != 1 || g.apiType(params[0]).Kind != "pointer" {
			g.fail("invalid %s receiver", field)
		}
		if g.apiIdentity(g.apiType(params[0]).Pointee) != g.apiIdentity(id) {
			g.fail("mismatched %s receiver", field)
		}
		args = append(args, g.referenceHeader("receiver", params[0], "v.Addr", "v.Meta"))
	} else if len(params) != 0 {
		g.fail("invalid Default arguments")
	}
	if f.TrackCaller {
		g.fail("unsupported track_caller %s method", field)
	}
	g.callResult(g.storageLocation("unsafe.Pointer(out.Addr)", ret), g.names[symbol], args)
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}; restore=retained")
}

func (g *generator) emitIteration(field string, id int, it *IterationAPI) {
	for symbol, expected := range map[string]int{it.Symbol: it.IteratorType, it.NextSymbol: it.ItemType} {
		f := g.functions[symbol]
		if f == nil {
			g.fail("missing Rust iterator function %s", symbol)
		}
		_, ret := g.signature(f)
		if g.apiIdentity(ret) != g.apiIdentity(expected) {
			g.fail("invalid Rust iterator result")
		}
	}
	g.line("%s:func(ctx *oxide.Context,v oxide.Value) oxide.Iterator {", field)
	g.emitTypeMethodBody(field, id, it.Symbol, true)
	g.line("return oxide.Iterator{State:out,NextFunc:func(ctx *oxide.Context,v oxide.Value) oxide.Value {")
	g.emitTypeMethodBody("Iterator::next", it.IteratorType, it.NextSymbol, true)
	g.line("return out }} },")
}

func (g *generator) emitFromJSON(symbol string) {
	f := g.functions[symbol]
	if f == nil {
		g.fail("missing JSON deserialize function %s", symbol)
	}
	params, ret := g.signature(f)
	if len(params) != 1 || g.typ(params[0]).Kind != "pointer" || g.typ(g.typ(params[0]).Pointee).Kind != "str" || g.publicKind(ret) != "value" || f.TrackCaller {
		g.fail("invalid JSON deserialize signature")
	}
	g.line("FromJSON:func(ctx *oxide.Context,input oxide.Span) oxide.Value {")
	g.beginPublicFrame(ret)
	arg := g.referenceHeader("inputRef", params[0], "input.Data", "input.Len")
	g.publicCall(symbol, ret, []string{"ctx", arg})
	g.line("},")
}

func (g *generator) emitDebugMethod(id int, d *DebugAPI) {
	g.line("Debug:func(ctx *oxide.Context,v oxide.Value) oxide.Value {")
	g.checkPublicValue("v", id)
	g.line("entry:=ctx.Mark(); restore:=entry; defer func(){ctx.Restore(restore)}()")
	result := g.apiType(d.StringType)
	g.line("out:=oxide.Value{Addr:ctx.Alloc(%d,%d),Type:%s}; retained:=ctx.Mark()", result.Size, result.Align, g.apiTypeExpr(d.StringType))
	value := "v.Addr"
	if d.ByReference {
		x := g.referenceHeader("valueRef", d.ValueType, "v.Addr", "v.Meta")
		t := g.typ(d.ValueType)
		g.line("valueStorage:=ctx.Alloc(%d,%d)", t.Size, t.Align)
		g.storeValue(g.storageLocation("unsafe.Pointer(valueStorage)", d.ValueType), x)
		value = "valueStorage"
	}
	a := g.typ(d.ArgumentArray)
	if a.Kind != "array" || a.Length != 1 || a.Element != d.ArgumentType {
		g.fail("invalid Debug argument array")
	}
	g.line("arguments:=ctx.Alloc(%d,%d)", a.Size, a.Align)
	g.callResult(g.storageLocation("unsafe.Pointer(arguments)", d.ArgumentType), g.names[d.ArgumentSymbol], []string{"ctx", value})
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}")
	t := g.typ(d.TemplateType)
	if t.Kind != "array" || t.Size != uint64(len(d.Template)) {
		g.fail("invalid Debug format template")
	}
	g.line("template:=ctx.Alloc(%d,%d)", t.Size, t.Align)
	for i, b := range d.Template {
		g.line("*(*byte)(unsafe.Pointer(template+%d))=%d", i, b)
	}
	f := g.functions[d.ArgumentsSymbol]
	if f == nil {
		g.fail("missing fmt::Arguments constructor")
	}
	params, ret := g.signature(f)
	if len(params) != 2 || ret != d.ArgumentsType {
		g.fail("invalid fmt::Arguments signature")
	}
	arg0 := g.referenceHeader("templateRef", params[0], "template", "0")
	arg1 := g.referenceHeader("argumentsRef", params[1], "arguments", "0")
	g.line("formattedArgs:=%s(ctx,%s,%s)", g.names[d.ArgumentsSymbol], arg0, arg1)
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}")
	g.callResult(g.storageLocation("unsafe.Pointer(out.Addr)", d.StringType), g.names[d.FormatSymbol], []string{"ctx", "formattedArgs"})
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}; restore=retained; return out },")
}
