package oxide

import (
	"math/big"
	"math/rand/v2"
	"testing"
)

func Test128SaturatingBoundaries(t *testing.T) {
	maxUnsigned := U128{Lo: ^uint64(0), Hi: ^uint64(0)}
	maxSigned := I128{Lo: ^uint64(0), Hi: (1 << 63) - 1}
	minSigned := I128{Hi: 1 << 63}
	for _, test := range []struct {
		a, b, add, sub U128
	}{
		{U128{}, U128{}, U128{}, U128{}},
		{U128{}, maxUnsigned, maxUnsigned, U128{}},
		{maxUnsigned, U128{Lo: 1}, maxUnsigned, U128{Lo: ^uint64(0) - 1, Hi: ^uint64(0)}},
		{maxUnsigned, maxUnsigned, maxUnsigned, U128{}},
		{U128{Lo: ^uint64(0)}, U128{Lo: 1}, U128{Hi: 1}, U128{Lo: ^uint64(0) - 1}},
		{U128{Hi: 1}, U128{Lo: 1}, U128{Lo: 1, Hi: 1}, U128{Lo: ^uint64(0)}},
	} {
		if add, sub := U128SaturatingAdd(test.a, test.b), U128SaturatingSub(test.a, test.b); add != test.add || sub != test.sub {
			t.Fatalf("u128 saturation %#v, %#v: add %#v/%#v, sub %#v/%#v", test.a, test.b, add, test.add, sub, test.sub)
		}
	}
	for _, test := range []struct {
		a, b, add, sub I128
	}{
		{I128{}, minSigned, minSigned, maxSigned},
		{I128{}, maxSigned, maxSigned, I128{Lo: 1, Hi: 1 << 63}},
		{maxSigned, I128From64(1), maxSigned, I128{Lo: ^uint64(0) - 1, Hi: (1 << 63) - 1}},
		{minSigned, I128From64(-1), minSigned, I128{Lo: 1, Hi: 1 << 63}},
		{maxSigned, minSigned, I128From64(-1), maxSigned},
		{minSigned, maxSigned, I128From64(-1), minSigned},
		{minSigned, minSigned, minSigned, I128{}},
		{maxSigned, maxSigned, maxSigned, I128{}},
		{I128From64(-1), I128From64(1), I128{}, I128From64(-2)},
	} {
		if add, sub := I128SaturatingAdd(test.a, test.b), I128SaturatingSub(test.a, test.b); add != test.add || sub != test.sub {
			t.Fatalf("i128 saturation %#v, %#v: add %#v/%#v, sub %#v/%#v", test.a, test.b, add, test.add, sub, test.sub)
		}
	}
}

func Test128SaturatingAgainstBigInt(t *testing.T) {
	uMin := new(big.Int)
	uMax := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	iMax := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
	iMin := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))
	random := rand.New(rand.NewPCG(29, 71))
	for range 10000 {
		a := U128{Lo: random.Uint64(), Hi: random.Uint64()}
		b := U128{Lo: random.Uint64(), Hi: random.Uint64()}
		for _, signed := range []bool{false, true} {
			x, y := bigUnsigned128(a), bigUnsigned128(b)
			lo, hi := uMin, uMax
			if signed {
				x, y = bigSigned128(I128(a)), bigSigned128(I128(b))
				lo, hi = iMin, iMax
			}
			for _, subtract := range []bool{false, true} {
				want := new(big.Int)
				var got U128
				if subtract {
					want.Sub(x, y)
					got = U128SaturatingSub(a, b)
					if signed {
						got = U128(I128SaturatingSub(I128(a), I128(b)))
					}
				} else {
					want.Add(x, y)
					got = U128SaturatingAdd(a, b)
					if signed {
						got = U128(I128SaturatingAdd(I128(a), I128(b)))
					}
				}
				if want.Cmp(lo) < 0 {
					want.Set(lo)
				} else if want.Cmp(hi) > 0 {
					want.Set(hi)
				}
				if got != bigTo128(want) {
					t.Fatalf("128-bit saturation signed=%v subtract=%v a=%#v b=%#v: got %#v want %#v", signed, subtract, a, b, got, bigTo128(want))
				}
			}
		}
	}
}

var saturating128Sink U128

func Test128SaturatingNoGoAllocs(t *testing.T) {
	a, b := U128{Lo: ^uint64(0), Hi: ^uint64(0)}, U128{Lo: 1, Hi: 1 << 63}
	requireNoGoAllocations(t, 100, func() {
		u := U128SaturatingAdd(a, b)
		v := U128SaturatingSub(b, a)
		x := I128SaturatingAdd(I128(a), I128(b))
		y := I128SaturatingSub(I128(b), I128(a))
		saturating128Sink = U128Xor(U128Xor(u, v), U128Xor(U128(x), U128(y)))
	})
}
