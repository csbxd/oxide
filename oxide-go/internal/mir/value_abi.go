package mir

import (
	"fmt"
	"math/big"
	"strings"
)

// Use rustc's value representation for transparent wrappers and niche enums.
// Matching only their memory layout is insufficient: Go passes byte arrays on
// the stack, but passes the equivalent scalar in a register. Rust permits
// function-pointer casts between ABI-compatible wrappers and their scalars.
func (g *generator) primitive(p Primitive) (string, uint64) {
	if p.Pointer != nil {
		if *p.Pointer != 0 {
			g.fail("unsupported pointer address space %d", *p.Pointer)
		}
		return "uintptr", 8
	}
	if p.Int != nil {
		prefix := "uint"
		if p.Int.Signed {
			prefix = "int"
		}
		switch p.Int.Length {
		case "I8":
			return prefix + "8", 1
		case "I16":
			return prefix + "16", 2
		case "I32":
			return prefix + "32", 4
		case "I64":
			return prefix + "64", 8
		case "I128":
			if p.Int.Signed {
				return "oxide.I128", 16
			}
			return "oxide.U128", 16
		}
	}
	if p.Float != nil {
		switch p.Float.Length {
		case "F16":
			return "oxide.F16", 2
		case "F32":
			return "float32", 4
		case "F64":
			return "float64", 8
		case "F128":
			return "oxide.F128", 16
		}
	}
	g.fail("unsupported ABI primitive %+v", p)
	return "", 0
}

func (g *generator) scalarPairDecl(t *Type) {
	if t.ABIPair == nil {
		g.fail("missing scalar pair ABI for %s", t.Name)
	}
	p := t.ABIPair
	a, as := g.primitive(p.A)
	b, bs := g.primitive(p.B)
	ba := min(bs, 8)
	if p.BOffset < as || p.BOffset%ba != 0 || p.BOffset+bs > t.Size {
		g.fail("invalid scalar pair layout for %s", t.Name)
	}
	g.line("// T%d: %s (Rust size %d, align %d).", t.ID, t.Name, t.Size, t.Align)
	g.line("type T%d = struct { _ [0]uint%d; A %s", t.ID, min(t.Align, 8)*8, a)
	if p.BOffset != alignUp(as, ba) {
		g.line("_ [%d]byte", p.BOffset-as)
	}
	g.line("B %s", b)
	if t.Size != alignUp(p.BOffset+bs, min(t.Align, 8)) {
		g.line("_ [%d]byte", t.Size-(p.BOffset+bs))
	}
	g.line("}")
	g.line("var _ [%d-int(unsafe.Sizeof(T%d{}))]byte; var _ [int(unsafe.Sizeof(T%d{}))-%d]byte", t.Size, t.ID, t.ID, t.Size)
	g.line("var _ [%d-int(unsafe.Offsetof(T%d{}.B))]byte; var _ [int(unsafe.Offsetof(T%d{}.B))-%d]byte", p.BOffset, t.ID, t.ID, p.BOffset)
}

func (g *generator) scalarPairConstant(t *Type, data []byte) string {
	if t.ABIPair == nil {
		g.fail("missing scalar pair ABI for %s", t.Name)
	}
	p := t.ABIPair
	_, as := g.primitive(p.A)
	_, bs := g.primitive(p.B)
	if uint64(len(data)) < p.BOffset+bs || p.BOffset < as {
		g.fail("short scalar pair constant for %s", t.Name)
	}
	return fmt.Sprintf("T%d{A:%s,B:%s}", t.ID, g.primitiveConstant(p.A, data[:as]), g.primitiveConstant(p.B, data[p.BOffset:p.BOffset+bs]))
}

func (g *generator) primitiveConstant(p Primitive, data []byte) string {
	t, size := g.primitive(p)
	if uint64(len(data)) != size {
		g.fail("invalid %s ABI constant size %d", t, len(data))
	}
	bits := new(big.Int)
	for i := len(data) - 1; i >= 0; i-- {
		bits.Lsh(bits, 8)
		bits.Or(bits, big.NewInt(int64(data[i])))
	}
	switch t {
	case "float32":
		return "math.Float32frombits(" + bits.String() + ")"
	case "float64":
		return "math.Float64frombits(" + bits.String() + ")"
	case "oxide.U128", "oxide.I128", "oxide.F128":
		lo := bits.Uint64()
		hi := new(big.Int).Rsh(bits, 64).Uint64()
		return fmt.Sprintf("%s{Lo:%d,Hi:%d}", t, lo, hi)
	}
	if strings.HasPrefix(t, "int") && data[len(data)-1]&128 != 0 {
		bits.Sub(bits, new(big.Int).Lsh(big.NewInt(1), uint(size*8)))
	}
	return fmt.Sprintf("%s(%s)", t, bits.String())
}
