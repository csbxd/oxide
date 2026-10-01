package oxide

import (
	"math"
	"unsafe"
)

// Small formats are rounded directly from the source significand. Passing
// through a narrower intermediate would double-round casts near half-way.
func packFloat(negative bool, sig floatWide, scale, frac, bias int) uint64 {
	sign := uint64(0)
	if negative {
		sign = uint64(2*bias+2) << frac
	}
	n := sig.len()
	if n == 0 {
		return sign
	}
	exp := max(scale+n-1, 1-bias)
	r := sig.round(exp - frac - scale).Lo
	if r >= uint64(2)<<frac {
		r >>= 1
		exp++
	}
	if exp > bias {
		return sign | uint64(2*bias+1)<<frac
	}
	biased := uint64(0)
	if r >= uint64(1)<<frac {
		biased = uint64(exp + bias)
	}
	return sign | biased<<frac | r&(uint64(1)<<frac-1)
}

func extendFloat(x uint64, frac, bias int) F128 {
	negative := x>>frac&uint64(2*bias+2) != 0
	e := int(x>>frac) & (2*bias + 1)
	s := x & (uint64(1)<<frac - 1)
	if e == 2*bias+1 {
		r := F128{Hi: f128Inf}
		if s != 0 {
			payload := U128Shl(U128{Lo: s}, uint64(112-frac))
			r.Lo, r.Hi = payload.Lo, r.Hi|payload.Hi|1<<47
		}
		return f128Signed(r, negative)
	}
	if e != 0 {
		s |= uint64(1) << frac
	} else {
		e = 1
	}
	return pack128(negative, floatWide{s}, e-bias-frac)
}

func narrowFloat(x F128, frac, bias int) uint64 {
	negative := x.Hi>>63 != 0
	if f128IsInf(x) || F128IsNaN(x) {
		r := uint64(2*bias+1) << frac
		if negative {
			r |= uint64(2*bias+2) << frac
		}
		if F128IsNaN(x) {
			payload := U128Shr(U128{Lo: x.Lo, Hi: x.Hi & f128Frac}, uint64(112-frac))
			r |= payload.Lo | uint64(1)<<(frac-1)
		}
		return r
	}
	if f128IsZero(x) {
		return packFloat(negative, floatWide{}, 0, frac, bias)
	}
	s, e := unpack128(x)
	return packFloat(negative, wideFloat(s), e-112, frac, bias)
}

func F32ToF128(x float32) F128 { return extendFloat(uint64(math.Float32bits(x)), 23, 127) }
func F64ToF128(x float64) F128 { return extendFloat(math.Float64bits(x), 52, 1023) }
func F16ToF128(x F16) F128     { return extendFloat(uint64(x), 10, 15) }
func F128ToF32(x F128) float32 { return math.Float32frombits(uint32(narrowFloat(x, 23, 127))) }
func F128ToF64(x F128) float64 { return math.Float64frombits(narrowFloat(x, 52, 1023)) }
func F128ToF16(x F128) F16     { return F16(narrowFloat(x, 10, 15)) }

func U128ToF128(x U128) F128 { return pack128(false, wideFloat(x), 0) }
func I128ToF128(x I128) F128 {
	negative := x.Hi>>63 != 0
	s := U128(x)
	if negative {
		s = U128SubValue(U128{}, s)
	}
	return pack128(negative, wideFloat(s), 0)
}
func IntToF128[T Integer](x T) F128 {
	if x < 0 {
		return I128ToF128(I128From64(int64(x)))
	}
	return U128ToF128(U128{Lo: uint64(x)})
}

func F128ToU128(x F128) U128 {
	if F128IsNaN(x) || x.Hi>>63 != 0 || f128IsZero(x) {
		return U128{}
	}
	s, e := unpack128(x)
	if e >= 128 {
		return U128{Lo: ^uint64(0), Hi: ^uint64(0)}
	}
	if e < 0 {
		return U128{}
	}
	if e >= 112 {
		return U128Shl(s, uint64(e-112))
	}
	return U128Shr(s, uint64(112-e))
}
func F128ToI128(x F128) I128 {
	if F128IsNaN(x) || f128IsZero(x) {
		return I128{}
	}
	negative := x.Hi>>63 != 0
	_, e := unpack128(x)
	if e >= 127 {
		if negative {
			return I128{Hi: 1 << 63}
		}
		return I128{Lo: ^uint64(0), Hi: 1<<63 - 1}
	}
	r := F128ToU128(F128Abs(x))
	if negative {
		r = U128SubValue(U128{}, r)
	}
	return I128(r)
}
func F128ToInt[T Integer](x F128) T {
	if F128IsNaN(x) {
		return 0
	}
	width := uint(unsafe.Sizeof(T(0)) * 8)
	if ^T(0) > 0 {
		r := F128ToU128(x)
		if r.Hi != 0 || r.Lo > uint64(^T(0)) {
			return ^T(0)
		}
		return T(r.Lo)
	}
	r := F128ToI128(x)
	if I128Lt(r, I128From64(-1<<(width-1))) {
		return minValue[T]()
	}
	if I128Lt(I128From64(1<<(width-1)-1), r) {
		return ^minValue[T]()
	}
	return T(r.Lo)
}
