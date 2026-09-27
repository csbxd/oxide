package mir

import (
	"fmt"
	"strings"
)

// Public values retain Rust identity independently of their internal Go ABI.
func (g *generator) publicKind(id int) string {
	t := g.apiType(id)
	if t.Kind == "pointer" {
		if t.PointerKind != "ref" {
			if t.Size == 8 {
				return "scalar"
			}
			return "value"
		}
		if c := t.Container; c != nil {
			switch c.Kind {
			case "str_ref", "path_ref", "os_str_ref":
				return "span"
			case "slice_ref":
				return "slice"
			}
		}
		return "ref"
	}
	if t.Kind == "aggregate" || t.Kind == "array" {
		return "value"
	}
	if g.scalar(t) != "" {
		return "scalar"
	}
	return "value"
}

func (g *generator) publicGoType(id int, result bool) string {
	t := g.apiType(id)
	if result && t.Name == "()" {
		return ""
	}
	switch g.publicKind(id) {
	case "scalar":
		return g.goType(id)
	case "span":
		if !result {
			return "oxide.Span"
		}
	}
	return "oxide.Value"
}

func (g *generator) checkPublicValue(name string, id int) {
	g.line("if %s.Type != %s || %s.Addr == 0 { panic(%q) }", name, g.apiTypeExpr(id), name, "oxide: Rust argument type mismatch")
}

// referenceHeader constructs a reference in stable Rust storage. No Go stack
// address is converted to uintptr across a translated call.
func (g *generator) referenceHeader(name string, id int, data, meta string) string {
	t := g.typ(id)
	if t.Size == 8 {
		return data
	}
	if t.Size != 16 || t.ABIPair == nil {
		g.fail("unsupported reference layout %s", t.Name)
	}
	g.line("%s := ctx.Alloc(%d,%d)", name, t.Size, t.Align)
	g.line("*(*uintptr)(unsafe.Pointer(%s)) = %s", name, data)
	g.line("*(*uintptr)(unsafe.Pointer(%s+%d)) = %s", name, t.ABIPair.BOffset, meta)
	return g.storageLocation("unsafe.Pointer("+name+")", id).value()
}

func (g *generator) publicArgument(name string, id int) string {
	t := g.apiType(id)
	switch g.publicKind(id) {
	case "scalar":
		return name
	case "span":
		return g.referenceHeader(name+"Ref", id, name+".Data", name+".Len")
	case "slice":
		g.line("%sParts := %s.Elements(%s)", name, name, g.apiTypeExpr(t.Container.Element))
		return g.referenceHeader(name+"Ref", id, name+"Parts.Data", name+"Parts.Len")
	case "ref":
		g.checkPublicValue(name, t.Pointee)
		return g.referenceHeader(name+"Ref", id, name+".Addr", name+".Meta")
	default:
		g.checkPublicValue(name, id)
		return g.storageLocation("unsafe.Pointer("+name+".Addr)", id).value()
	}
}

// beginPublicFrame keeps an owned return slot, while unwinding all temporary
// headers. On panic it restores the complete boundary frame.
func (g *generator) beginPublicFrame(ret int) {
	g.line("entry := ctx.Mark(); restore := entry; defer func(){ctx.Restore(restore)}()")
	if g.publicKind(ret) == "value" && g.publicGoType(ret, true) != "" {
		t := g.apiType(ret)
		g.line("out := oxide.Value{Addr:ctx.Alloc(%d,%d),Type:%s}", t.Size, t.Align, g.apiTypeExpr(ret))
		g.line("retained := ctx.Mark()")
	}
}

func (g *generator) publicCall(symbol string, ret int, args []string) {
	name := g.names[symbol]
	if name == "" {
		g.fail("missing public callee %s", symbol)
	}
	kind := g.publicKind(ret)
	if g.publicGoType(ret, true) == "" {
		g.line("%s(%s)", name, strings.Join(args, ","))
	} else if kind == "value" {
		g.callResult(g.storageLocation("unsafe.Pointer(out.Addr)", ret), name, args)
	} else {
		g.line("result := %s(%s)", name, strings.Join(args, ","))
	}
	g.line("if ctx.Failed(){panic(ctx.TakePanic())}")
	if g.publicGoType(ret, true) == "" {
		g.line("return")
		return
	}
	switch kind {
	case "value":
		g.line("restore = retained; return out")
	case "scalar":
		g.line("return result")
	default:
		t := g.apiType(ret)
		data, meta := "result", "uintptr(0)"
		if t.Size != 8 {
			// Read a local value without retaining its Go address.
			g.line("header := ctx.Alloc(%d,%d)", t.Size, t.Align)
			g.storeValue(g.storageLocation("unsafe.Pointer(header)", ret), "result")
			data = "*(*uintptr)(unsafe.Pointer(header))"
			meta = fmt.Sprintf("*(*uintptr)(unsafe.Pointer(header+%d))", t.ABIPair.BOffset)
		}
		g.line("return oxide.Value{Addr:%s,Meta:%s,Type:%s}", data, meta, g.apiTypeExpr(t.Pointee))
	}
}

func (g *generator) publicSignature(r Root, f *Function) ([]int, int) {
	if r.Params != nil {
		return r.Params, r.Return
	}
	return g.signature(f)
}

func (g *generator) emitProcessExit() {
	if g.p.ProcessExitSymbol == "" {
		return
	}
	f := g.functions[g.p.ProcessExitSymbol]
	if f == nil {
		g.fail("missing std::process::exit")
	}
	params, _ := g.signature(f)
	if len(params) != 1 || g.typ(params[0]).Kind != "i32" || f.TrackCaller {
		g.fail("invalid std::process::exit signature")
	}
	g.line("// RustExit runs Rust's process cleanup and exits, including buffered stdout.")
	g.line("// It does not run Rust stack drops or Go defers.")
	g.line("func RustExit(ctx *oxide.Context,code int32){ %s(ctx,code); if ctx.Failed(){panic(ctx.TakePanic())};panic(\"oxide: std::process::exit returned\") }", g.names[f.Symbol])
}
