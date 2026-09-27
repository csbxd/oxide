package oxide

import (
	"math"
	"math/bits"
)

// U128 and I128 are the exact 16-byte Rust integer representations on the
// supported targets. They deliberately expose words instead of using big.Int:
// MIR arithmetic must not allocate.
type U128 struct{ Lo, Hi uint64 }
type I128 struct{ Lo, Hi uint64 }

func U128Add(a, b U128) (U128, bool) {
	lo, c := bits.Add64(a.Lo, b.Lo, 0)
	hi, c2 := bits.Add64(a.Hi, b.Hi, c)
	return U128{lo, hi}, c2 != 0
}
func U128Sub(a, b U128) (U128, bool) {
	lo, c := bits.Sub64(a.Lo, b.Lo, 0)
	hi, c2 := bits.Sub64(a.Hi, b.Hi, c)
	return U128{lo, hi}, c2 != 0
}
func U128From64(v uint64) U128 { return U128{Lo: v} }
func U128To64(v U128) uint64   { return v.Lo }
func I128From64(v int64) I128  { return I128{Lo: uint64(v), Hi: uint64(v >> 63)} }
func I128To64(v I128) int64    { return int64(v.Lo) }
func I128Add(a, b I128) (I128, bool) {
	r := I128{Lo: a.Lo + b.Lo}
	carry := uint64(0)
	if r.Lo < a.Lo {
		carry = 1
	}
	r.Hi = a.Hi + b.Hi + carry
	return r, ((a.Hi^r.Hi)&(b.Hi^r.Hi))>>63 != 0
}
func I128Sub(a, b I128) (I128, bool) {
	r := I128{Lo: a.Lo - b.Lo}
	borrow := uint64(0)
	if a.Lo < b.Lo {
		borrow = 1
	}
	r.Hi = a.Hi - b.Hi - borrow
	return r, ((a.Hi^b.Hi)&(a.Hi^r.Hi))>>63 != 0
}
func U128AddValue(a, b U128) U128 { r, _ := U128Add(a, b); return r }
func U128SubValue(a, b U128) U128 { r, _ := U128Sub(a, b); return r }
func U128MulValue(a, b U128) U128 {
	carry, lo := bits.Mul64(a.Lo, b.Lo)
	_, x := bits.Mul64(a.Hi, b.Lo)
	_, y := bits.Mul64(a.Lo, b.Hi)
	return U128{Lo: lo, Hi: carry + x + y}
}
func U128Mul(a, b U128) (U128, bool) {
	carry, lo := bits.Mul64(a.Lo, b.Lo)
	xhi, xlo := bits.Mul64(a.Hi, b.Lo)
	yhi, ylo := bits.Mul64(a.Lo, b.Hi)
	hi, c1 := bits.Add64(carry, xlo, 0)
	hi, c2 := bits.Add64(hi, ylo, 0)
	overflow := a.Hi != 0 && b.Hi != 0 || xhi != 0 || yhi != 0 || c1 != 0 || c2 != 0
	return U128{Lo: lo, Hi: hi}, overflow
}
func I128Mul(a, b I128) (I128, bool) {
	x, y := U128(a), U128(b)
	aNegative, bNegative := a.Hi>>63 != 0, b.Hi>>63 != 0
	if aNegative {
		x = U128SubValue(U128{}, x)
	}
	if bNegative {
		y = U128SubValue(U128{}, y)
	}
	r, overflow := U128Mul(x, y)
	if aNegative != bNegative {
		overflow = overflow || r.Hi > 1<<63 || r.Hi == 1<<63 && r.Lo != 0
		r = U128SubValue(U128{}, r)
	} else {
		overflow = overflow || r.Hi>>63 != 0
	}
	return I128(r), overflow
}
func I128MulValue(a, b I128) I128 { return I128(U128MulValue(U128(a), U128(b))) }
func I128AddValue(a, b I128) I128 { r, _ := I128Add(a, b); return r }
func I128SubValue(a, b I128) I128 { r, _ := I128Sub(a, b); return r }
func U128Shl(a U128, n uint64) U128 {
	// MIR emits overflow assertions separately. With overflow checks disabled,
	// Rust masks the shift count, just like the scalar integer backend.
	n &= 127
	if n >= 64 {
		return U128{Lo: 0, Hi: a.Lo << (n - 64)}
	}
	return U128{Lo: a.Lo << n, Hi: a.Hi<<n | a.Lo>>(64-n)}
}
func U128Shr(a U128, n uint64) U128 {
	n &= 127
	if n >= 64 {
		return U128{Lo: a.Hi >> (n - 64)}
	}
	return U128{Lo: a.Lo>>n | a.Hi<<(64-n), Hi: a.Hi >> n}
}
func I128Shl(a I128, n uint64) I128 { u := U128Shl(U128(a), n); return I128(u) }
func I128Shr(a I128, n uint64) I128 {
	n &= 127
	if n >= 64 {
		fill := uint64(0)
		if a.Hi>>63 != 0 {
			fill = ^uint64(0)
		}
		return I128{Lo: uint64(int64(a.Hi) >> (n - 64)), Hi: fill}
	}
	return I128{Lo: a.Lo>>n | a.Hi<<(64-n), Hi: uint64(int64(a.Hi) >> n)}
}
func U128And(a, b U128) U128 { return U128{a.Lo & b.Lo, a.Hi & b.Hi} }
func U128Or(a, b U128) U128  { return U128{a.Lo | b.Lo, a.Hi | b.Hi} }
func U128Xor(a, b U128) U128 { return U128{a.Lo ^ b.Lo, a.Hi ^ b.Hi} }
func U128ReverseBits(a U128) U128 {
	return U128{Lo: bits.Reverse64(a.Hi), Hi: bits.Reverse64(a.Lo)}
}
func I128ReverseBits(a I128) I128 { return I128(U128ReverseBits(U128(a))) }
func U128ByteSwap(a U128) U128 {
	return U128{Lo: bits.ReverseBytes64(a.Hi), Hi: bits.ReverseBytes64(a.Lo)}
}
func I128ByteSwap(a I128) I128 { return I128(U128ByteSwap(U128(a))) }
func U128Eq(a, b U128) bool    { return a == b }
func U128Lt(a, b U128) bool    { return a.Hi < b.Hi || (a.Hi == b.Hi && a.Lo < b.Lo) }
func U128Le(a, b U128) bool    { return a == b || U128Lt(a, b) }
func I128Eq(a, b I128) bool    { return a == b }
func I128Lt(a, b I128) bool {
	sa, sb := a.Hi>>63, b.Hi>>63
	if sa != sb {
		return sa != 0
	}
	return a.Hi < b.Hi || (a.Hi == b.Hi && a.Lo < b.Lo)
}
func I128Le(a, b I128) bool { return a == b || I128Lt(a, b) }

func U128DivRem(a, b U128) (U128, U128) {
	if b.Hi == 0 {
		// Div64 requires its high input to be below the divisor.
		qhi, rem := a.Hi/b.Lo, a.Hi%b.Lo
		qlo, rem := bits.Div64(rem, a.Lo, b.Lo)
		return U128{qlo, qhi}, U128{Lo: rem}
	}
	if U128Lt(a, b) {
		return U128{}, a
	}
	// With a two-word divisor the quotient fits in one word. Align the
	// divisor with the dividend; at most 64 subtract/shift steps remain.
	n := bits.LeadingZeros64(b.Hi) - bits.LeadingZeros64(a.Hi)
	d, q := U128Shl(b, uint64(n)), uint64(0)
	for ; n >= 0; n-- {
		if U128Le(d, a) {
			a = U128SubValue(a, d)
			q |= uint64(1) << n
		}
		d = U128Shr(d, 1)
	}
	return U128{Lo: q}, a
}

func U128Div(a, b U128) U128 { q, _ := U128DivRem(a, b); return q }
func U128Rem(a, b U128) U128 { _, r := U128DivRem(a, b); return r }
func I128DivRem(a, b I128) (I128, I128) {
	x, y := U128(a), U128(b)
	negativeA, negativeB := a.Hi>>63 != 0, b.Hi>>63 != 0
	if negativeA {
		x = U128SubValue(U128{}, x)
	}
	if negativeB {
		y = U128SubValue(U128{}, y)
	}
	q, r := U128DivRem(x, y)
	if negativeA != negativeB {
		q = U128SubValue(U128{}, q)
	}
	if negativeA {
		r = U128SubValue(U128{}, r)
	}
	// MIR checks MIN/-1 before both division and remainder, as it does for
	// scalar arithmetic. The helper itself uses wrapping word operations.
	return I128(q), I128(r)
}
func I128Div(a, b I128) I128 { q, _ := I128DivRem(a, b); return q }
func I128Rem(a, b I128) I128 { _, r := I128DivRem(a, b); return r }

// U128CarryingMulAdd returns the low and high halves of the full 256-bit
// a*b+c+d result. This sum cannot overflow 256 bits.
func U128CarryingMulAdd(a, b, c, d U128) (U128, U128) {
	p01, p00 := bits.Mul64(a.Lo, b.Lo)
	p11, p10 := bits.Mul64(a.Hi, b.Lo)
	p21, p20 := bits.Mul64(a.Lo, b.Hi)
	p31, p30 := bits.Mul64(a.Hi, b.Hi)
	w1, carry := bits.Add64(p01, p10, 0)
	w2, w3 := bits.Add64(p11, p21, carry)
	w1, carry = bits.Add64(w1, p20, 0)
	w2, carry = bits.Add64(w2, p30, carry)
	w3 += p31 + carry
	lo, hi := U128{p00, w1}, U128{w2, w3}
	for _, v := range [2]U128{c, d} {
		lo.Lo, carry = bits.Add64(lo.Lo, v.Lo, 0)
		lo.Hi, carry = bits.Add64(lo.Hi, v.Hi, carry)
		hi.Lo, carry = bits.Add64(hi.Lo, 0, carry)
		hi.Hi += carry
	}
	return lo, hi
}

func U64CarryingMulAdd(a, b, c, d uint64) (uint64, uint64) {
	hi, lo := bits.Mul64(a, b)
	lo, carry := bits.Add64(lo, c, 0)
	hi += carry
	lo, carry = bits.Add64(lo, d, 0)
	return lo, hi + carry
}
func I64CarryingMulAdd(a, b, c, d int64) (uint64, int64) {
	lo, hi := U64CarryingMulAdd(uint64(a), uint64(b), uint64(c), uint64(d))
	if a < 0 {
		hi -= uint64(b)
	}
	if b < 0 {
		hi -= uint64(a)
	}
	return lo, int64(hi) + (c >> 63) + (d >> 63)
}

func I128CarryingMulAdd(a, b, c, d I128) (U128, I128) {
	lo, hi := U128CarryingMulAdd(U128(a), U128(b), U128(c), U128(d))
	if a.Hi>>63 != 0 {
		hi = U128SubValue(hi, U128(b))
	}
	if b.Hi>>63 != 0 {
		hi = U128SubValue(hi, U128(a))
	}
	if c.Hi>>63 != 0 {
		hi = U128SubValue(hi, U128{Lo: 1})
	}
	if d.Hi>>63 != 0 {
		hi = U128SubValue(hi, U128{Lo: 1})
	}
	return lo, I128(hi)
}

// Rust float-to-integer casts saturate and map NaN to zero. Decomposing the
// IEEE significand avoids implementation-dependent out-of-range Go casts.
func FloatToU128(x float64) U128 {
	if !(x > 0) {
		return U128{}
	}
	if x >= 0x1p128 {
		return U128{^uint64(0), ^uint64(0)}
	}
	b := math.Float64bits(x)
	e := int((b>>52)&2047) - 1023
	if e < 0 {
		return U128{}
	}
	m := (b & ((1 << 52) - 1)) | (1 << 52)
	if e < 52 {
		return U128{Lo: m >> (52 - e)}
	}
	return U128Shl(U128{Lo: m}, uint64(e-52))
}

func FloatToI128(x float64) I128 {
	if x >= 0x1p127 {
		return I128{^uint64(0), (1 << 63) - 1}
	}
	if x <= -0x1p127 {
		return I128{Hi: 1 << 63}
	}
	if x < 0 {
		return I128(U128SubValue(U128{}, FloatToU128(-x)))
	}
	return I128(FloatToU128(x))
}

// Round once to the destination precision, ties to even. Converting the two
// words separately or routing f32 through f64 can introduce double rounding.
func u128FloatBits(x U128, fraction, bias int) uint64 {
	if x == (U128{}) {
		return 0
	}
	e := 127 - bits.LeadingZeros64(x.Hi)
	if x.Hi == 0 {
		e = 63 - bits.LeadingZeros64(x.Lo)
	}
	var m uint64
	if e <= fraction {
		m = x.Lo << (fraction - e)
	} else {
		shift := e - fraction
		m = U128Shr(x, uint64(shift)).Lo
		remainder := U128SubValue(x, U128Shl(U128{Lo: m}, uint64(shift)))
		half := U128Shl(U128{Lo: 1}, uint64(shift-1))
		if U128Lt(half, remainder) || remainder == half && m&1 != 0 {
			m++
		}
	}
	// A rounded significand of 2^(fraction+1) carries into the exponent.
	return uint64(e+bias-1)<<fraction + m
}
func U128ToF64(x U128) float64 { return math.Float64frombits(u128FloatBits(x, 52, 1023)) }
func U128ToF32(x U128) float32 { return math.Float32frombits(uint32(u128FloatBits(x, 23, 127))) }
func I128ToF64(x I128) float64 {
	if x.Hi>>63 != 0 {
		return -U128ToF64(U128SubValue(U128{}, U128(x)))
	}
	return U128ToF64(U128(x))
}
func I128ToF32(x I128) float32 {
	if x.Hi>>63 != 0 {
		return -U128ToF32(U128SubValue(U128{}, U128(x)))
	}
	return U128ToF32(U128(x))
}
