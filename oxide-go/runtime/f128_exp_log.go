package oxide

import "github.com/shogo82148/floats"

// Horner evaluation avoids separately recomputing each power and factorial.
// The reduced domains give substantially more accuracy than binary128 needs;
// all coefficients and intermediate arithmetic retain 237 significant bits.
func expWide128(x floats.Float256, base2, minusOne bool) floats.Float256 {
	one := floats.NewFloat256(1)
	if x.IsNaN() {
		return floats.NewFloat256NaN()
	}
	limit := floats.NewFloat256(12000)
	if base2 {
		limit = floats.NewFloat256(17000)
	}
	if x.Gt(limit) {
		return floats.NewFloat256Inf(1)
	}
	if x.Lt(limit.Neg()) {
		if minusOne {
			return one.Neg()
		}
		return floats.Float256{}
	}
	k := int64(0)
	var r floats.Float256
	if base2 {
		k = x.RoundToEven().Int64()
		r = x.Sub(floats.NewFloat256(float64(k))).Mul(f128Ln2)
	} else {
		k = x.Mul(f128Log2E).RoundToEven().Int64()
		r = floats.FMA256(floats.NewFloat256(-float64(k)), f128Ln2, x)
	}
	p := f128ExpCoefficients[len(f128ExpCoefficients)-1]
	for i := len(f128ExpCoefficients) - 2; i >= 0; i-- {
		p = floats.FMA256(p, r, f128ExpCoefficients[i])
	}
	if minusOne && k == 0 {
		return r.Mul(p)
	}
	result := floats.FMA256(r, p, one).Ldexp(int(k))
	if minusOne {
		result = result.Sub(one)
	}
	return result
}

func atanhSeries128(x floats.Float256) floats.Float256 {
	t := x.Mul(x)
	p := f128LogCoefficients[len(f128LogCoefficients)-1]
	for i := len(f128LogCoefficients) - 2; i >= 0; i-- {
		p = floats.FMA256(p, t, f128LogCoefficients[i])
	}
	return x.Mul(p).Ldexp(1)
}

func logWide128(x floats.Float256, base int) floats.Float256 {
	if x.IsNaN() || x.Lt(floats.Float256{}) {
		return floats.NewFloat256NaN()
	}
	if x.IsZero() {
		return floats.NewFloat256Inf(-1)
	}
	if x.IsInf(1) {
		return x
	}
	m, e := x.Frexp()
	if m.Lt(f128SqrtHalf) {
		m = m.Ldexp(1)
		e--
	}
	one := floats.NewFloat256(1)
	z := m.Sub(one).Quo(m.Add(one))
	r := atanhSeries128(z)
	k := floats.NewFloat256(float64(e))
	if base == 2 {
		return floats.FMA256(r, f128Log2E, k)
	}
	r = floats.FMA256(k, f128Ln2, r)
	if base == 10 {
		r = r.Mul(f128Log10E)
	}
	return r
}

func log1pWide128(x floats.Float256) floats.Float256 {
	one := floats.NewFloat256(1)
	if x.IsNaN() || x.Lt(one.Neg()) {
		return floats.NewFloat256NaN()
	}
	if x.IsZero() || x.IsInf(1) {
		return x
	}
	if x.Eq(one.Neg()) {
		return floats.NewFloat256Inf(-1)
	}
	if x.Abs().Le(floats.NewFloat256(0.25)) {
		return atanhSeries128(x.Quo(x.Add(floats.NewFloat256(2))))
	}
	return logWide128(one.Add(x), 0)
}

func hyperbolic128(x floats.Float256, op string) floats.Float256 {
	if x.IsNaN() {
		return floats.NewFloat256NaN()
	}
	one := floats.NewFloat256(1)
	two := floats.NewFloat256(2)
	a := x.Abs()
	var r floats.Float256
	switch op {
	case "sinh":
		r = expWide128(a, false, true)
		if !r.IsInf(1) {
			r = r.Add(r.Quo(r.Add(one))).Ldexp(-1)
		}
	case "cosh":
		r = expWide128(a, false, false)
		return r.Add(one.Quo(r)).Ldexp(-1)
	case "tanh":
		r = expWide128(a.Ldexp(1).Neg(), false, true)
		r = r.Neg().Quo(r.Add(two))
	case "asinh":
		if a.IsInf(1) {
			return x
		}
		t := a.Mul(a)
		r = log1pWide128(a.Add(t.Quo(t.Add(one).Sqrt().Add(one))))
	case "acosh":
		if x.Lt(one) {
			return floats.NewFloat256NaN()
		}
		if x.IsInf(1) {
			return x
		}
		t := x.Sub(one)
		return log1pWide128(t.Add(t.Mul(t.Add(two)).Sqrt()))
	case "atanh":
		if a.Gt(one) {
			return floats.NewFloat256NaN()
		}
		r = log1pWide128(a.Ldexp(1).Quo(one.Sub(a))).Ldexp(-1)
	}
	return r.Copysign(x)
}
