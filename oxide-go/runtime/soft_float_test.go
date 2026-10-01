package oxide

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
	"unsafe"
)

// The reference uses arbitrary precision only in tests. 66000 bits cover
// even the exact sum of a maximum binary128 product and a minimum subnormal.
const floatOraclePrecision = 66000

func oracleFloat(x F128) *big.Float {
	m := new(big.Int).SetUint64(x.Hi & 0x0000ffffffffffff)
	m.Lsh(m, 64).Or(m, new(big.Int).SetUint64(x.Lo))
	e := int(x.Hi >> 48 & 0x7fff)
	if e != 0 {
		m.SetBit(m, 112, 1)
	} else {
		e = 1
	}
	f := new(big.Float).SetPrec(floatOraclePrecision).SetInt(m)
	f.SetMantExp(f, e-16383-112)
	if x.Hi>>63 != 0 {
		f.Neg(f)
	}
	return f
}

func oracleBits(f *big.Float, frac, bias int) *big.Int {
	sign := new(big.Int)
	if f.Signbit() {
		sign.SetBit(sign, frac+bitsForBias(bias), 1)
	}
	if f.Sign() == 0 {
		return sign
	}
	v := new(big.Float).SetPrec(floatOraclePrecision).Abs(f)
	exp := max(v.MantExp(nil)-1, 1-bias)
	if exp <= bias {
		v.SetMantExp(v, frac-exp)
		integer, _ := v.Int(nil)
		v.Sub(v, new(big.Float).SetPrec(floatOraclePrecision).SetInt(integer))
		cmp := v.Cmp(big.NewFloat(0.5))
		if cmp > 0 || cmp == 0 && integer.Bit(0) != 0 {
			integer.Add(integer, big.NewInt(1))
		}
		if integer.BitLen() > frac+1 {
			integer.Rsh(integer, 1)
			exp++
		}
		if exp <= bias {
			if integer.Bit(frac) != 0 {
				integer.SetBit(integer, frac, 0)
				integer.Or(integer, new(big.Int).Lsh(big.NewInt(int64(exp+bias)), uint(frac)))
			}
			return sign.Or(sign, integer)
		}
	}
	return sign.Or(sign, new(big.Int).Lsh(big.NewInt(int64(2*bias+1)), uint(frac)))
}
func bitsForBias(bias int) int {
	n := 0
	for bias != 0 {
		n++
		bias >>= 1
	}
	return n + 1
}
func oracle128(f *big.Float) F128 {
	x := oracleBits(f, 112, 16383)
	lo := x.Uint64()
	return F128{lo, x.Rsh(x, 64).Uint64()}
}

func TestF16AllPatterns(t *testing.T) {
	for i := 0; i < 65536; i++ {
		x := F16(i)
		if F16Neg(x) != x^0x8000 || F16Abs(x) != x&0x7fff || F16Copysign(x, 0x8000) != x|0x8000 {
			t.Fatalf("non-arithmetic NaN/sign bits: %04x", x)
		}
		if F16IsNaN(x) {
			continue
		}
		f := F16ToF64(x)
		if F64ToF16(f) != x || F128ToF16(F16ToF128(x)) != x || F128ToF64(F16ToF128(x)) != f {
			t.Fatalf("binary16 round trip %04x", x)
		}
		// Adjacent finite midpoints and their immediate f64 neighbours must
		// round directly, not through f32. Covers every tie and subnormal.
		if i >= 0x7bff {
			continue
		}
		mid := (f + F16ToF64(x+1)) / 2
		for _, probe := range []struct {
			value float64
			want  F16
		}{
			{math.Nextafter(mid, math.Inf(-1)), x}, {mid, x + F16(i&1)}, {math.Nextafter(mid, math.Inf(1)), x + 1},
		} {
			if got := F64ToF16(probe.value); got != probe.want {
				t.Fatalf("midpoint %04x %x: %04x want %04x", x, math.Float64bits(probe.value), got, probe.want)
			}
			if got := F128ToF16(F64ToF128(probe.value)); got != probe.want {
				t.Fatalf("f128 midpoint %04x: %04x want %04x", x, got, probe.want)
			}
		}
	}
}

func TestFMA16LLVM98389(t *testing.T) {
	if got := FMA16(0x520b, 0x00e9, 0x2ff6); got != 0x3001 {
		t.Fatalf("LLVM #98389: %04x, want 3001", got)
	}
	wrong := F32ToF16(FMA32(F16ToF32(0x520b), F16ToF32(0x00e9), F16ToF32(0x2ff6)))
	if wrong != 0x3000 {
		t.Fatalf("test no longer reproduces double rounding: %04x", wrong)
	}
	rng := rand.New(rand.NewPCG(98389, 16))
	for range 20000 {
		a, b, c := F16(rng.Uint32()), F16(rng.Uint32()), F16(rng.Uint32())
		if a&0x7c00 == 0x7c00 || b&0x7c00 == 0x7c00 || c&0x7c00 == 0x7c00 {
			continue
		}
		x, y, z := big.NewFloat(F16ToF64(a)), big.NewFloat(F16ToF64(b)), big.NewFloat(F16ToF64(c))
		sum := new(big.Float).SetPrec(128).Mul(x, y)
		sum.Add(sum, z)
		want := F16(oracleBits(sum, 10, 15).Uint64())
		if got := FMA16(a, b, c); got != want {
			t.Fatalf("fma16(%04x,%04x,%04x)=%04x want %04x", a, b, c, got, want)
		}
	}
}

func TestF128ExactArithmetic(t *testing.T) {
	rng := rand.New(rand.NewPCG(128, 98389))
	boundaries := []F128{{}, {Hi: 1 << 63}, {Lo: 1}, {Lo: 1, Hi: 1 << 63}, {Hi: 1 << 48}, {Lo: ^uint64(0), Hi: 1<<48 - 1}, {Hi: 0x3fff000000000000}, {Lo: 1, Hi: 0x3fff000000000000}, {Lo: ^uint64(0), Hi: 0x7ffeffffffffffff}, {Lo: ^uint64(0), Hi: 0xfffeffffffffffff}}
	check := func(a, b, c F128) {
		x, y, z := oracleFloat(a), oracleFloat(b), oracleFloat(c)
		ops := []struct {
			name string
			got  F128
			want *big.Float
		}{
			{"add", F128Add(a, b), new(big.Float).SetPrec(floatOraclePrecision).Add(x, y)},
			{"sub", F128Sub(a, b), new(big.Float).SetPrec(floatOraclePrecision).Sub(x, y)},
			{"mul", F128Mul(a, b), new(big.Float).SetPrec(floatOraclePrecision).Mul(x, y)},
		}
		fma := new(big.Float).SetPrec(floatOraclePrecision).Mul(x, y)
		fma.Add(fma, z)
		ops = append(ops, struct {
			name string
			got  F128
			want *big.Float
		}{"fma", FMA128(a, b, c), fma})
		if y.Sign() != 0 {
			div := new(big.Float).SetPrec(512).Quo(x, y)
			ops = append(ops, struct {
				name string
				got  F128
				want *big.Float
			}{"div", F128Div(a, b), div})
			// The integer quotient is exact at this precision over the full
			// exponent range; fmod preserves the dividend's sign at zero.
			q, _ := new(big.Float).SetPrec(floatOraclePrecision).Quo(x, y).Int(nil)
			rem := new(big.Float).SetPrec(floatOraclePrecision).SetInt(q)
			rem.Mul(rem, y).Sub(x, rem)
			if rem.Sign() == 0 && x.Signbit() {
				rem.Abs(rem).Neg(rem)
			}
			ops = append(ops, struct {
				name string
				got  F128
				want *big.Float
			}{"rem", F128Rem(a, b), rem})
		}
		if x.Sign() >= 0 {
			ops = append(ops, struct {
				name string
				got  F128
				want *big.Float
			}{"sqrt", F128Sqrt(a), new(big.Float).SetPrec(512).Sqrt(x)})
		}
		for _, op := range ops {
			if want := oracle128(op.want); op.got != want {
				t.Fatalf("%s(%016x:%016x,%016x:%016x,%016x:%016x)=%016x:%016x want %016x:%016x", op.name, a.Hi, a.Lo, b.Hi, b.Lo, c.Hi, c.Lo, op.got.Hi, op.got.Lo, want.Hi, want.Lo)
			}
		}
		if F128Eq(a, b) != (x.Cmp(y) == 0) || F128Lt(a, b) != (x.Cmp(y) < 0) || F128Le(a, b) != (x.Cmp(y) <= 0) {
			t.Fatal("comparison", a, b)
		}
	}
	for _, a := range boundaries {
		for _, b := range boundaries {
			for _, c := range boundaries {
				check(a, b, c)
			}
		}
	}
	for range 3000 {
		next := func() F128 {
			x := F128{rng.Uint64(), rng.Uint64()}
			if x.Hi&f128Inf == f128Inf {
				x.Hi ^= 1 << 48
			}
			return x
		}
		check(next(), next(), next())
	}
	// Exact halfway products with an addend too small for even a 256-bit
	// sum force sticky bits to choose the correct side of the tie.
	for _, sign := range []uint64{0, 1 << 63} {
		for _, tiny := range []F128{{Lo: 1}, {Lo: 1, Hi: 1 << 63}} {
			check(F128{Lo: 1, Hi: 0x3fff000000000000 | sign}, F128{Hi: 0x3fff800000000000}, tiny)
		}
	}
}

func TestSoftFloatSpecialsAndAllocations(t *testing.T) {
	if unsafe.Sizeof(F16(0)) != 2 || unsafe.Alignof(F16(0)) != 2 || unsafe.Sizeof(F128{}) != 16 {
		t.Fatal("float storage layout")
	}
	for _, a := range []float64{0, math.Copysign(0, -1), 1, -1, math.Inf(1), math.Inf(-1), math.NaN()} {
		for _, b := range []float64{0, math.Copysign(0, -1), 1, -1, math.Inf(1), math.Inf(-1), math.NaN()} {
			x, y := F64ToF128(a), F64ToF128(b)
			for _, op := range []struct {
				got  F128
				want float64
			}{{F128Add(x, y), a + b}, {F128Mul(x, y), a * b}, {F128Div(x, y), a / b}, {F128Rem(x, y), math.Mod(a, b)}} {
				if math.IsNaN(op.want) {
					if !F128IsNaN(op.got) {
						t.Fatal("nonfinite", a, b, op.got)
					}
				} else if op.got != F64ToF128(op.want) {
					t.Fatal("signed zero / infinity", a, b, op)
				}
			}
			if F128Eq(x, y) != (a == b) || F128Lt(x, y) != (a < b) || F128Le(x, y) != (a <= b) {
				t.Fatal("unordered comparison")
			}
		}
	}
	a, b, c := F128{Lo: 1, Hi: 0x3fff000000000000}, F128{Hi: 0x3fff800000000000}, F128{Lo: 1}
	requireNoGoAllocations(t, 1000, func() {
		softFloatSink = F128Sqrt(F128Add(FMA128(a, b, c), F128Rem(F128Div(a, b), c)))
		softFloatSink = F128RoundEven(softFloatSink)
		softFloat16Sink = FMA16(0x520b, 0x00e9, 0x2ff6)
	})
}

var softFloatSink F128
var softFloat16Sink F16

func TestF128Conversions(t *testing.T) {
	rng := rand.New(rand.NewPCG(64, 128))
	values := []F128{{}, {Hi: 1 << 63}}
	for _, e := range []uint64{0, 1, 16382, 16383, 16435, 16446, 16447, 16494, 16495, 16509, 16510, 16511, 32766} {
		for _, frac := range []U128{{}, {Lo: 1}, {Hi: 1 << 47}, {Lo: ^uint64(0), Hi: 1<<48 - 1}} {
			for _, sign := range []uint64{0, 1 << 63} {
				values = append(values, F128{frac.Lo, sign | e<<48 | frac.Hi})
			}
		}
	}
	for range 4000 {
		x := F128{rng.Uint64(), rng.Uint64()}
		if x.Hi&f128Inf != f128Inf {
			values = append(values, x)
		}
	}
	for _, x := range values {
		f := oracleFloat(x)
		for _, tc := range []struct {
			frac, bias int
			got        uint64
		}{{10, 15, uint64(F128ToF16(x))}, {23, 127, uint64(math.Float32bits(F128ToF32(x)))}, {52, 1023, math.Float64bits(F128ToF64(x))}} {
			want := oracleBits(f, tc.frac, tc.bias).Uint64()
			if tc.got != want {
				t.Fatalf("narrow %v / %d: %x want %x", x, tc.frac, tc.got, want)
			}
		}
		n, _ := f.Int(nil)
		for _, signed := range []bool{false, true} {
			lo, hi := new(big.Int), new(big.Int)
			hi.Lsh(big.NewInt(1), 128)
			if signed {
				hi.Rsh(hi, 1)
				lo.Neg(hi)
			}
			hi.Sub(hi, big.NewInt(1))
			want := new(big.Int).Set(n)
			if want.Cmp(lo) < 0 {
				want.Set(lo)
			}
			if want.Cmp(hi) > 0 {
				want.Set(hi)
			}
			got := F128ToU128(x)
			if signed {
				got = U128(F128ToI128(x))
			}
			if got != bigTo128(want) {
				t.Fatalf("cast %v signed=%v: %v want %v", x, signed, got, want)
			}
		}
	}
	for range 10000 {
		x := U128{rng.Uint64(), rng.Uint64()}
		for _, signed := range []bool{false, true} {
			n := bigUnsigned128(x)
			got := U128ToF128(x)
			if signed {
				n = bigSigned128(I128(x))
				got = I128ToF128(I128(x))
			}
			if want := oracle128(new(big.Float).SetPrec(128).SetInt(n)); got != want {
				t.Fatalf("integer rounding %v: %v want %v", n, got, want)
			}
		}
	}
	for _, x := range []F128{{Hi: f128Inf}, {Hi: f128Inf | f128Sign}, {Lo: 1, Hi: f128Inf}} {
		f := math.Inf(1)
		if x.Hi>>63 != 0 {
			f = math.Inf(-1)
		}
		if F128IsNaN(x) {
			f = math.NaN()
		}
		if F128ToI128(x) != FloatToI128(f) || F128ToU128(x) != FloatToU128(f) {
			t.Fatal("nonfinite cast", x)
		}
	}
}

func TestF128IntegralRounding(t *testing.T) {
	for _, value := range []float64{0, math.Copysign(0, -1), 0.5, -0.5, 0.25, -0.25, 1.5, -1.5, 2.5, -2.5, 3.75, -3.75, math.Inf(1), math.Inf(-1)} {
		x := F64ToF128(value)
		for _, tc := range []struct {
			got  F128
			want float64
		}{{F128Trunc(x), math.Trunc(value)}, {F128Floor(x), math.Floor(value)}, {F128Ceil(x), math.Ceil(value)}, {F128Round(x), math.Round(value)}, {F128RoundEven(x), math.RoundToEven(value)}} {
			if tc.got != F64ToF128(tc.want) {
				t.Fatalf("integral rounding %v: %v want %v", value, tc.got, tc.want)
			}
		}
	}
	// Low binary128 fraction bits must not disappear through f64 rounding.
	for _, sign := range []bool{false, true} {
		x := F128{Lo: 1, Hi: 0x4063000000000000} // 2^100 + 2^-12
		base := F128{Hi: x.Hi}
		next := F128{Lo: 1 << 12, Hi: x.Hi}
		if sign {
			x = F128Neg(x)
			base = F128Neg(base)
			next = F128Neg(next)
		}
		floor, ceil := base, next
		if sign {
			floor, ceil = next, base
		}
		if F128Trunc(x) != base || F128Round(x) != base || F128RoundEven(x) != base || F128Floor(x) != floor || F128Ceil(x) != ceil {
			t.Fatal("wide integral precision", x)
		}
	}
}

func TestF128PowiRange(t *testing.T) {
	two := F128{Hi: 0x4000000000000000}
	for _, n := range []int32{-16496, -16495, -16494, -16384, -16383, -16382, -1, 0, 1, 16383, 16384} {
		want := oracle128(new(big.Float).SetPrec(128).SetMantExp(big.NewFloat(1), int(n)))
		if got := F128Powi(two, n); got != want {
			t.Fatalf("2^%d=%v want %v", n, got, want)
		}
	}
	if F128Powi(two, -1<<31) != (F128{}) || F128Powi(F128Neg(two), -16493) != (F128{Lo: 2, Hi: 1 << 63}) {
		t.Fatal("power exponent/sign")
	}
}

func BenchmarkSoftFloat(b *testing.B) {
	x, y, z := F128{Lo: 1, Hi: 0x3fff000000000000}, F128{Lo: 3, Hi: 0x3fff800000000000}, F128{Lo: 1}
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"FMA16", func() { softFloat16Sink = FMA16(0x520b, 0x00e9, 0x2ff6) }},
		{"Add128", func() { softFloatSink = F128Add(x, y) }},
		{"Mul128", func() { softFloatSink = F128Mul(x, y) }},
		{"Div128", func() { softFloatSink = F128Div(x, y) }},
		{"FMA128", func() { softFloatSink = FMA128(x, y, z) }},
		{"Sqrt128", func() { softFloatSink = F128Sqrt(y) }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tc.run()
			}
		})
	}
}
