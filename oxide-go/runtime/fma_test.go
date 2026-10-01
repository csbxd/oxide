package oxide

import (
	"math"
	"math/big"
	"testing"
)

func TestFMA32ExactRounding(t *testing.T) {
	check := func(a, b, c uint32) {
		x, y, z := math.Float32frombits(a), math.Float32frombits(b), math.Float32frombits(c)
		if a&0x7f800000 == 0x7f800000 || b&0x7f800000 == 0x7f800000 || c&0x7f800000 == 0x7f800000 {
			return
		}
		// 1024 bits cover the entire exponent range of an exact f32 product
		// plus an f32 addend. Float32 then rounds only the exact final sum.
		fx := new(big.Float).SetPrec(1024).SetFloat64(float64(x))
		fy := new(big.Float).SetPrec(1024).SetFloat64(float64(y))
		fz := new(big.Float).SetPrec(1024).SetFloat64(float64(z))
		sum := new(big.Float).SetPrec(1024).Mul(fx, fy)
		sum.Add(sum, fz)
		want, _ := sum.Float32()
		if got := FMA32(x, y, z); math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("fma(%08x,%08x,%08x)=%08x, want %08x", a, b, c, math.Float32bits(got), math.Float32bits(want))
		}
	}
	bounds := []uint32{
		0, 0x80000000, 1, 0x80000001, 0x007fffff, 0x00800000, 0x00800001,
		0x3f000000, 0x3f800000, 0x3f800001, 0x3f800003, 0x3fc00000, 0x40000000,
		0x7f7fffff, 0xff7fffff,
	}
	for _, a := range bounds {
		for _, b := range bounds {
			for _, c := range bounds {
				check(a, b, c)
			}
		}
	}
	// Tiny addends push an f64 halfway value to either side of the f32 tie.
	for _, x := range []uint32{0x3f800001, 0x3f800003, 0xbf800001, 0xbf800003} {
		for _, y := range []uint32{0x3fc00000, 0xbfc00000} {
			for _, z := range []uint32{1, 0x80000001} {
				check(x, y, z)
			}
		}
	}
	state := uint64(0x6a09e667f3bcc909)
	next := func() uint32 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return uint32(state)
	}
	for range 50000 {
		check(next(), next(), next())
	}
}

func TestFMA32Nonfinite(t *testing.T) {
	bounds := []uint32{0, 0x80000000, 0x3f800000, 0xbf800000, 0x7f800000, 0xff800000, 0x7fc00001, 0x7f800001}
	for _, a := range bounds {
		for _, b := range bounds {
			for _, c := range bounds {
				x, y, z := math.Float32frombits(a), math.Float32frombits(b), math.Float32frombits(c)
				want := float32(math.FMA(float64(x), float64(y), float64(z)))
				got := FMA32(x, y, z)
				if math.IsNaN(float64(want)) {
					if !math.IsNaN(float64(got)) {
						t.Fatalf("fma(%08x,%08x,%08x)=%08x, want NaN", a, b, c, math.Float32bits(got))
					}
				} else if math.Float32bits(got) != math.Float32bits(want) {
					t.Fatalf("fma(%08x,%08x,%08x)=%08x, want %08x", a, b, c, math.Float32bits(got), math.Float32bits(want))
				}
			}
		}
	}
}
