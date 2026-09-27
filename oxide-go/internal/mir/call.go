package mir

import (
	"encoding/json"
	"fmt"
)

type callArgument struct {
	value string
	typ   int
}

// RustCall passes the final MIR argument as a tuple, but its function ABI
// spreads that tuple. rustc exports which calls use this convention. Ordinary
// tuple arguments are never inferred to be RustCall from their shape.
func (g *generator) callArguments(bb int, args []json.RawMessage) []callArgument {
	spread, ok := g.f.CallUntuple[bb]
	if ok && spread != len(args)-1 {
		g.fail("RustCall tuple must be the final operand")
	}
	out := make([]callArgument, 0, len(args))
	for i, raw := range args {
		x, id := g.operand(raw)
		if !ok || i != spread {
			out = append(out, callArgument{x, id})
			continue
		}
		t := g.typ(id)
		if t.Kind != "aggregate" || len(t.Fields) != len(t.FieldTypes) {
			g.fail("RustCall tuple layout")
		}
		k, _ := variant(raw)
		address := ""
		if g.indirectValue(id) {
			address = "unsafe.Pointer(" + x + ")"
		} else if k == "Move" || k == "Copy" {
			address = g.operandPlace(raw).address
		} else {
			address = fmt.Sprintf("unsafe.Pointer(&struct{V %s}{%s})", g.goType(id), x)
		}
		for j, ft := range t.FieldTypes {
			field := g.storageLocation(fmt.Sprintf("unsafe.Add(%s,%d)", address, t.Fields[j]), ft)
			out = append(out, callArgument{field.value(), ft})
		}
	}
	return out
}
