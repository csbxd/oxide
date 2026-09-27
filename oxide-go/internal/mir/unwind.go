package mir

import (
	"encoding/json"
	"fmt"
)

func (g *generator) catchUnwind(args []json.RawMessage, dst location) {
	if len(args) != 3 || g.typ(dst.typ).Kind != "bool" {
		g.fail("catch_unwind signature")
	}
	try, tryID := g.operand(args[0])
	data, dataID := g.operand(args[1])
	catch, catchID := g.operand(args[2])
	t, c := g.typ(tryID), g.typ(catchID)
	if t.Kind != "fnptr" || c.Kind != "fnptr" || t.FnVariadic || c.FnVariadic || len(t.FnInputs) != 1 || len(c.FnInputs) != 2 || g.typ(t.FnOutput).Size != 0 || g.typ(c.FnOutput).Size != 0 || !g.cScalar(dataID, "uintptr") {
		g.fail("catch_unwind callback ABI")
	}
	for _, id := range append(append([]int{}, t.FnInputs...), c.FnInputs...) {
		if !g.cScalar(id, "uintptr") {
			g.fail("catch_unwind callback pointer ABI")
		}
	}
	g.line("{ tryDescriptor,catchDescriptor,data:=%s,%s,%s", try, catch, data)
	g.line("tryFn:=*(*func(*oxide.Context,%s) %s)(unsafe.Pointer(&tryDescriptor)); _=tryFn(ctx,%s(data))", g.goType(t.FnInputs[0]), g.goType(t.FnOutput), g.goType(t.FnInputs[0]))
	g.line("%s=ctx.Failed(); if ctx.Failed() {", dst.read())
	g.line("exception:=ctx.TakeException()")
	g.line("catchFn:=*(*func(*oxide.Context,%s,%s) %s)(unsafe.Pointer(&catchDescriptor)); _=catchFn(ctx,%s(data),%s(exception))", g.goType(c.FnInputs[0]), g.goType(c.FnInputs[1]), g.goType(c.FnOutput), g.goType(c.FnInputs[0]), g.goType(c.FnInputs[1]))
	g.line("if ctx.Failed() { oxide.Abort() } } }")
}

func (g *generator) unwindFunction(f *Function, params []int, ret int) bool {
	if spec, ok := map[string]cFunction{
		"_Unwind_GetIP":                 {"UnwindGetIP", "uintptr", "uintptr"},
		"_Unwind_GetCFA":                {"UnwindGetCFA", "uintptr", "uintptr"},
		"_Unwind_FindEnclosingFunction": {"UnwindFindEnclosingFunction", "uintptr", "uintptr"},
	}[f.Symbol]; ok {
		g.cFunctionBody(f.Symbol, spec, params, ret)
		g.line("}")
		return true
	}
	if f.Symbol == "_Unwind_Backtrace" {
		if len(params) != 2 || !g.cScalar(params[1], "uintptr") || g.typ(ret).Size != 4 {
			g.fail("_Unwind_Backtrace ABI")
		}
		t := g.typ(params[0])
		if t.Kind != "fnptr" || t.FnABI != "C" || t.FnVariadic || len(t.FnInputs) != 2 || !g.cScalar(t.FnInputs[0], "uintptr") || !g.cScalar(t.FnInputs[1], "uintptr") || g.typ(t.FnOutput).Size != 4 {
			g.fail("_Unwind_Trace_Fn ABI")
		}
		g.line("var result %s; code:=oxide.UnwindBacktrace(ctx,uintptr(v1),uintptr(v2),oxideTrace%d); *(*uint32)(unsafe.Pointer(&result))=code; return result }", g.goType(ret), t.ID)
		g.line("func oxideTrace%d(ctx *oxide.Context,descriptor,frame,data uintptr) uint32 {", t.ID)
		g.line("f:=*(*func(*oxide.Context,%s,%s) %s)(unsafe.Pointer(&descriptor)); result:=f(ctx,%s(frame),%s(data)); return *(*uint32)(unsafe.Pointer(&result)) }", g.goType(t.FnInputs[0]), g.goType(t.FnInputs[1]), g.goType(t.FnOutput), g.goType(t.FnInputs[0]), g.goType(t.FnInputs[1]))
		return true
	}
	if f.Symbol == "dl_iterate_phdr" {
		if len(params) != 2 || !g.cScalar(params[1], "uintptr") || !g.cScalar(ret, "int32") {
			g.fail("dl_iterate_phdr ABI")
		}
		t := g.typ(params[0])
		if t.Kind == "aggregate" {
			if t.Size != 8 || t.Tag == nil || t.Tag.Encoding != "niche" || t.Tag.Start != "0" {
				g.fail("dl_iterate_phdr nullable callback layout")
			}
			for _, v := range t.VariantFieldTypes {
				if len(v) == 1 && g.typ(v[0]).Kind == "fnptr" {
					t = g.typ(v[0])
					break
				}
			}
		}
		if t.Kind != "fnptr" || t.FnABI != "C" || t.FnVariadic || len(t.FnInputs) != 3 || !g.cScalar(t.FnOutput, "int32") {
			g.fail("dl_iterate_phdr callback ABI")
		}
		for _, id := range t.FnInputs {
			if !g.cScalar(id, "uintptr") {
				g.fail("dl_iterate_phdr callback argument ABI")
			}
		}
		g.line("return %s(oxide.LibcDlIteratePhdr(ctx,*(*uintptr)(unsafe.Pointer(&v1)),uintptr(v2),oxidePhdr%d)) }", g.goType(ret), t.ID)
		g.line("func oxidePhdr%d(ctx *oxide.Context,descriptor,info,size,data uintptr) int32 {", t.ID)
		args := fmt.Sprintf("%s(info),%s(size),%s(data)", g.goType(t.FnInputs[0]), g.goType(t.FnInputs[1]), g.goType(t.FnInputs[2]))
		g.line("f:=*(*func(*oxide.Context,%s,%s,%s) %s)(unsafe.Pointer(&descriptor)); return int32(f(ctx,%s)) }", g.goType(t.FnInputs[0]), g.goType(t.FnInputs[1]), g.goType(t.FnInputs[2]), g.goType(t.FnOutput), args)
		return true
	}
	if f.Symbol == "_Unwind_RaiseException" {
		if len(params) != 1 || g.typ(params[0]).Kind != "pointer" || g.typ(ret).Size != 4 {
			g.fail("_Unwind_RaiseException ABI")
		}
		g.line("ctx.RaiseException(uintptr(v1)); return %s }", g.zero(ret))
		return true
	}
	if f.Symbol == "_Unwind_DeleteException" {
		if len(params) != 1 || g.typ(params[0]).Kind != "pointer" || g.typ(ret).Size != 0 {
			g.fail("_Unwind_DeleteException ABI")
		}
		t := g.typ(g.typ(params[0]).Pointee)
		if len(t.Fields) < 2 || len(t.VariantFieldTypes) != 1 || len(t.VariantFieldTypes[0]) < 2 {
			g.fail("_Unwind_Exception layout")
		}
		ft := g.typ(t.VariantFieldTypes[0][1])
		// exception_cleanup is nullable extern C fn(reason, exception).
		var fn *Type
		for _, fields := range ft.VariantFieldTypes {
			if len(fields) == 1 && g.typ(fields[0]).Kind == "fnptr" {
				fn = g.typ(fields[0])
			}
		}
		if fn == nil || len(fn.FnInputs) != 2 || g.typ(fn.FnOutput).Size != 0 || g.typ(fn.FnInputs[0]).Size != 4 || !g.cScalar(fn.FnInputs[1], "uintptr") {
			g.fail("exception_cleanup ABI")
		}
		g.line("descriptor:=*(*uintptr)(unsafe.Add(unsafe.Pointer(v1),%d)); if descriptor!=0 {", t.Fields[1])
		g.line("var reason %s; *(*uint32)(unsafe.Pointer(&reason))=1", g.goType(fn.FnInputs[0]))
		g.line("f:=*(*func(*oxide.Context,%s,%s) %s)(unsafe.Pointer(&descriptor)); _=f(ctx,reason,%s(v1))", g.goType(fn.FnInputs[0]), g.goType(fn.FnInputs[1]), g.goType(fn.FnOutput), g.goType(fn.FnInputs[1]))
		g.line("if ctx.Failed() { oxide.Abort() } }; return %s }", g.zero(ret))
		return true
	}
	return false
}
