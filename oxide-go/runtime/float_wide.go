package oxide

import "math/bits"

// floatWide holds exact binary128 products and aligned FMA sums. The extra
// low bits carry rounding information; no operation needs heap storage.
type floatWide [4]uint64

func wideFloat(x U128) floatWide { return floatWide{x.Lo, x.Hi} }

func (x floatWide) len() int {
	for i := 3; i >= 0; i-- {
		if x[i] != 0 {
			return i*64 + bits.Len64(x[i])
		}
	}
	return 0
}

func (x floatWide) less(y floatWide) bool {
	for i := 3; i >= 0; i-- {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return false
}

func (x floatWide) add(y floatWide) (z floatWide) {
	var carry uint64
	for i := range z {
		z[i], carry = bits.Add64(x[i], y[i], carry)
	}
	return z
}

func (x floatWide) sub(y floatWide) (z floatWide) {
	var borrow uint64
	for i := range z {
		z[i], borrow = bits.Sub64(x[i], y[i], borrow)
	}
	return z
}

func (x floatWide) shl(n int) (z floatWide) {
	if n >= 256 {
		return z
	}
	w, b := n/64, uint(n%64)
	for i := w; i < 4; i++ {
		z[i] = x[i-w] << b
		if i > w && b != 0 {
			z[i] |= x[i-w-1] >> (64 - b)
		}
	}
	return z
}

func (x floatWide) shr(n int) (z floatWide) {
	if n >= 256 {
		return z
	}
	w, b := n/64, uint(n%64)
	for i := 0; i+w < 4; i++ {
		z[i] = x[i+w] >> b
		if i+w+1 < 4 && b != 0 {
			z[i] |= x[i+w+1] << (64 - b)
		}
	}
	return z
}

// below reports whether any of the lowest n bits are set.
func (x floatWide) below(n int) bool {
	n = min(n, 256)
	for i := 0; i < n/64; i++ {
		if x[i] != 0 {
			return true
		}
	}
	return n%64 != 0 && x[n/64]<<(64-uint(n%64)) != 0
}

func (x floatWide) shrJam(n int) floatWide {
	z := x.shr(n)
	if x.below(n) {
		z[0] |= 1
	}
	return z
}

// round shifts to an integer, rounding once to nearest, ties to even.
func (x floatWide) round(n int) U128 {
	if n <= 0 {
		x = x.shl(-n)
		return U128{x[0], x[1]}
	}
	z := x.shr(n)
	r := U128{z[0], z[1]}
	if n <= 256 && x[(n-1)/64]>>uint((n-1)%64)&1 != 0 && (r.Lo&1 != 0 || x.below(n-1)) {
		r = U128AddValue(r, U128{Lo: 1})
	}
	return r
}

func floatProduct(a, b U128) floatWide {
	hi, lo := bits.Mul64(a.Lo, b.Lo)
	xh, xl := bits.Mul64(a.Hi, b.Lo)
	yh, yl := bits.Mul64(a.Lo, b.Hi)
	zh, zl := bits.Mul64(a.Hi, b.Hi)
	r := floatWide{lo, hi, zl, zh}
	return r.add(floatWide{0, xl, xh}).add(floatWide{0, yl, yh})
}
