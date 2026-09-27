package mir

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// rustc represents repr(simd) values as a single array field. Rust determines
// the vector alignment; lane operations use that field's exported layout.
func (g *generator) vectorInfo(id int) (elem, lanes int) {
	t := g.typ(id)
	if t.Kind == "array" {
		return t.Element, int(t.Length)
	}
	if t.Kind == "aggregate" && len(t.VariantFieldTypes) == 1 && len(t.VariantFieldTypes[0]) == 1 && len(t.Fields) == 1 && t.Fields[0] == 0 {
		return g.vectorInfo(t.VariantFieldTypes[0][0])
	}
	g.fail("SIMD vector layout %s", t.Name)
	return 0, 0
}

func constantBytes(raw json.RawMessage) ([]byte, bool) {
	return constantBytePrefix(raw, -1)
}

// repr(simd) allocations can end with uninitialized alignment padding. Only
// the bytes occupied by the requested lanes must be initialized.
func constantBytePrefix(raw json.RawMessage, n int) ([]byte, bool) {
	k, v := variant(raw)
	if k != "Constant" {
		return nil, false
	}
	var c struct {
		Const struct {
			Kind json.RawMessage `json:"kind"`
		} `json:"const_"`
	}
	if json.Unmarshal(v, &c) != nil {
		return nil, false
	}
	k, v = variant(c.Const.Kind)
	if k != "Allocated" {
		return nil, false
	}
	var a struct {
		Bytes []*uint8 `json:"bytes"`
	}
	if json.Unmarshal(v, &a) != nil {
		return nil, false
	}
	if n >= 0 {
		if n > len(a.Bytes) {
			return nil, false
		}
		a.Bytes = a.Bytes[:n]
	}
	b := make([]byte, len(a.Bytes))
	for i, x := range a.Bytes {
		if x == nil {
			return nil, false
		}
		b[i] = *x
	}
	return b, true
}

func (g *generator) simdIntrinsic(name string, args []json.RawMessage, dst Place) {
	d := g.place(dst)
	g.line("{")
	defer g.line("}")
	temporaryStorage := false
	defer func() {
		if temporaryStorage {
			g.line("ctx.Restore(vectorMark)")
		}
	}()
	// MIR operands are values. Snapshot vectors before any lane is written,
	// including constants and casts whose destination aliases source storage.
	next := 0
	operand := func(raw json.RawMessage) location {
		value, typ := g.operand(raw)
		name := fmt.Sprintf("vector%d", next)
		next++
		if g.indirectValue(typ) {
			if !temporaryStorage {
				g.line("vectorMark := ctx.Mark()")
				temporaryStorage = true
			}
			t := g.typ(typ)
			g.line("%s := ctx.Alloc(%d,%d)", name, t.Size, t.Align)
			g.copyValue("unsafe.Pointer("+name+")", "unsafe.Pointer("+value+")", t.Size)
			return g.storageLocation("unsafe.Pointer("+name+")", typ)
		}
		g.line("%s := %s", name, value)
		return location{g: g, typ: typ, local: name, address: "unsafe.Pointer(&" + name + ")"}
	}
	arity := func(n int) {
		if len(args) != n {
			g.fail("%s arity", name)
		}
	}
	lane := func(p location, elem int, i string) string {
		return fmt.Sprintf("*(*%s)(unsafe.Add(%s,uintptr(%s)*%d))", g.goType(elem), p.address, i, g.typ(elem).Size)
	}
	if name == "simd_splat" {
		arity(1)
		de, dn := g.vectorInfo(d.typ)
		x, _ := g.operand(args[0])
		g.line("for i:=uintptr(0); i<%d; i++ { %s=%s }", dn, lane(d, de, "i"), x)
		return
	}
	a := operand(args[0])
	ae, an := g.vectorInfo(a.typ)
	av := lane(a, ae, "i")
	switch name {
	case "simd_reduce_all", "simd_reduce_any":
		arity(1)
		init, expr := "true", "r && ("+av+" != 0)"
		if name == "simd_reduce_any" {
			init, expr = "false", "r || ("+av+" != 0)"
		}
		g.line("{ r:=%s; for i:=uintptr(0); i<%d; i++ { r=%s }; %s=r }", init, an, expr, d.read())
		return
	case "simd_reduce_max":
		arity(1)
		g.line("{ r:=%s; for i:=uintptr(1); i<%d; i++ { r=max(r,%s) }; %s=r }", lane(a, ae, "0"), an, av, d.read())
		return
	case "simd_reduce_min":
		arity(1)
		g.line("{ r:=%s; for i:=uintptr(1); i<%d; i++ { r=min(r,%s) }; %s=r }", lane(a, ae, "0"), an, av, d.read())
		return
	case "simd_reduce_or":
		arity(1)
		if g.typ(ae).Kind == "f32" || g.typ(ae).Kind == "f64" {
			g.fail("simd_reduce_or integer lane type")
		}
		g.line("{ var r %s; for i:=uintptr(0); i<%d; i++ { r|=%s }; %s=r }", g.goType(ae), an, av, d.read())
		return
	case "simd_select":
		arity(3)
		b := operand(args[1])
		c := operand(args[2])
		be, bn := g.vectorInfo(b.typ)
		ce, cn := g.vectorInfo(c.typ)
		de, dn := g.vectorInfo(d.typ)
		if an != bn || an != cn || an != dn || be != ce || be != de || g.typ(ae).Size != g.typ(be).Size {
			g.fail("SIMD select layout")
		}
		for i := 0; i < dn; i++ {
			g.line("if %s!=0 { %s=%s } else { %s=%s }", lane(a, ae, fmt.Sprint(i)), lane(d, de, fmt.Sprint(i)), lane(b, be, fmt.Sprint(i)), lane(d, de, fmt.Sprint(i)), lane(c, ce, fmt.Sprint(i)))
		}
		return
	case "simd_extract":
		arity(2)
		b, ok := constantBytes(args[1])
		if !ok || len(b) != 4 {
			g.fail("simd_extract constant index")
		}
		i := binary.LittleEndian.Uint32(b)
		if uint64(i) >= uint64(an) {
			g.fail("simd_extract lane %d", i)
		}
		g.line("%s=%s", d.read(), lane(a, ae, fmt.Sprint(i)))
		return
	case "simd_insert":
		arity(3)
		index, ok := constantBytes(args[1])
		if !ok || len(index) != 4 {
			g.fail("simd_insert constant index")
		}
		i := binary.LittleEndian.Uint32(index)
		de, dn := g.vectorInfo(d.typ)
		if ae != de || an != dn || uint64(i) >= uint64(an) {
			g.fail("simd_insert lane/layout %d", i)
		}
		// Snapshot the scalar too: it may refer to the destination vector.
		value := operand(args[2])
		if value.typ != ae {
			g.fail("simd_insert element type")
		}
		g.storeValue(d, a.value())
		g.line("%s=%s", lane(d, de, fmt.Sprint(i)), value.read())
		return
	case "simd_bitmask":
		arity(1)
		if an > 64 || g.scalar(g.typ(d.typ)) == "" {
			g.fail("simd_bitmask result width")
		}
		g.line("{ var r uint64; for i:=uintptr(0); i<%d; i++ { r|=((uint64(%s)>>%d)&1)<<i }; %s=%s(r) }", an, av, g.typ(ae).Size*8-1, d.read(), g.goType(d.typ))
		return
	}
	de, dn := g.vectorInfo(d.typ)
	dv := lane(d, de, "i")
	if name == "simd_fsqrt" {
		arity(1)
		if an != dn || ae != de || (g.typ(ae).Kind != "f32" && g.typ(ae).Kind != "f64") {
			g.fail("simd_fsqrt floating-point lane layout")
		}
		g.line("for i:=uintptr(0); i<%d; i++ { %s=%s(math.Sqrt(float64(%s))) }", dn, dv, g.goType(de), av)
		return
	}
	if name == "simd_cast" {
		arity(1)
		if an != dn {
			g.fail("simd_cast lane count")
		}
		// Rust simd_cast requires float-to-int inputs to be in range.
		g.line("for i:=uintptr(0); i<%d; i++ { %s=%s(%s) }", dn, dv, g.goType(de), av)
		return
	}
	if name == "simd_shuffle" {
		arity(3)
		b := operand(args[1])
		be, bn := g.vectorInfo(b.typ)
		indices, ok := constantBytePrefix(args[2], dn*4)
		if !ok || len(indices) != dn*4 || ae != be || ae != de || an != bn {
			g.fail("simd_shuffle layout")
		}
		g.line("_ = %s; _ = %s", a.read(), b.read())
		for i := 0; i < dn; i++ {
			j := int(binary.LittleEndian.Uint32(indices[i*4:]))
			if j >= an+bn {
				g.fail("simd_shuffle lane %d", j)
			}
			src := a
			if j >= an {
				src, j = b, j-an
			}
			g.line("%s=%s", lane(d, de, fmt.Sprint(i)), lane(src, de, fmt.Sprint(j)))
		}
		return
	}
	arity(2)
	b := operand(args[1])
	be, bn := g.vectorInfo(b.typ)
	if an != dn || bn != dn {
		g.fail("%s lane count", name)
	}
	bv := lane(b, be, "i")
	op := map[string]string{"simd_add": "+", "simd_sub": "-", "simd_mul": "*", "simd_div": "/", "simd_shl": "<<", "simd_shr": ">>", "simd_or": "|", "simd_and": "&", "simd_xor": "^", "simd_eq": "==", "simd_ne": "!=", "simd_lt": "<", "simd_le": "<=", "simd_gt": ">", "simd_ge": ">=", "simd_max": ">", "simd_min": "<"}[name]
	if op == "" {
		g.fail("SIMD intrinsic %s", name)
	}
	if name == "simd_max" || name == "simd_min" {
		g.line("for i:=uintptr(0); i<%d; i++ { av:=%s; bv:=%s; if av %s bv { %s=av } else { %s=bv } }", dn, av, bv, op, dv, dv)
		return
	}
	compare := strings.Contains(" simd_eq simd_ne simd_lt simd_le simd_gt simd_ge ", " "+name+" ")
	if compare {
		g.line("for i:=uintptr(0); i<%d; i++ { if %s %s %s { %s=^%s(0) } else { %s=0 } }", dn, av, op, bv, dv, g.goType(de), dv)
	} else {
		expr := fmt.Sprintf("(%s %s %s)", av, op, bv)
		if kind := g.typ(de).Kind; kind == "f32" || kind == "f64" {
			expr = g.goType(de) + expr
		}
		g.line("for i:=uintptr(0); i<%d; i++ { %s=%s }", dn, dv, expr)
	}
}
