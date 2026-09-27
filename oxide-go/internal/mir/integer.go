package mir

import (
	"encoding/json"
	"strings"
)

func (g *generator) carryingMulAdd(args []json.RawMessage, dst location) {
	if len(args) != 4 {
		g.fail("carrying_mul_add arity")
	}
	values := make([]string, 4)
	var input int
	for i, arg := range args {
		x, t := g.operand(arg)
		if i == 0 {
			input = t
		} else if t != input {
			g.fail("carrying_mul_add operand types")
		}
		values[i] = x
	}
	t, result := g.typ(input), g.typ(dst.typ)
	if len(result.Fields) != 2 || len(result.FieldTypes) != 2 {
		g.fail("carrying_mul_add tuple layout")
	}
	low, high := result.FieldTypes[0], result.FieldTypes[1]
	if g.typ(low).Size != t.Size || g.typ(high).Size != t.Size || !strings.HasPrefix(g.typ(low).Kind, "u") {
		g.fail("carrying_mul_add output types")
	}
	g.line("{")
	if t.Size < 8 {
		wide := "uint64"
		if strings.HasPrefix(t.Kind, "i") {
			wide = "int64"
		}
		g.line("r:=%s(%s)*%s(%s)+%s(%s)+%s(%s)", wide, values[0], wide, values[1], wide, values[2], wide, values[3])
		g.line("lo,hi:=r,r>>%d", t.Size*8)
	} else {
		prefix := strings.ToUpper(t.Kind[:1])
		if t.Size == 8 {
			cast := "uint64"
			if prefix == "I" {
				cast = "int64"
			}
			for i, x := range values {
				values[i] = cast + "(" + x + ")"
			}
		}
		g.line("lo,hi:=oxide.%s%dCarryingMulAdd(%s)", prefix, t.Size*8, strings.Join(values, ","))
	}
	g.line("*(*%s)(unsafe.Add(%s,%d))=%s(lo)", g.goType(low), dst.address, result.Fields[0], g.goType(low))
	g.line("*(*%s)(unsafe.Add(%s,%d))=%s(hi)", g.goType(high), dst.address, result.Fields[1], g.goType(high))
	g.line("}")
}
