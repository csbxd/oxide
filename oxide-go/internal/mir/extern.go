package mir

import (
	"fmt"
	"strings"
)

func (g *generator) runtimeFunction(f *Function, params []int, ret int) bool {
	if g.cRuntimeFunction(f, params, ret) {
		return true
	}
	name := f.Name
	if i := strings.LastIndex(name, "::"); i >= 0 {
		name = name[i+2:]
	}
	if strings.HasPrefix(name, "__rust_") && !strings.HasPrefix(f.Name, "alloc::alloc::") {
		return false
	}
	switch name {
	case "now":
		if f.Name != "std::time::Instant::now" || len(params) != 0 {
			return false
		}
		sec, ns, okSec, okNS := g.instantFields(ret, 0)
		if !okSec || !okNS {
			g.fail("unsupported Rust Instant layout %s", g.typ(ret).Name)
		}
		g.line("{ var r %s; sec,nsec:=oxide.MonotonicTimespec(); *(*int64)(unsafe.Add(unsafe.Pointer(&r),%d))=sec; *(*uint32)(unsafe.Add(unsafe.Pointer(&r),%d))=uint32(nsec); return r }", g.goType(ret), sec, ns)
		g.line("}")
		return true
	case "elapsed":
		if f.Name != "std::time::Instant::elapsed" || len(params) != 1 {
			return false
		}
		r := g.typ(ret)
		if len(r.Fields) < 2 {
			g.fail("unsupported Rust Duration layout")
		}
		g.line("{ sec,nsec:=oxide.InstantElapsed(v1); var r %s; *(*uint64)(unsafe.Add(unsafe.Pointer(&r),%d))=sec; *(*uint32)(unsafe.Add(unsafe.Pointer(&r),%d))=nsec; return r }", g.goType(ret), r.Fields[0], r.Fields[1])
		g.line("}")
		return true
	case "__rust_alloc", "__rust_alloc_zeroed":
		if len(params) != 2 {
			g.fail("Rust allocator signature")
		}
		g.line("return oxide.RustAlloc(%s,%s)", g.allocatorWord(params, 1), g.allocatorWord(params, 2))
	case "__rust_dealloc":
		if len(params) != 3 {
			g.fail("Rust deallocator signature")
		}
		g.line("oxide.RustDealloc(%s,%s,%s); return %s", g.allocatorWord(params, 1), g.allocatorWord(params, 2), g.allocatorWord(params, 3), g.zero(ret))
	case "__rust_realloc":
		if len(params) != 4 {
			g.fail("Rust realloc signature")
		}
		g.line("return oxide.RustRealloc(%s,%s,%s,%s)", g.allocatorWord(params, 1), g.allocatorWord(params, 2), g.allocatorWord(params, 3), g.allocatorWord(params, 4))
	case "__rust_alloc_error_handler":
		g.line("oxide.Abort(); return %s", g.zero(ret))
	case "__rust_no_alloc_shim_is_unstable_v2":
		g.line("return %s", g.zero(ret))
	default:
		return false
	}
	g.line("}")
	return true
}

// The pinned allocator ABI uses transparent Alignment and NonNull newtypes.
// Their MIR parameters retain these types instead of the scalar C ABI form.
func (g *generator) allocatorWord(params []int, index int) string {
	t := g.typ(params[index-1])
	if t.Size != 8 || t.Align != 8 {
		g.fail("allocator argument %d has layout %s (%d, %d)", index, t.Name, t.Size, t.Align)
	}
	if g.scalar(t) == "uintptr" {
		return fmt.Sprintf("v%d", index)
	}
	return fmt.Sprintf("*(*uintptr)(unsafe.Pointer(&v%d))", index)
}

func (g *generator) instantFields(id int, base uint64) (sec, ns uint64, okSec, okNS bool) {
	t := g.typ(id)
	if t.Kind == "i64" {
		return base, 0, true, false
	}
	if t.Kind == "u32" {
		return 0, base, false, true
	}
	if t.Kind == "aggregate" && t.Size == 8 && t.Align == 8 {
		return base, 0, true, false
	}
	if t.Kind == "aggregate" && t.Size == 4 && t.Align == 4 {
		return 0, base, false, true
	}
	if t.Kind != "aggregate" || len(t.VariantFieldTypes) == 0 || len(t.VariantFieldTypes[0]) != len(t.Fields) {
		return 0, 0, false, false
	}
	for i, ft := range t.VariantFieldTypes[0] {
		s, n, hasSec, hasNS := g.instantFields(ft, base+t.Fields[i])
		if hasSec {
			sec = s
			okSec = true
		}
		if hasNS {
			ns = n
			okNS = true
		}
	}
	return sec, ns, okSec, okNS
}
