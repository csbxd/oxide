package oxide

import "github.com/shogo82148/floats"

// Binary128Math optionally overrides the built-in binary128 math. Inputs and
// results are raw IEEE bits. Unary operations receive a zero y; lgamma returns
// its sign separately. Contexts are confined to one call chain.
type Binary128Math func(ctx *Context, operation string, x, y F128) (value F128, sign int32)

func F128Math(ctx *Context, operation string, x, y F128) F128 {
	value, _ := F128MathResult(ctx, operation, x, y)
	return value
}

func F128MathResult(ctx *Context, operation string, x, y F128) (F128, int32) {
	if ctx.Binary128Math != nil {
		return ctx.Binary128Math(ctx, operation, x, y)
	}
	return f128Math(operation, x, y)
}

func mathWide(x F128) floats.Float256   { return (floats.Float128{x.Hi, x.Lo}).Float256() }
func mathNarrow(x floats.Float256) F128 { r := x.Float128(); return F128{Lo: r[1], Hi: r[0]} }

// Use 237-bit fixed-size intermediates, then round to binary128. Extra
// precision also leaves room for cancellation and argument reduction.
func f128Math(operation string, x, y F128) (F128, int32) {
	a, b := mathWide(x), mathWide(y)
	var r floats.Float256
	sign := 0
	switch operation {
	case "exp":
		r = expWide128(a, false, false)
	case "exp2":
		r = expWide128(a, true, false)
	case "expm1":
		r = expWide128(a, false, true)
	case "log":
		r = logWide128(a, 0)
	case "log2":
		r = logWide128(a, 2)
	case "log10":
		r = logWide128(a, 10)
	case "log1p":
		r = log1pWide128(a)
	case "sin", "cos", "tan":
		r = trig128(x, operation)
	case "asin":
		r = a.Asin()
	case "acos":
		r = a.Acos()
	case "atan":
		r = a.Atan()
	case "atan2":
		r = a.Atan2(b)
	case "sinh", "cosh", "tanh", "asinh", "acosh", "atanh":
		r = hyperbolic128(a, operation)
	case "cbrt":
		r = a.Cbrt()
	case "hypot":
		r = floats.Hypot256(a, b)
	case "pow":
		r = pow128(x, y)
	case "tgamma":
		r = a.Gamma()
	case "lgamma":
		g := a.Gamma()
		if !g.IsInf(0) && !g.IsZero() && !g.IsNaN() {
			r, sign = logWide128(g.Abs(), 0), 1
			if g.Signbit() {
				sign = -1
			}
		} else {
			r, sign = a.Lgamma()
		}
		if a.IsInf(0) {
			r, sign = floats.NewFloat256Inf(1), 1
		}
		if f128IsZero(x) && x.Hi>>63 != 0 {
			sign = -1
		}
		if x.Hi>>63 != 0 && !f128IsZero(x) && !f128IsInf(x) && !F128IsNaN(x) && !F128Eq(F128Trunc(x), x) {
			sign = -1
			if odd128(F128Floor(F128Abs(x))) {
				sign = 1
			}
		}
	case "erf":
		r = erfWide128(a)
	case "erfc":
		r = erfc128(a)
	default:
		panic("oxide: unknown binary128 math operation " + operation)
	}
	return mathNarrow(r), int32(sign)
}
