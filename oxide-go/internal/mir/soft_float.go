package mir

import (
	"encoding/json"
	"fmt"
	"strings"
)

func softFloat(kind string) bool { return kind == "f16" || kind == "f128" }
func floatKind(kind string) bool { return softFloat(kind) || kind == "f32" || kind == "f64" }

func (g *generator) softFloatCast(x string, from, to int) (string, bool) {
	s, d := g.typ(from).Kind, g.typ(to).Kind
	if !softFloat(s) && !softFloat(d) {
		return "", false
	}
	if s == d {
		return x, true
	}
	if floatKind(s) && floatKind(d) {
		return "oxide." + strings.ToUpper(s) + "To" + strings.ToUpper(d) + "(" + x + ")", true
	}
	if floatKind(d) {
		prefix := "Int"
		if s == "u128" || s == "i128" {
			prefix = strings.ToUpper(s)
		}
		return "oxide." + prefix + "To" + strings.ToUpper(d) + "(" + x + ")", true
	}
	fn := "Int[" + g.goType(to) + "]"
	if d == "i128" || d == "u128" {
		fn = strings.ToUpper(d)
	}
	return "oxide." + strings.ToUpper(s) + "To" + fn + "(" + x + ")", true
}

func (g *generator) softFloatBinary(op, a, b, kind string) string {
	prefix := "oxide." + strings.ToUpper(kind)
	switch op {
	case "Add", "Sub", "Mul", "Div", "Rem", "Eq", "Lt", "Le":
		return fmt.Sprintf("%s%s(%s,%s)", prefix, op, a, b)
	case "Ne":
		return fmt.Sprintf("!%sEq(%s,%s)", prefix, a, b)
	case "Gt":
		return fmt.Sprintf("%sLt(%s,%s)", prefix, b, a)
	case "Ge":
		return fmt.Sprintf("%sLe(%s,%s)", prefix, b, a)
	}
	g.fail("binary operation %s on %s", op, kind)
	return ""
}

func (g *generator) softFloatIntrinsic(name string, args []json.RawMessage, dst Place) bool {
	if len(args) == 0 {
		return false
	}
	x, t := g.operand(args[0])
	kind := g.typ(t).Kind
	if !softFloat(kind) {
		return false
	}
	prefix := "oxide." + strings.ToUpper(kind)
	fn, arity := "", 1
	switch name {
	case "float_to_int_unchecked":
		if len(args) != 1 {
			g.fail("%s arity", name)
		}
		d := g.place(dst)
		expr, _ := g.softFloatCast(x, t, d.typ)
		g.storeValue(d, expr)
		return true
	case "fabs", "fabsf16", "fabsf128":
		fn = prefix + "Abs"
	case "sqrt", "sqrtf16", "sqrtf128":
		fn = prefix + "Sqrt"
	case "floor", "floorf16", "floorf128":
		fn = prefix + "Floor"
	case "ceil", "ceilf16", "ceilf128":
		fn = prefix + "Ceil"
	case "round", "roundf16", "roundf128":
		fn = prefix + "Round"
	case "truncf16", "truncf128":
		fn = prefix + "Trunc"
	case "round_ties_even_f16", "round_ties_even_f128":
		fn = prefix + "RoundEven"
	case "copysignf16", "copysignf128":
		fn, arity = prefix+"Copysign", 2
	case "powif16", "powif128":
		fn, arity = prefix+"Powi", 2
	case "minimum_number_nsz_f16", "minimum_number_nsz_f128":
		fn, arity = prefix+"Min", 2
	case "maximum_number_nsz_f16", "maximum_number_nsz_f128":
		fn, arity = prefix+"Max", 2
	case "fmaf16", "fmaf128":
		fn, arity = "oxide.FMA"+kind[1:], 3
	case "exp", "exp2", "log", "log2", "log10", "sin", "cos", "powf16", "powf128":
		op := name
		if strings.HasPrefix(name, "powf") {
			op, arity = "pow", 2
		}
		if kind == "f128" {
			if len(args) != arity {
				g.fail("%s arity", name)
			}
			y := "oxide.F128{}"
			if arity == 2 {
				y, _ = g.operand(args[1])
			}
			g.storeValue(g.place(dst), fmt.Sprintf("oxide.F128Math(ctx,%q,%s,%s)", op, x, y))
			return true
		}
		fn = prefix + strings.ToUpper(op[:1]) + op[1:]
	default:
		return false
	}
	if len(args) != arity {
		g.fail("%s arity", name)
	}
	values := []string{x}
	for _, arg := range args[1:] {
		v, _ := g.operand(arg)
		values = append(values, v)
	}
	g.storeValue(g.place(dst), fn+"("+strings.Join(values, ",")+")")
	return true
}
