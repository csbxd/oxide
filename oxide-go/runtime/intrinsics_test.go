package oxide

import (
	"math"
	"math/big"
	"math/bits"
	"sync"
	"testing"
	"unsafe"
)

func TestU128MulMatchesBigInt(t *testing.T) {
	for i := uint64(0); i < 1000; i += 37 {
		a := U128{Lo: i*0x9e3779b97f4a7c15 + 1, Hi: ^(i * 0x123456789abcdef)}
		b := U128{Lo: i*0xd6e8feb86659fd93 + 7, Hi: i*0x517cc1b727220a95 + 3}
		got := U128MulValue(a, b)
		am := new(big.Int).SetUint64(a.Hi)
		am.Lsh(am, 64)
		am.Add(am, new(big.Int).SetUint64(a.Lo))
		bm := new(big.Int).SetUint64(b.Hi)
		bm.Lsh(bm, 64)
		bm.Add(bm, new(big.Int).SetUint64(b.Lo))
		mod := new(big.Int).Lsh(big.NewInt(1), 128)
		want := new(big.Int).Mul(am, bm)
		want.Mod(want, mod)
		lo := new(big.Int).And(new(big.Int).Set(want), new(big.Int).SetUint64(^uint64(0))).Uint64()
		hi := new(big.Int).Rsh(new(big.Int).Set(want), 64).Uint64()
		if got.Lo != lo || got.Hi != hi {
			t.Fatalf("mul mismatch %#v %#v", got, want)
		}
	}
}

func TestNarrowIntegerIntrinsics(t *testing.T) {
	for a := 0; a < 256; a++ {
		u := uint8(a)
		if LeadingZeros(u) != uint32(bits.LeadingZeros8(u)) || TrailingZeros(u) != uint32(bits.TrailingZeros8(u)) || OnesCount(u) != uint32(bits.OnesCount8(u)) {
			t.Fatalf("bit counts for %d", a)
		}
		if LeadingZeros(int8(u)) != LeadingZeros(u) || TrailingZeros(int8(u)) != TrailingZeros(u) || OnesCount(int8(u)) != OnesCount(u) {
			t.Fatalf("signed bit counts for %d", a)
		}
		for n := uint32(0); n < 32; n++ {
			if RotateLeft(u, n) != bits.RotateLeft8(u, int(n)) || RotateRight(u, n) != bits.RotateLeft8(u, -int(n)) {
				t.Fatalf("rotate %d by %d", a, n)
			}
		}
		for b := 0; b < 256; b++ {
			v := uint8(b)
			if int(SaturatingAdd(u, v)) != min(a+b, 255) || int(SaturatingSub(u, v)) != max(a-b, 0) {
				t.Fatalf("unsigned saturation %d %d", a, b)
			}
			x, y := int8(u), int8(v)
			if int(SaturatingAdd(x, y)) != min(max(int(x)+int(y), -128), 127) || int(SaturatingSub(x, y)) != min(max(int(x)-int(y), -128), 127) {
				t.Fatalf("signed saturation %d %d", x, y)
			}
		}
	}
	if SaturatingAdd(int64(math.MaxInt64), int64(1)) != math.MaxInt64 || SaturatingSub(int64(math.MinInt64), int64(1)) != math.MinInt64 {
		t.Fatal("64-bit saturation")
	}
	if ByteSwap(uint16(0x1234)) != 0x3412 || ByteSwap(uint32(0x12345678)) != 0x78563412 || ByteSwap(uint64(0x0123456789abcdef)) != 0xefcdab8967452301 {
		t.Fatal("byte swap")
	}
}

func TestAtomicResultsAndNarrowStorage(t *testing.T) {
	for _, width := range []uintptr{1, 2, 4, 8} {
		var storage [3]uint64
		storage[0], storage[2] = math.MaxUint64, math.MaxUint64
		p := unsafe.Pointer(&storage[1])
		AtomicStore(p, 7, width)
		if AtomicAdd(p, 5, width) != 7 || AtomicSub(p, 3, width) != 12 || AtomicSwap(p, 19, width) != 9 {
			t.Fatalf("RMW old value, width %d", width)
		}
		if got, ok := AtomicCompareExchange(p, 19, 23, width); !ok || got != 19 || AtomicLoad(p, width) != 23 {
			t.Fatalf("successful CAS: %d %v (width %d)", got, ok, width)
		}
		if got, ok := AtomicCompareExchange(p, 19, 0, width); ok || got != 23 {
			t.Fatalf("failed CAS: %d %v (width %d)", got, ok, width)
		}
		AtomicStore(p, math.MaxUint64, width)
		if _, ok := AtomicCompareExchange(p, math.MaxUint64, 0, width); !ok {
			t.Fatalf("signed bit pattern CAS, width %d", width)
		}
		if storage[0] != math.MaxUint64 || storage[2] != math.MaxUint64 || storage[1] != 0 {
			t.Fatalf("atomic overwrote adjacent storage, width %d", width)
		}
	}
}

func TestBitReverseIntegerWidths(t *testing.T) {
	checkBitReverse[int8](t)
	checkBitReverse[uint8](t)
	checkBitReverse[int16](t)
	checkBitReverse[uint16](t)
	checkBitReverse[int32](t)
	checkBitReverse[uint32](t)
	checkBitReverse[int64](t)
	checkBitReverse[uint64](t)
	checkBitReverse[int](t)
	checkBitReverse[uintptr](t)
}

func checkBitReverse[T Integer](t *testing.T) {
	t.Helper()
	values := []uint64{0, 1, 2, 0x0123456789abcdef, 1 << 31, 1 << 63, ^uint64(0)}
	for i := uint64(0); i < 256; i++ {
		values = append(values, i)
	}
	width := unsafe.Sizeof(T(0)) * 8
	for _, value := range values {
		v := T(value)
		u, want := uint64(v), uint64(0)
		for range width {
			want = want<<1 | u&1
			u >>= 1
		}
		if got := BitReverse(v); got != T(want) || BitReverse(got) != v {
			t.Fatalf("%T reverse bits %#x: got %#x want %#x", v, v, got, T(want))
		}
	}
}

func TestAtomicConcurrentCompareExchange(t *testing.T) {
	var value uint64
	p := unsafe.Pointer(&value)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				for {
					old := AtomicLoad(p, 8)
					got, ok := AtomicCompareExchange(p, old, old+1, 8)
					if ok {
						if got != old {
							t.Errorf("successful CAS returned %d, want %d", got, old)
						}
						break
					}
					if got == old {
						t.Errorf("strong CAS spuriously failed")
					}
				}
			}
		})
	}
	wg.Wait()
	if value != 8000 {
		t.Fatalf("lost increments: %d", value)
	}
}

func TestIntrinsicRuntimeNoAllocs(t *testing.T) {
	var x uint64
	p := unsafe.Pointer(&x)
	requireNoGoAllocations(t, 100, func() {
		AtomicStore(p, 1, 8)
		AtomicAdd(p, 1, 8)
		AtomicCompareExchange(p, 2, 3, 8)
		AtomicFence()
		WriteBytes(p, 0, 8)
	})
}

func TestFloatToIntSaturatesLikeRust(t *testing.T) {
	for _, x := range []float64{math.NaN(), math.Inf(-1), -300, -128.9, -127.1, -1.9, 0, 1.9, 126.9, 127.9, 300, math.Inf(1)} {
		want := int8(x)
		if math.IsNaN(x) {
			want = 0
		} else if x <= -128 {
			want = -128
		} else if x >= 128 {
			want = 127
		}
		if got := FloatToInt[int8](x); got != want {
			t.Fatalf("int8(%v): got %d want %d", x, got, want)
		}
		uwant := uint8(x)
		if math.IsNaN(x) || x <= 0 {
			uwant = 0
		} else if x >= 256 {
			uwant = 255
		}
		if got := FloatToInt[uint8](x); got != uwant {
			t.Fatalf("uint8(%v): got %d want %d", x, got, uwant)
		}
	}
}
