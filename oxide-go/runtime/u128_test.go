package oxide

import (
	"math/big"
	"math/rand/v2"
	"testing"
)

func bigUnsigned128(v U128) *big.Int {
	r := new(big.Int).SetUint64(v.Hi)
	r.Lsh(r, 64)
	return r.Add(r, new(big.Int).SetUint64(v.Lo))
}

func bigSigned128(v I128) *big.Int {
	r := bigUnsigned128(U128(v))
	if v.Hi>>63 != 0 {
		r.Sub(r, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	return r
}

func bigTo128(v *big.Int) U128 {
	r := new(big.Int).Mod(v, new(big.Int).Lsh(big.NewInt(1), 128))
	lo := r.Uint64()
	hi := r.Rsh(r, 64).Uint64()
	return U128{Lo: lo, Hi: hi}
}

func integer128Cases() []U128 {
	values := []U128{
		{}, {Lo: 1}, {Lo: 2}, {Lo: 1 << 32}, {Lo: 1 << 63},
		{Lo: ^uint64(0)}, {Hi: 1},
		{Lo: 1, Hi: 1}, {Lo: ^uint64(0), Hi: (1 << 63) - 1},
		{Hi: 1 << 63}, {Lo: 1, Hi: 1 << 63},
		{Lo: ^uint64(0) - 1, Hi: ^uint64(0)}, {Lo: ^uint64(0), Hi: ^uint64(0)},
	}
	rng := rand.New(rand.NewPCG(123, 456))
	for range 32 {
		values = append(values, U128{Lo: rng.Uint64(), Hi: rng.Uint64()})
	}
	return values
}

func Test128ArithmeticMatchesBigInt(t *testing.T) {
	mod := new(big.Int).Lsh(big.NewInt(1), 128)
	minSigned := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))
	maxSigned := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
	values := integer128Cases()
	for _, a := range values {
		for _, b := range values {
			ua, ub := bigUnsigned128(a), bigUnsigned128(b)
			sa, sb := bigSigned128(I128(a)), bigSigned128(I128(b))
			for _, op := range []struct {
				name string
				big  func(*big.Int, *big.Int, *big.Int) *big.Int
				u    func(U128, U128) (U128, bool)
				i    func(I128, I128) (I128, bool)
			}{
				{"add", (*big.Int).Add, U128Add, I128Add},
				{"sub", (*big.Int).Sub, U128Sub, I128Sub},
				{"mul", (*big.Int).Mul, U128Mul, I128Mul},
			} {
				want := op.big(new(big.Int), ua, ub)
				wantOverflow := want.Sign() < 0 || want.Cmp(mod) >= 0
				if got, overflow := op.u(a, b); got != bigTo128(want) || overflow != wantOverflow {
					t.Fatalf("u128 %s(%#v, %#v): got %#v/%v, want %#v/%v", op.name, a, b, got, overflow, bigTo128(want), wantOverflow)
				}
				want = op.big(new(big.Int), sa, sb)
				wantOverflow = want.Cmp(minSigned) < 0 || want.Cmp(maxSigned) > 0
				if got, overflow := op.i(I128(a), I128(b)); got != I128(bigTo128(want)) || overflow != wantOverflow {
					t.Fatalf("i128 %s(%#v, %#v): got %#v/%v, want %#v/%v", op.name, a, b, got, overflow, I128(bigTo128(want)), wantOverflow)
				}
			}
		}
	}
}

func Test128ShiftsWrapCount(t *testing.T) {
	for _, a := range integer128Cases() {
		for _, n := range []uint64{0, 1, 31, 63, 64, 65, 127, 128, 129, 255, 256, ^uint64(0)} {
			shift := uint(n & 127)
			wantLeft := bigTo128(new(big.Int).Lsh(bigUnsigned128(a), shift))
			wantRight := bigTo128(new(big.Int).Rsh(bigUnsigned128(a), shift))
			wantSignedRight := I128(bigTo128(new(big.Int).Rsh(bigSigned128(I128(a)), shift)))
			if got := U128Shl(a, n); got != wantLeft {
				t.Fatalf("u128 %#v << %d: %#v != %#v", a, n, got, wantLeft)
			}
			if got := I128Shl(I128(a), n); got != I128(wantLeft) {
				t.Fatalf("i128 %#v << %d: %#v != %#v", a, n, got, wantLeft)
			}
			if got := U128Shr(a, n); got != wantRight {
				t.Fatalf("u128 %#v >> %d: %#v != %#v", a, n, got, wantRight)
			}
			if got := I128Shr(I128(a), n); got != wantSignedRight {
				t.Fatalf("i128 %#v >> %d: %#v != %#v", a, n, got, wantSignedRight)
			}
		}
	}
}

func Test128ArithmeticNoGoAllocs(t *testing.T) {
	a, b := U128{Lo: 0x123456789abcdef0, Hi: 1}, U128{Lo: 1, Hi: 1 << 63}
	requireNoGoAllocations(t, 100, func() {
		u, _ := U128Mul(a, b)
		i, _ := I128Mul(I128(a), I128(b))
		if U128(i) != u {
			panic("signed multiplication changed the low 128 bits")
		}
		if U128Shl(a, 128) != a || I128Shr(I128(b), 128) != I128(b) {
			panic("shift count did not wrap")
		}
	})
}

func Test128ReverseBits(t *testing.T) {
	for _, a := range integer128Cases() {
		source := bigUnsigned128(a)
		want := new(big.Int)
		for i := 0; i < 128; i++ {
			want.SetBit(want, 127-i, source.Bit(i))
		}
		if got := U128ReverseBits(a); got != bigTo128(want) || U128ReverseBits(got) != a {
			t.Fatalf("u128 reverse bits %#v: got %#v want %#v", a, got, bigTo128(want))
		}
		if got := I128ReverseBits(I128(a)); U128(got) != bigTo128(want) {
			t.Fatalf("i128 reverse bits %#v: got %#v want %#v", a, got, bigTo128(want))
		}
	}
}
