package mir

import (
	"fmt"
	"sort"
)

func (g *generator) destructorType(id int) *Type {
	t := g.typ(id)
	if t.Kind == "aggregate" && t.Size == 8 && t.Tag != nil && t.Tag.Encoding == "niche" && t.Tag.Offset == 0 && t.Tag.Size == 8 && t.Tag.Start == "0" {
		var fn *Type
		for _, fields := range t.VariantFieldTypes {
			if len(fields) == 1 && g.typ(fields[0]).Kind == "fnptr" {
				fn = g.typ(fields[0])
			}
		}
		if fn != nil {
			t = fn
		}
	}
	if t.Kind != "fnptr" || t.FnABI != "C" || t.FnVariadic || len(t.FnInputs) != 1 || !g.cScalar(t.FnInputs[0], "uintptr") || g.typ(t.FnOutput).Size != 0 {
		g.fail("TLS destructor function ABI %s", t.Name)
	}
	if g.destructorTypes == nil {
		g.destructorTypes = map[int]bool{}
	}
	g.destructorTypes[t.ID] = true
	return t
}

func (g *generator) destructorAdapters() {
	ids := make([]int, 0, len(g.destructorTypes))
	for id := range g.destructorTypes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		t := g.typ(id)
		g.line("func oxideDestructor%d(ctx *oxide.Context, descriptor, argument uintptr) {", id)
		g.line("f:=*(*func(*oxide.Context,%s) %s)(unsafe.Pointer(&descriptor)); _=f(ctx,%s(argument))", g.goType(t.FnInputs[0]), g.goType(t.FnOutput), g.goType(t.FnInputs[0]))
		g.line("if ctx.Failed() { oxide.Abort() } }")
	}
}

func (g *generator) pthreadFunction(f *Function, params []int, ret int) bool {
	if f.Signature.Variadic {
		return false
	}
	if f.Symbol == "pthread_key_create" {
		if len(params) != 2 || !g.cScalar(params[0], "uintptr") || !g.cScalar(ret, "int32") {
			g.fail("pthread_key_create signature")
		}
		dtor := g.destructorType(params[1])
		g.line("return %s(oxide.LibcPthreadKeyCreate(ctx,uintptr(v1),*(*uintptr)(unsafe.Pointer(&v2)),oxideDestructor%d)) }", g.goType(ret), dtor.ID)
		return true
	}
	spec, ok := map[string]cFunction{
		"pthread_key_delete":  {"LibcPthreadKeyDelete", "uint32", "int32"},
		"pthread_setspecific": {"LibcPthreadSetspecific", "uint32 uintptr", "int32"},
		"pthread_getspecific": {"LibcPthreadGetspecific", "uint32", "uintptr"},
	}[f.Symbol]
	if !ok {
		return false
	}
	g.cFunctionBody(f.Symbol, spec, params, ret)
	g.line("}")
	return true
}

func (g *generator) cxaThreadAtExit(params []int, ret int) {
	if len(params) != 3 || !g.cScalar(params[1], "uintptr") || !g.cScalar(params[2], "uintptr") {
		g.fail("__cxa_thread_atexit_impl arguments")
	}
	dtor := g.destructorType(params[0])
	call := fmt.Sprintf("oxide.LibcCxaThreadAtExit(ctx,*(*uintptr)(unsafe.Pointer(&v1)),uintptr(v2),uintptr(v3),oxideDestructor%d)", dtor.ID)
	if g.cScalar(ret, "int32") {
		g.line("return %s(%s)", g.goType(ret), call)
		return
	}
	// std's registration declaration uses repr(transparent) struct c_int(i32)
	// to control CFI encoding. Keep its actual MIR aggregate representation.
	t := g.typ(ret)
	if t.Kind != "aggregate" || t.Size != 4 || t.Align != 4 || len(t.Fields) != 1 || t.Fields[0] != 0 || len(t.VariantFieldTypes) != 1 || len(t.VariantFieldTypes[0]) != 1 || g.typ(t.VariantFieldTypes[0][0]).Kind != "i32" {
		g.fail("__cxa_thread_atexit_impl result ABI")
	}
	g.line("var r %s; *(*int32)(unsafe.Pointer(&r))=%s; return r", g.goType(ret), call)
}
