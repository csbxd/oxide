package oxide

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func Test128Division(t *testing.T) {
	for _, a := range integer128Cases() {
		for _, b := range integer128Cases()[1:] {
			wantQ, wantR := new(big.Int), new(big.Int)
			wantQ.QuoRem(bigUnsigned128(a), bigUnsigned128(b), wantR)
			q, r := U128DivRem(a, b)
			if q != bigTo128(wantQ) || r != bigTo128(wantR) {
				t.Fatalf("unsigned %v / %v: %v %v, want %v %v", a, b, q, r, wantQ, wantR)
			}
			wantQ.QuoRem(bigSigned128(I128(a)), bigSigned128(I128(b)), wantR)
			iq, ir := I128DivRem(I128(a), I128(b))
			if U128(iq) != bigTo128(wantQ) || U128(ir) != bigTo128(wantR) {
				t.Fatalf("signed %v / %v: %v %v, want %v %v", a, b, iq, ir, wantQ, wantR)
			}
		}
	}
}

func Test128CarryingMulAdd(t *testing.T) {
	values := integer128Cases()
	rng := rand.New(rand.NewPCG(871, 623))
	for _, a := range values {
		for _, b := range values {
			for _, c := range []U128{values[len(values)-33], {rng.Uint64(), rng.Uint64()}} {
				d := U128{rng.Uint64(), rng.Uint64()}
				for _, signed := range []bool{false, true} {
					asBig := bigUnsigned128
					if signed {
						asBig = func(x U128) *big.Int { return bigSigned128(I128(x)) }
					}
					want := new(big.Int).Mul(asBig(a), asBig(b))
					want.Add(want, asBig(c)).Add(want, asBig(d))
					wantLo := bigTo128(want)
					wantHi := bigTo128(new(big.Int).Rsh(want, 128))
					lo, hi := U128CarryingMulAdd(a, b, c, d)
					if signed {
						var ih I128
						lo, ih = I128CarryingMulAdd(I128(a), I128(b), I128(c), I128(d))
						hi = U128(ih)
					}
					if lo != wantLo || hi != wantHi {
						t.Fatalf("signed=%v %v * %v + %v + %v: %v %v, want %v %v", signed, a, b, c, d, lo, hi, wantLo, wantHi)
					}
				}
			}
		}
	}
}

func Test128FloatConversions(t *testing.T) {
	values := integer128Cases()
	// Values immediately around rounding ties, including ties that f64
	// rounds to exactly the wrong midpoint for a subsequent f32 conversion.
	for n := uint64(24); n < 128; n++ {
		power := U128Shl(U128{Lo: 1}, n)
		half := U128Shl(U128{Lo: 1}, n-24)
		for _, delta := range []U128{U128SubValue(half, U128{Lo: 1}), half, U128AddValue(half, U128{Lo: 1})} {
			values = append(values, U128AddValue(power, delta), U128SubValue(power, delta))
		}
	}
	for _, x := range values {
		for _, signed := range []bool{false, true} {
			v := bigUnsigned128(x)
			got64, got32 := U128ToF64(x), U128ToF32(x)
			if signed {
				v = bigSigned128(I128(x))
				got64, got32 = I128ToF64(I128(x)), I128ToF32(I128(x))
			}
			f := new(big.Float).SetPrec(128).SetInt(v)
			want64, _ := f.Float64()
			want32, _ := f.Float32()
			if got64 != want64 || got32 != want32 {
				t.Fatalf("signed=%v %v -> f64 %x/%x f32 %x/%x", signed, x, math.Float64bits(got64), math.Float64bits(want64), math.Float32bits(got32), math.Float32bits(want32))
			}
		}
	}
	floats := []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1), 0.9, -0.9, 1.9, -1.9, 0x1p64, -0x1p64, 0x1p127, -0x1p127, 0x1p128}
	rng := rand.New(rand.NewPCG(543, 239))
	for range 1000 {
		floats = append(floats, math.Float64frombits(rng.Uint64()))
	}
	for _, x := range floats {
		for _, signed := range []bool{false, true} {
			want, min, max := new(big.Int), new(big.Int), new(big.Int)
			if signed {
				min.Neg(new(big.Int).Lsh(big.NewInt(1), 127))
				max.Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
			} else {
				max.Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
			}
			switch {
			case math.IsNaN(x):
			case math.IsInf(x, 1):
				want.Set(max)
			case math.IsInf(x, -1):
				want.Set(min)
			default:
				big.NewFloat(x).Int(want)
				if want.Cmp(min) < 0 {
					want.Set(min)
				}
				if want.Cmp(max) > 0 {
					want.Set(max)
				}
			}
			got := FloatToU128(x)
			if signed {
				got = U128(FloatToI128(x))
			}
			if got != bigTo128(want) {
				t.Fatalf("signed=%v %g -> %v, want %v", signed, x, got, want)
			}
		}
	}
}

func Test128MoreNoGoAllocs(t *testing.T) {
	a, b := U128{17, 39}, U128{12, 1}
	requireNoGoAllocations(t, 100, func() {
		q, r := U128DivRem(a, b)
		lo, hi := U128CarryingMulAdd(q, b, r, U128{})
		if lo != a || hi != (U128{}) || FloatToU128(U128ToF64(b)) == (U128{}) {
			panic("arithmetic")
		}
	})
}
