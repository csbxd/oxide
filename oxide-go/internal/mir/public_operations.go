package mir

func (g *generator) emitStaticOperations(id int) {
	t := g.apiType(id)
	if t.DefaultSymbol != "" {
		g.emitStaticMethod("Default", id, t.DefaultSymbol, false, false)
	}
	if t.DisplaySymbol != "" {
		g.emitStaticMethod("Display", id, t.DisplaySymbol, true, false)
	}
	if t.Debug != nil {
		g.emitDebugMethod(id, t.Debug)
	}
	if t.Iteration != nil {
		g.emitStaticMethod("Iter", id, t.Iteration.Symbol, true, false)
	}
	if t.MutableIteration != nil {
		g.emitStaticMethod("Iter", id, t.MutableIteration.Symbol, true, true)
	}
	if j := t.JSON; j != nil {
		if j.SerializeSymbol != "" {
			g.emitStaticMethod("JSON", id, j.SerializeSymbol, true, false)
		}
		if j.ValueSymbol != "" {
			g.emitStaticMethod("JSONValue", id, j.ValueSymbol, true, false)
		}
		if j.DeserializeSymbol != "" {
			g.emitFromJSON(id, j.DeserializeSymbol)
		}
	}
}

// Static adapters call compiler-selected Rust instances directly. No runtime
// type descriptor, method table, or interface dispatch participates.
func (g *generator) emitStaticMethod(method string, id int, symbol string, receiver, mutable bool) {
	f := g.functions[symbol]
	if f == nil {
		g.fail("missing %s method %s", method, symbol)
	}
	params, ret := g.signature(f)
	result := g.publicGoType(ret, true)
	if receiver {
		if len(params) != 1 || g.apiType(params[0]).Kind != "pointer" || g.apiIdentity(g.apiType(params[0]).Pointee) != g.apiIdentity(id) {
			g.fail("invalid %s receiver for %s", method, g.apiName(id))
		}
		g.line("func(v %s) %s(ctx *oxide.Context) %s {", g.apiRefType(id, mutable), method, result)
	} else {
		if len(params) != 0 {
			g.fail("invalid Default arguments")
		}
		g.line("func %s__%s(ctx *oxide.Context) %s {", method, g.apiName(id), result)
	}
	if f.TrackCaller {
		g.fail("unsupported track_caller %s method", method)
	}
	g.beginPublicFrame(ret)
	args := []string{"ctx"}
	if receiver {
		if mutable {
			g.line("_ = v.Ref()")
		}
		args = append(args, g.referenceHeader(params[0], "v.addr", g.apiMeta("v", id)))
	}
	g.publicCall(symbol, ret, args)
	g.line("}")
}

func (g *generator) emitFromJSON(id int, symbol string) {
	f := g.functions[symbol]
	if f == nil {
		g.fail("missing JSON deserialize function %s", symbol)
	}
	params, ret := g.signature(f)
	if len(params) != 1 || g.typ(params[0]).Kind != "pointer" || g.typ(g.typ(params[0]).Pointee).Kind != "str" || g.publicKind(ret) != "value" || f.TrackCaller {
		g.fail("invalid JSON deserialize signature")
	}
	g.line("func FromJSON__%s(ctx *oxide.Context,input %s) %s {", g.apiName(id), g.publicGoType(params[0], false), g.publicGoType(ret, true))
	g.beginPublicFrame(ret)
	arg := g.publicArgument("input", params[0])
	g.publicCall(symbol, ret, []string{"ctx", arg})
	g.line("}")
}

func (g *generator) emitDebugMethod(id int, d *DebugAPI) {
	g.line("func(v Ref__%s) Debug(ctx *oxide.Context) Value__%s {", g.apiName(id), g.apiName(d.StringType))
	g.line("entry:=ctx.Mark(); restore:=entry; defer func(){ctx.Restore(restore)}()")
	result := g.apiType(d.StringType)
	g.line("out:=Value__%s{addr:ctx.Alloc(%d,%d)}; retained:=ctx.Mark()", g.apiName(d.StringType), result.Size, result.Align)
	value := "v.addr"
	if d.ByReference {
		x := g.referenceHeader(d.ValueType, "v.addr", g.apiMeta("v", id))
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
	arg0 := g.referenceHeader(params[0], "template", "0")
	arg1 := g.referenceHeader(params[1], "arguments", "0")
	g.line("formattedArgs:=%s(ctx,%s,%s)", g.names[d.ArgumentsSymbol], arg0, arg1)
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}")
	g.callResult(g.storageLocation("unsafe.Pointer(out.addr)", d.StringType), g.names[d.FormatSymbol], []string{"ctx", "formattedArgs"})
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}; restore=retained; return out }")
}
