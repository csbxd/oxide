package oxide

// F128 stores IEEE binary128 bits in little-endian word order. Rust storage
// has the compiler's alignment (16 on our targets), supplied by Context and
// public storage views; Go's by-value representation itself aligns to 8.
type F128 struct{ Lo, Hi uint64 }

const (
	f128Sign = uint64(1) << 63
	f128Frac = uint64(1)<<48 - 1
	f128Inf  = uint64(0x7fff) << 48
)

func F128Neg(x F128) F128 { x.Hi ^= f128Sign; return x }
func F128Abs(x F128) F128 { x.Hi &^= f128Sign; return x }
func F128Copysign(x, y F128) F128 {
	x.Hi = x.Hi&^f128Sign | y.Hi&f128Sign
	return x
}
func F128IsNaN(x F128) bool  { return x.Hi&^f128Sign >= f128Inf && (x.Hi&f128Frac != 0 || x.Lo != 0) }
func f128IsInf(x F128) bool  { return x.Hi&^f128Sign == f128Inf && x.Lo == 0 }
func f128IsZero(x F128) bool { return x.Hi&^f128Sign == 0 && x.Lo == 0 }
func f128NaN() F128          { return F128{Hi: f128Inf | 1<<47} }
func f128Signed(x F128, negative bool) F128 {
	if negative {
		x.Hi |= f128Sign
	}
	return x
}

// Finite nonzero values are sig * 2^(exp-112), with sig's bit 112 set.
func unpack128(x F128) (sig U128, exp int) {
	sig = U128{x.Lo, x.Hi & f128Frac}
	exp = int(x.Hi>>48&0x7fff) - 16383
	if exp != -16383 {
		sig.Hi |= 1 << 48
	} else {
		shift := 113 - wideFloat(sig).len()
		sig = U128Shl(sig, uint64(shift))
		exp = -16382 - shift
	}
	return sig, exp
}

// pack128 rounds sig * 2^scale exactly once, including gradual underflow.
func pack128(negative bool, sig floatWide, scale int) F128 {
	n := sig.len()
	if n == 0 {
		return f128Signed(F128{}, negative)
	}
	exp := max(scale+n-1, -16382)
	r := sig.round(exp - 112 - scale)
	if r.Hi >= 1<<49 {
		r = U128Shr(r, 1)
		exp++
	}
	if exp > 16383 {
		return f128Signed(F128{Hi: f128Inf}, negative)
	}
	biased := uint64(0)
	if r.Hi >= 1<<48 {
		biased = uint64(exp + 16383)
	}
	return f128Signed(F128{r.Lo, r.Hi&f128Frac | biased<<48}, negative)
}

// sum128 aligns exact significands at bit 254, leaving one carry bit. Only
// distant operands lose bits, retained as sticky information. Cancellation
// of nearby operands is exact, including an overflowing product in FMA.
func sum128(a floatWide, ae int, an bool, b floatWide, be int, bn bool) F128 {
	if a == (floatWide{}) && b == (floatWide{}) {
		return f128Signed(F128{}, an && bn)
	}
	if a == (floatWide{}) {
		return pack128(bn, b, be)
	}
	if b == (floatWide{}) {
		return pack128(an, a, ae)
	}
	as, bs := 255-a.len(), 255-b.len()
	a, b = a.shl(as), b.shl(bs)
	ae, be = ae-as, be-bs
	if ae < be {
		a, b, ae, be, an, bn = b, a, be, ae, bn, an
	}
	b = b.shrJam(ae - be)
	if an == bn {
		return pack128(an, a.add(b), ae)
	}
	if a.less(b) {
		a, b, an = b, a, bn
	}
	r := a.sub(b)
	return pack128(an && r != (floatWide{}), r, ae)
}

func F128Add(a, b F128) F128 {
	if F128IsNaN(a) || F128IsNaN(b) {
		return f128NaN()
	}
	if f128IsInf(a) {
		if f128IsInf(b) && a.Hi != b.Hi {
			return f128NaN()
		}
		return a
	}
	if f128IsInf(b) {
		return b
	}
	if f128IsZero(a) && f128IsZero(b) {
		return F128{Hi: a.Hi & b.Hi}
	}
	if f128IsZero(a) {
		return b
	}
	if f128IsZero(b) {
		return a
	}
	ax, ae := unpack128(a)
	bx, be := unpack128(b)
	return sum128(wideFloat(ax), ae-112, a.Hi>>63 != 0, wideFloat(bx), be-112, b.Hi>>63 != 0)
}
func F128Sub(a, b F128) F128 { return F128Add(a, F128Neg(b)) }

func F128Mul(a, b F128) F128 {
	negative := (a.Hi^b.Hi)>>63 != 0
	if F128IsNaN(a) || F128IsNaN(b) {
		return f128NaN()
	}
	if f128IsInf(a) || f128IsInf(b) {
		if f128IsZero(a) || f128IsZero(b) {
			return f128NaN()
		}
		return f128Signed(F128{Hi: f128Inf}, negative)
	}
	if f128IsZero(a) || f128IsZero(b) {
		return f128Signed(F128{}, negative)
	}
	ax, ae := unpack128(a)
	bx, be := unpack128(b)
	return pack128(negative, floatProduct(ax, bx), ae+be-224)
}

func FMA128(a, b, c F128) F128 {
	if F128IsNaN(a) || F128IsNaN(b) || F128IsNaN(c) {
		return f128NaN()
	}
	if f128IsInf(a) || f128IsInf(b) || f128IsZero(a) || f128IsZero(b) {
		return F128Add(F128Mul(a, b), c)
	}
	if f128IsInf(c) {
		return c
	}
	if f128IsZero(c) {
		return F128Mul(a, b)
	}
	ax, ae := unpack128(a)
	bx, be := unpack128(b)
	cx, ce := unpack128(c)
	return sum128(floatProduct(ax, bx), ae+be-224, (a.Hi^b.Hi)>>63 != 0, wideFloat(cx), ce-112, c.Hi>>63 != 0)
}

func F128Div(a, b F128) F128 {
	negative := (a.Hi^b.Hi)>>63 != 0
	if F128IsNaN(a) || F128IsNaN(b) || f128IsInf(a) && f128IsInf(b) || f128IsZero(a) && f128IsZero(b) {
		return f128NaN()
	}
	if f128IsInf(a) || f128IsZero(b) {
		return f128Signed(F128{Hi: f128Inf}, negative)
	}
	if f128IsZero(a) || f128IsInf(b) {
		return f128Signed(F128{}, negative)
	}
	r, ae := unpack128(a)
	d, be := unpack128(b)
	var q U128
	// 116 quotient bits include guard/round bits for either normalization.
	for i := 0; i < 116; i++ {
		q = U128Shl(q, 1)
		if !U128Lt(r, d) {
			r = U128SubValue(r, d)
			q.Lo |= 1
		}
		r = U128Shl(r, 1)
	}
	if r != (U128{}) {
		q.Lo |= 1
	}
	return pack128(negative, wideFloat(q), ae-be-115)
}

func F128Rem(a, b F128) F128 {
	if F128IsNaN(a) || F128IsNaN(b) || f128IsInf(a) || f128IsZero(b) {
		return f128NaN()
	}
	if f128IsZero(a) || f128IsInf(b) {
		return a
	}
	r, ae := unpack128(a)
	d, be := unpack128(b)
	if ae < be {
		return a
	}
	for i := ae - be; ; i-- {
		if !U128Lt(r, d) {
			r = U128SubValue(r, d)
		}
		if i == 0 || r == (U128{}) {
			break
		}
		r = U128Shl(r, 1)
	}
	return pack128(a.Hi>>63 != 0, wideFloat(r), be-112)
}

func F128Sqrt(x F128) F128 {
	if f128IsZero(x) {
		return x
	}
	if F128IsNaN(x) || x.Hi>>63 != 0 {
		return f128NaN()
	}
	if f128IsInf(x) {
		return x
	}
	s, e := unpack128(x)
	n := wideFloat(s).shl(118 + e&1)
	var root, rem U128
	for i := 115; i >= 0; i-- {
		rem = U128Shl(rem, 2)
		rem.Lo |= n[(2*i)/64] >> uint((2*i)%64) & 3
		root = U128Shl(root, 1)
		trial := U128Shl(root, 1)
		trial.Lo |= 1
		if !U128Lt(rem, trial) {
			rem = U128SubValue(rem, trial)
			root.Lo++
		}
	}
	if rem != (U128{}) {
		root.Lo |= 1
	}
	return pack128(false, wideFloat(root), (e-(e&1))/2-115)
}

func F128Eq(a, b F128) bool {
	return !F128IsNaN(a) && !F128IsNaN(b) && (a == b || f128IsZero(a) && f128IsZero(b))
}
func F128Lt(a, b F128) bool {
	if F128IsNaN(a) || F128IsNaN(b) || f128IsZero(a) && f128IsZero(b) {
		return false
	}
	if (a.Hi^b.Hi)>>63 != 0 {
		return a.Hi>>63 != 0
	}
	if a.Hi>>63 != 0 {
		a, b = b, a
	}
	return U128Lt(U128(a), U128(b))
}
func F128Le(a, b F128) bool { return F128Lt(a, b) || F128Eq(a, b) }

// Minimum/maximum propagate NaNs and order -0 below +0. Min/max ignore one
// NaN; Rust permits either zero sign for that pair of operations.
func F128Minimum(a, b F128) F128 {
	if F128IsNaN(a) || F128IsNaN(b) {
		return f128NaN()
	}
	if F128Lt(a, b) || F128Eq(a, b) && a.Hi>>63 != 0 {
		return a
	}
	return b
}
func F128Maximum(a, b F128) F128 { return F128Neg(F128Minimum(F128Neg(a), F128Neg(b))) }
func F128Min(a, b F128) F128 {
	if F128IsNaN(a) {
		return b
	}
	if F128IsNaN(b) {
		return a
	}
	return F128Minimum(a, b)
}
func F128Max(a, b F128) F128 {
	if F128IsNaN(a) {
		return b
	}
	if F128IsNaN(b) {
		return a
	}
	return F128Maximum(a, b)
}

const (
	floatTrunc = iota
	floatFloor
	floatCeil
	floatRound
	floatRoundEven
)

func round128(x F128, mode int) F128 {
	if f128IsZero(x) || f128IsInf(x) {
		return x
	}
	if F128IsNaN(x) {
		return f128NaN()
	}
	s, e := unpack128(x)
	if e >= 112 {
		return x
	}
	negative := x.Hi>>63 != 0
	w := wideFloat(s)
	shift := 112 - e
	r := w.shr(shift)
	increase := false
	switch mode {
	case floatFloor:
		increase = negative && w.below(shift)
	case floatCeil:
		increase = !negative && w.below(shift)
	case floatRound:
		increase = e >= -1 && w[(shift-1)/64]>>uint((shift-1)%64)&1 != 0
	case floatRoundEven:
		return pack128(negative, wideFloat(w.round(shift)), 0)
	}
	if increase {
		r = r.add(floatWide{1})
	}
	return pack128(negative, r, 0)
}
func F128Trunc(x F128) F128     { return round128(x, floatTrunc) }
func F128Floor(x F128) F128     { return round128(x, floatFloor) }
func F128Ceil(x F128) F128      { return round128(x, floatCeil) }
func F128Round(x F128) F128     { return round128(x, floatRound) }
func F128RoundEven(x F128) F128 { return round128(x, floatRoundEven) }

func F128Powi(x F128, n int32) F128 {
	one := F128{Hi: 0x3fff000000000000}
	if n == 0 {
		return one
	}
	if F128IsNaN(x) {
		return f128NaN()
	}
	negative := x.Hi>>63 != 0 && n&1 != 0
	if f128IsZero(x) || f128IsInf(x) {
		if f128IsZero(x) == (n > 0) {
			return f128Signed(F128{}, negative)
		}
		return f128Signed(F128{Hi: f128Inf}, negative)
	}
	// Track the exponent separately so a negative power can produce a
	// subnormal even if the corresponding positive power would overflow.
	// Multiplication still has powi's documented unspecified precision.
	s, e := unpack128(x)
	x = F128{Lo: s.Lo, Hi: s.Hi&f128Frac | one.Hi}
	xe, re := int64(e), int64(0)
	count := uint32(n)
	if n < 0 {
		count = -count
	}
	r := one
	for count != 0 {
		if count&1 != 0 {
			r = F128Mul(r, x)
			re += xe + int64(r.Hi>>48) - 16383
			r.Hi = r.Hi&f128Frac | one.Hi
		}
		count >>= 1
		if count != 0 {
			x = F128Mul(x, x)
			xe = 2*xe + int64(x.Hi>>48) - 16383
			x.Hi = x.Hi&f128Frac | one.Hi
		}
	}
	if n < 0 {
		r = F128Div(one, r)
		re = -re
	}
	s, e = unpack128(r)
	re += int64(e)
	if re > 16383 {
		return f128Signed(F128{Hi: f128Inf}, negative)
	}
	if re < -16495 {
		return f128Signed(F128{}, negative)
	}
	return pack128(negative, wideFloat(s), int(re)-112)
}
