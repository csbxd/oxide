package oxide

import "github.com/shogo82148/floats"

func odd128(x F128) bool {
	if f128IsZero(x) || F128IsNaN(x) || f128IsInf(x) {
		return false
	}
	s, e := unpack128(x)
	if e < 0 || e > 112 {
		return false
	}
	return U128Shr(s, uint64(112-e)).Lo&1 != 0
}

func pow128(x, y F128) floats.Float256 {
	one := F128{Hi: 0x3fff000000000000}
	if f128IsZero(y) || F128Eq(x, one) {
		return mathWide(one)
	}
	if F128IsNaN(x) || F128IsNaN(y) {
		return floats.NewFloat256NaN()
	}
	if F128Eq(y, one) {
		return mathWide(x)
	}
	ax := F128Abs(x)
	if f128IsInf(y) {
		if F128Eq(ax, one) {
			return mathWide(one)
		}
		if F128Lt(one, ax) == (y.Hi>>63 == 0) {
			return floats.NewFloat256Inf(1)
		}
		return floats.Float256{}
	}
	negative := x.Hi>>63 != 0 && F128Eq(F128Trunc(y), y) && odd128(y)
	if f128IsZero(x) || f128IsInf(x) {
		r := F128{}
		if f128IsZero(x) == (y.Hi>>63 != 0) {
			r.Hi = f128Inf
		}
		return mathWide(f128Signed(r, negative))
	}
	if x.Hi>>63 != 0 && !F128Eq(F128Trunc(y), y) {
		return floats.NewFloat256NaN()
	}
	r := expWide128(logWide128(mathWide(ax), 0).Mul(mathWide(y)), false, false)
	if negative {
		r = r.Neg()
	}
	return r
}

func erfc128(x floats.Float256) floats.Float256 {
	one := floats.NewFloat256(1)
	if x.IsNaN() {
		return floats.NewFloat256NaN()
	}
	if x.Signbit() {
		return floats.NewFloat256(2).Sub(erfc128(x.Neg()))
	}
	if x.Gt(floats.NewFloat256(128)) {
		return floats.Float256{}
	}
	if x.Lt(floats.NewFloat256(2)) {
		return one.Sub(erfSmall128(x))
	}
	// A continued fraction computes the tail directly, without subtracting
	// an erf already rounded to one. The f64 estimate controls work only.
	xf := float64(x.Float64())
	n := int(2500/(xf*xf)) + 32
	t := x.Ldexp(1)
	var r floats.Float256
	for ; n >= 1; n-- {
		r = floats.NewFloat256(float64(2 * n)).Quo(t.Add(r))
	}
	return expWide128(x.Mul(x).Neg(), false, false).Mul(f128TwoOverSqrtPi).Quo(t.Add(r))
}

func erfSmall128(x floats.Float256) floats.Float256 {
	t := x.Mul(x)
	p := f128ErfCoefficients[len(f128ErfCoefficients)-1]
	for i := len(f128ErfCoefficients) - 2; i >= 0; i-- {
		p = floats.FMA256(p, t, f128ErfCoefficients[i])
	}
	return x.Mul(p).Mul(f128TwoOverSqrtPi)
}

func erfWide128(x floats.Float256) floats.Float256 {
	if x.IsNaN() || x.IsZero() {
		return x
	}
	a := x.Abs()
	var r floats.Float256
	if a.Lt(floats.NewFloat256(2)) {
		r = erfSmall128(a)
	} else {
		r = floats.NewFloat256(1).Sub(erfc128(a))
	}
	return r.Copysign(x)
}
