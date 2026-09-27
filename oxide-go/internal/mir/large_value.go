package mir

import "strings"

// Go 1.27 permits explicit stack variables up to 128 KiB and implicit ones up
// to 64 KiB; -smallframes lowers those limits to 64/16 KiB. Classify only by
// Rust size so ABI-compatible wrappers always use the same calling convention.
const directValueLimit = 16 << 10

func (g *generator) indirectValue(id int) bool {
	return g.typ(id).Size > directValueLimit
}

func (g *generator) argumentType(id int) string {
	if g.indirectValue(id) {
		return "uintptr"
	}
	return g.goType(id)
}

func (g *generator) returnType(id int) string {
	if g.indirectValue(id) {
		return ""
	}
	return g.goType(id)
}

func (g *generator) returnZero(id int) string {
	if g.indirectValue(id) {
		return ""
	}
	return g.zero(id)
}

// Large operand values are addresses of existing storage, never Go aggregate
// expressions. Every callee copies an indirect parameter into its own frame.
func (p location) value() string {
	if p.g.indirectValue(p.typ) {
		return "uintptr(" + p.address + ")"
	}
	return p.read()
}

func (g *generator) copyValue(dst, src string, size uint64) {
	g.line("copy(unsafe.Slice((*byte)(%s),%d),unsafe.Slice((*byte)(%s),%d))", dst, size, src, size)
}

func (g *generator) storeValue(dst location, value string) {
	if g.indirectValue(dst.typ) {
		g.copyValue(dst.address, "unsafe.Pointer("+value+")", g.typ(dst.typ).Size)
		return
	}
	g.line("%s = %s", dst.read(), value)
}

func (g *generator) callResult(dst location, callee string, args []string) {
	if g.indirectValue(dst.typ) {
		call := []string{args[0], "uintptr(" + dst.address + ")"}
		call = append(call, args[1:]...)
		g.line("%s(%s)", callee, strings.Join(call, ","))
		return
	}
	g.line("%s = %s(%s)", dst.read(), callee, strings.Join(args, ","))
}

func (g *generator) returnValue(ret int) {
	p := g.place(Place{Local: 0})
	if g.indirectValue(ret) {
		g.copyValue("unsafe.Pointer(result)", p.address, g.typ(ret).Size)
		g.line("return")
		return
	}
	g.line("return %s", p.read())
}

func (g *generator) storageLocation(address string, id int) location {
	return location{g: g, address: address, typ: id, variant: -1}
}
