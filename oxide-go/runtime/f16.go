package oxide

import (
	"math"
	"math/bits"
)

// F16 is an IEEE binary16 bit pattern, with Rust's size and alignment.
type F16 uint16

func F16ToF64(x F16) float64 {
	sign := uint64(x&0x8000) << 48
	e, s := int(x>>10&31), uint64(x&1023)
	if e == 31 {
		return math.Float64frombits(sign | 0x7ff0000000000000 | s<<42)
	}
	if e == 0 {
		if s == 0 {
			return math.Float64frombits(sign)
		}
		shift := 11 - bits.Len64(s)
		s <<= shift
		e = 1 - shift
	}
	return math.Float64frombits(sign | uint64(e-15+1023)<<52 | (s&1023)<<42)
}
func F16ToF32(x F16) float32 { return float32(F16ToF64(x)) }

func F64ToF16(x float64) F16 {
	b := math.Float64bits(x)
	e, s := int(b>>52&2047), b&(1<<52-1)
	if e == 2047 {
		r := F16(b>>48)&0x8000 | 0x7c00
		if s != 0 {
			r |= F16(s>>42) | 0x200
		}
		return r
	}
	if e != 0 {
		s |= 1 << 52
	} else {
		e = 1
	}
	return F16(packFloat(b>>63 != 0, floatWide{s}, e-1023-52, 10, 15))
}
func F32ToF16(x float32) F16 { return F64ToF16(float64(x)) }
func U128ToF16(x U128) F16   { return F16(packFloat(false, wideFloat(x), 0, 10, 15)) }
func I128ToF16(x I128) F16 {
	negative := x.Hi>>63 != 0
	s := U128(x)
	if negative {
		s = U128SubValue(U128{}, s)
	}
	return F16(packFloat(negative, wideFloat(s), 0, 10, 15))
}
func IntToF16[T Integer](x T) F16 { return F64ToF16(float64(x)) }
func F16ToU128(x F16) U128        { return FloatToU128(F16ToF64(x)) }
func F16ToI128(x F16) I128        { return FloatToI128(F16ToF64(x)) }
func F16ToInt[T Integer](x F16) T { return FloatToInt[T](F16ToF64(x)) }

func F16Neg(x F16) F16         { return x ^ 0x8000 }
func F16Abs(x F16) F16         { return x & 0x7fff }
func F16Copysign(x, y F16) F16 { return x&0x7fff | y&0x8000 }
func F16IsNaN(x F16) bool      { return x&0x7fff > 0x7c00 }
func F16Add(x, y F16) F16      { return F64ToF16(float64(F16ToF64(x) + F16ToF64(y))) }
func F16Sub(x, y F16) F16      { return F64ToF16(float64(F16ToF64(x) - F16ToF64(y))) }
func F16Mul(x, y F16) F16      { return F64ToF16(float64(F16ToF64(x) * F16ToF64(y))) }
func F16Div(x, y F16) F16      { return F64ToF16(float64(F16ToF64(x) / F16ToF64(y))) }
func F16Rem(x, y F16) F16      { return F64ToF16(math.Mod(F16ToF64(x), F16ToF64(y))) }

// LLVM #98389: f32 FMA followed by f16 truncation double-rounds. Binary64
// has enough precision for binary16 products and their final rounding;
// conversion here goes directly from binary64 to binary16, never via f32.
func FMA16(x, y, z F16) F16  { return F64ToF16(math.FMA(F16ToF64(x), F16ToF64(y), F16ToF64(z))) }
func F16Eq(x, y F16) bool    { return F16ToF64(x) == F16ToF64(y) }
func F16Lt(x, y F16) bool    { return F16ToF64(x) < F16ToF64(y) }
func F16Le(x, y F16) bool    { return F16ToF64(x) <= F16ToF64(y) }
func F16Sqrt(x F16) F16      { return F64ToF16(math.Sqrt(F16ToF64(x))) }
func F16Trunc(x F16) F16     { return F64ToF16(math.Trunc(F16ToF64(x))) }
func F16Floor(x F16) F16     { return F64ToF16(math.Floor(F16ToF64(x))) }
func F16Ceil(x F16) F16      { return F64ToF16(math.Ceil(F16ToF64(x))) }
func F16Round(x F16) F16     { return F64ToF16(math.Round(F16ToF64(x))) }
func F16RoundEven(x F16) F16 { return F64ToF16(math.RoundToEven(F16ToF64(x))) }
func F16Minimum(x, y F16) F16 {
	if F16IsNaN(x) || F16IsNaN(y) {
		return 0x7e00
	}
	return F64ToF16(math.Min(F16ToF64(x), F16ToF64(y)))
}
func F16Maximum(x, y F16) F16 {
	if F16IsNaN(x) || F16IsNaN(y) {
		return 0x7e00
	}
	return F64ToF16(math.Max(F16ToF64(x), F16ToF64(y)))
}
func F16Min(x, y F16) F16 {
	if F16IsNaN(x) {
		return y
	}
	if F16IsNaN(y) {
		return x
	}
	return F16Minimum(x, y)
}
func F16Max(x, y F16) F16 {
	if F16IsNaN(x) {
		return y
	}
	if F16IsNaN(y) {
		return x
	}
	return F16Maximum(x, y)
}
func F16Powi(x F16, n int32) F16 {
	// Keep intermediate products and the reciprocal wider than binary16, so
	// a representable negative power survives intermediate binary16 overflow.
	// Like Rust's powi, this operation has unspecified precision.
	a := F16ToF32(x)
	count := uint32(n)
	if n < 0 {
		count = -count
	}
	r := float32(1)
	for count != 0 {
		if count&1 != 0 {
			r = float32(r * a)
		}
		count >>= 1
		if count != 0 {
			a = float32(a * a)
		}
	}
	if n < 0 {
		r = float32(1 / r)
	}
	return F32ToF16(r)
}

// Rust's transcendental functions have unspecified precision. Their f16
// inputs are exact in f64; round the wider result directly to binary16.
func F16Exp(x F16) F16    { return F64ToF16(math.Exp(F16ToF64(x))) }
func F16Exp2(x F16) F16   { return F64ToF16(math.Exp2(F16ToF64(x))) }
func F16Log(x F16) F16    { return F64ToF16(math.Log(F16ToF64(x))) }
func F16Log2(x F16) F16   { return F64ToF16(math.Log2(F16ToF64(x))) }
func F16Log10(x F16) F16  { return F64ToF16(math.Log10(F16ToF64(x))) }
func F16Sin(x F16) F16    { return F64ToF16(math.Sin(F16ToF64(x))) }
func F16Cos(x F16) F16    { return F64ToF16(math.Cos(F16ToF64(x))) }
func F16Pow(x, y F16) F16 { return F64ToF16(math.Pow(F16ToF64(x), F16ToF64(y))) }
