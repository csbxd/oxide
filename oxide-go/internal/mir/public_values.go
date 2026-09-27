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
	case "ref":
		return g.apiRefType(t.Pointee, t.Mutable)
	}
	return "Value__" + g.apiName(id)
}

func (g *generator) apiRefType(id int, mutable bool) string {
	prefix := "Ref__"
	if mutable {
		prefix = "Mut__"
	}
	return prefix + g.apiName(id)
}

func (g *generator) apiMeta(name string, id int) string {
	if g.apiType(id).Sized {
		return "uintptr(0)"
	}
	return name + ".meta"
}

func (g *generator) apiRefLiteral(id int, mutable bool, addr, meta string) string {
	fields := "addr:" + addr
	if !g.apiType(id).Sized {
		fields += ",meta:" + meta
	}
	return g.apiRefType(id, mutable) + "{" + fields + "}"
}

// referenceHeader constructs the compiler's reference value ABI from stable
// addresses. Passing this scalar pair never exposes a Go stack address to Rust.
func (g *generator) referenceHeader(id int, data, meta string) string {
	t := g.typ(id)
	if t.Size == 8 {
		return data
	}
	if t.Size != 16 || t.ABIPair == nil {
		g.fail("unsupported reference layout %s", t.Name)
	}
	a, _ := g.primitive(t.ABIPair.A)
	b, _ := g.primitive(t.ABIPair.B)
	return fmt.Sprintf("%s{A:%s(%s),B:%s(%s)}", g.goType(id), a, data, b, meta)
}

func (g *generator) publicArgument(name string, id int) string {
	t := g.apiType(id)
	switch g.publicKind(id) {
	case "scalar":
		return name
	case "ref":
		if t.Mutable {
			// A mutable place may designate an unaligned packed field; a Rust
			// reference may not. The concrete Ref conversion checks alignment.
			g.line("_ = %s.Ref()", name)
		}
		return g.referenceHeader(id, name+".addr", g.apiMeta(name, t.Pointee))
	default:
		return g.storageLocation("unsafe.Pointer("+name+".addr)", id).value()
	}
}

// beginPublicFrame reserves an owned return slot. A successful call retains
// only that slot; a panic restores the complete boundary frame.
func (g *generator) beginPublicFrame(ret int) {
	if g.publicKind(ret) == "value" && g.publicGoType(ret, true) != "" {
		g.line("entry := ctx.Mark(); restore := entry; defer func(){ctx.Restore(restore)}()")
		t := g.apiType(ret)
		g.line("out := Value__%s{addr:ctx.Alloc(%d,%d)}", g.apiName(ret), t.Size, t.Align)
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
		g.callResult(g.storageLocation("unsafe.Pointer(out.addr)", ret), name, args)
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
			if t.Size != 16 || t.ABIPair == nil {
				g.fail("unsupported public reference return %s", t.Name)
			}
			data, meta = "uintptr(result.A)", "uintptr(result.B)"
		}
		g.line("return %s", g.apiRefLiteral(t.Pointee, t.Mutable, data, meta))
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
