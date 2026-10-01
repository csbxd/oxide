package oxide

import (
	"math/bits"

	"github.com/shogo82148/floats"
)

// Payne-Hanek reduction. Multiplying the full 113-bit significand by a fixed
// 2/pi expansion preserves the quadrant and residual for the entire binary128
// exponent range. Reducing by a rounded floating-point pi loses the residual
// at large arguments, however wide the intermediate float happens to be.
// The constant generator certifies a residual lower bound for every exponent
// and all 113-bit significands using continued-fraction convergents.
func reduceTrig128(x F128) (uint64, floats.Float256) {
	if F128Le(x, F128{Hi: 0x3ffe000000000000}) {
		return 0, mathWide(x)
	}
	s, e := unpack128(x)
	var product [len(f128TwoOverPi) + 2]uint64
	carry := uint64(0)
	for i, v := range f128TwoOverPi {
		hi, lo := bits.Mul64(v, s.Lo)
		lo, c := bits.Add64(lo, carry, 0)
		product[i], carry = lo, hi+c
	}
	product[len(f128TwoOverPi)] = carry
	carry = 0
	for i, v := range f128TwoOverPi {
		hi, lo := bits.Mul64(v, s.Hi)
		lo, c := bits.Add64(lo, product[i+1], 0)
		hi += c
		lo, c = bits.Add64(lo, carry, 0)
		product[i+1], carry = lo, hi+c
	}
	product[len(f128TwoOverPi)+1] = carry
	shift := f128ReductionBits + 112 - e
	word, bit := shift/64, uint(shift%64)
	quadrant := product[word] >> bit
	if bit == 63 {
		quadrant |= product[word+1] << 1
	}
	quadrant &= 3
	negative := product[(shift-1)/64]>>uint((shift-1)%64)&1 != 0
	if negative {
		quadrant = (quadrant + 1) & 3
		carry = 1
		for i := 0; i <= word; i++ {
			product[i], carry = bits.Add64(^product[i], 0, carry)
		}
	}
	product[word] &= uint64(1)<<bit - 1
	for i := word + 1; i < len(product); i++ {
		product[i] = 0
	}
	for product[word] == 0 {
		word--
	}
	length := word*64 + bits.Len64(product[word])
	// Keep 237 significant bits. The constant has at least 512 guard bits
	// beyond binary128's maximum exponent before this final reduction step.
	start := length - 237
	load := func(at int) uint64 {
		if at < 0 {
			if at <= -64 {
				return 0
			}
			return product[0] << uint(-at)
		}
		i, n := at/64, uint(at%64)
		v := product[i] >> n
		if n != 0 && i+1 < len(product) {
			v |= product[i+1] << (64 - n)
		}
		return v
	}
	var m [4]uint64
	for i := range m {
		m[i] = load(start + 64*i)
	}
	// Round to nearest-even; the discarded low bits are only used here.
	if start > 0 && product[(start-1)/64]>>uint((start-1)%64)&1 != 0 {
		sticky := m[0]&1 != 0
		for i := 0; i < (start-1)/64; i++ {
			sticky = sticky || product[i] != 0
		}
		sticky = sticky || product[(start-1)/64]<<(64-uint((start-1)%64)) != 0
		if sticky {
			carry = 1
			for i := range m {
				m[i], carry = bits.Add64(m[i], 0, carry)
			}
		}
	}
	exponent := length - 1 - shift
	if m[3]>>45 != 0 {
		for i := 0; i < 3; i++ {
			m[i] = m[i]>>1 | m[i+1]<<63
		}
		m[3] >>= 1
		exponent++
	}
	m[3] = m[3]&(1<<44-1) | uint64(exponent+262143)<<44
	if negative {
		m[3] |= 1 << 63
	}
	r := floats.Float256{m[3], m[2], m[1], m[0]}
	return quadrant, r.Mul(f128PiOver2)
}

func trigPolynomial128(r floats.Float256, sine bool) floats.Float256 {
	t := r.Mul(r)
	coefficients := &f128CosCoefficients
	if sine {
		coefficients = &f128SinCoefficients
	}
	p := coefficients[len(coefficients)-1]
	for i := len(coefficients) - 2; i >= 0; i-- {
		p = floats.FMA256(p, t, coefficients[i])
	}
	if sine {
		p = p.Mul(r)
	}
	return p
}

func trig128(x F128, op string) floats.Float256 {
	if F128IsNaN(x) || f128IsInf(x) {
		return floats.NewFloat256NaN()
	}
	if f128IsZero(x) {
		if op == "cos" {
			return floats.NewFloat256(1)
		}
		return mathWide(x)
	}
	q, r := reduceTrig128(F128Abs(x))
	var value floats.Float256
	switch op {
	case "sin":
		value = trigPolynomial128(r, q&1 == 0)
		if (q >= 2) != (x.Hi>>63 != 0) {
			value = value.Neg()
		}
	case "cos":
		value = trigPolynomial128(r, q&1 != 0)
		if q == 1 || q == 2 {
			value = value.Neg()
		}
	case "tan":
		s, c := trigPolynomial128(r, true), trigPolynomial128(r, false)
		if q&1 == 0 {
			value = s.Quo(c)
		} else {
			value = c.Quo(s).Neg()
		}
		if x.Hi>>63 != 0 {
			value = value.Neg()
		}
	}
	return value
}
