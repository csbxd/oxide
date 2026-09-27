package mir

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func (g *generator) tagType(tag *Tag, signed bool) *Type {
	prefix := "u"
	if signed {
		prefix = "i"
	}
	return &Type{Kind: fmt.Sprintf("%s%d", prefix, tag.Size*8), Size: tag.Size, Sized: true}
}

// rustc records discriminants and switch branches as u128 bit patterns, even
// for signed types. Normalize at the actual width before emitting Go values.
func (g *generator) integerConstant(value string, typ *Type) string {
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		g.fail("invalid integer constant %q", value)
	}
	return g.integerBits(n, typ)
}

func (g *generator) integerBits(n *big.Int, typ *Type) string {
	if typ.Size != 1 && typ.Size != 2 && typ.Size != 4 && typ.Size != 8 && typ.Size != 16 {
		g.fail("invalid integer size %d", typ.Size)
	}
	goType := g.scalar(typ)
	if goType == "" {
		g.fail("invalid integer type %s", typ.Kind)
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(typ.Size*8))
	n = new(big.Int).Mod(n, mod)
	if typ.Size == 16 {
		lo := n.Uint64()
		hi := n.Rsh(n, 64).Uint64()
		return fmt.Sprintf("%s{Lo:%d,Hi:%d}", goType, lo, hi)
	}
	if strings.HasPrefix(typ.Kind, "i") && n.Bit(int(typ.Size*8-1)) != 0 {
		n.Sub(n, mod)
	}
	return fmt.Sprintf("oxide.Scalar[%s](%s)", goType, n.String())
}

func (g *generator) integerConvert(value string, src, dst *Type) string {
	goType := g.scalar(dst)
	if goType == "" {
		g.fail("invalid discriminant type %s", dst.Kind)
	}
	if dst.Size == 16 && src.Size != 16 {
		if strings.HasPrefix(src.Kind, "i") {
			value = "oxide.I128From64(int64(" + value + "))"
		} else {
			value = "oxide.U128From64(uint64(" + value + "))"
		}
	} else if dst.Size != 16 && src.Size == 16 {
		value = "(" + value + ").Lo"
	}
	return goType + "(" + value + ")"
}

func (g *generator) variantDiscriminant(t *Type, index int, dst *Type) string {
	if index < 0 || index >= len(t.Discriminants) {
		g.fail("missing discriminant %s #%d", t.Name, index)
	}
	return g.integerConstant(t.Discriminants[index], dst)
}

func (g *generator) nicheDiscriminant(dst, src location) {
	t, dt := g.typ(src.typ), g.typ(dst.typ)
	tag := t.Tag
	if tag.First < 0 || tag.Last < tag.First || tag.Last >= len(t.Discriminants) {
		g.fail("invalid niche variants %s", t.Name)
	}
	st := g.tagType(tag, false)
	read := fmt.Sprintf("*(*%s)(unsafe.Add(%s,%d))", g.scalar(st), src.address, tag.Offset)
	start := g.integerConstant(tag.Start, st)
	delta := read + "-" + start
	limit := g.integerConstant(strconv.Itoa(tag.Last-tag.First), st)
	within := "delta <= " + limit
	if tag.Size == 16 {
		delta = "oxide.U128SubValue(" + read + "," + start + ")"
		within = "oxide.U128Le(delta," + limit + ")"
	}
	g.line("{ delta := %s; if %s {", delta, within)
	sequential := true
	for i := tag.First; i <= tag.Last; i++ {
		sequential = sequential && t.Discriminants[i] == strconv.Itoa(i)
	}
	if sequential {
		value := g.integerConvert("delta", st, dt)
		if tag.First != 0 {
			value = g.binary("Add", value, g.integerConstant(strconv.Itoa(tag.First), dt), dst.typ)
		}
		g.line("%s = %s", dst.read(), value)
	} else {
		g.line("switch delta {")
		for i := tag.First; i <= tag.Last; i++ {
			g.line("case %s: %s = %s", g.integerConstant(strconv.Itoa(i-tag.First), st), dst.read(), g.variantDiscriminant(t, i, dt))
		}
		g.line("}")
	}
	g.line("} else { %s = %s } }", dst.read(), g.variantDiscriminant(t, tag.Untagged, dt))
}
