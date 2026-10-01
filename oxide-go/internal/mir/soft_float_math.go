package mir

import "strings"

func (g *generator) f128MathFunction(f *Function, params []int, ret int) bool {
	if f.Signature.Variadic {
		return false
	}
	if f.Symbol == "lgammaf128_r" {
		if len(params) != 2 || !g.cScalar(params[0], "oxide.F128") || !g.cScalar(params[1], "uintptr") || !g.cScalar(ret, "oxide.F128") {
			g.fail("binary128 C math signature %s", f.Symbol)
		}
		g.line("value,sign:=oxide.F128MathResult(ctx,\"lgamma\",v1,oxide.F128{}); *(*int32)(unsafe.Pointer(v2))=sign; return value }")
		return true
	}
	name := strings.TrimSuffix(f.Symbol, "f128")
	if name == f.Symbol {
		return false
	}
	arity := 1
	switch name {
	case "acos", "asin", "atan", "acosh", "asinh", "atanh", "cbrt", "cos", "sin", "tan", "cosh", "sinh", "tanh", "exp", "exp2", "expm1", "log", "log2", "log10", "log1p", "tgamma", "erf", "erfc":
	case "atan2", "hypot", "pow":
		arity = 2
	default:
		return false
	}
	if len(params) != arity || !g.cScalar(ret, "oxide.F128") {
		g.fail("binary128 C math signature %s", f.Symbol)
	}
	for _, t := range params {
		if !g.cScalar(t, "oxide.F128") {
			g.fail("binary128 C math argument %s", f.Symbol)
		}
	}
	y := "oxide.F128{}"
	if arity == 2 {
		y = "v2"
	}
	g.line("return oxide.F128Math(ctx,%q,v1,%s) }", name, y)
	return true
}
