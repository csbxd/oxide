//go:build linux && amd64

package oxide

import (
	"math"
	"testing"
	"unsafe"
)

func TestX86CompareF32x4(t *testing.T) {
	a := [4]uint32{0x3f800000, 0x40000000, 0x40400000, 0x7fc12345}
	b := [4]uint32{0x40000000, 0x40000000, 0x40000000, 0x3f800000}
	wants := [16][4]bool{
		{false, true, false, false},  // EQ
		{true, false, false, false},  // LT
		{true, true, false, false},   // LE
		{false, false, false, true},  // UNORD
		{true, false, true, true},    // NEQ
		{false, true, true, true},    // NLT
		{false, false, true, true},   // NLE
		{true, true, true, false},    // ORD
		{false, true, false, true},   // EQ_UQ
		{true, false, false, true},   // NGE
		{true, true, false, true},    // NGT
		{false, false, false, false}, // FALSE
		{true, false, true, false},   // NEQ_OQ
		{false, true, true, false},   // GE
		{false, false, true, false},  // GT
		{true, true, true, true},     // TRUE
	}
	_, _, features, _ := X86CPUID(1, 0)
	avx := features&(1<<28|1<<27) == 1<<28|1<<27 && X86Xgetbv(0)&6 == 6
	for predicate := range 32 {
		if predicate > 7 && !avx {
			continue
		}
		want := wants[predicate%16]
		var out [4]uint32
		X86CompareF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b), uint8(predicate))
		for i, yes := range want {
			bits := uint32(0)
			if yes {
				bits = ^uint32(0)
			}
			if out[i] != bits {
				t.Fatalf("predicate %d lane %d: %08x, want %08x", predicate, i, out[i], bits)
			}
		}
		alias := a
		X86CompareF32x4(unsafe.Pointer(&alias), unsafe.Pointer(&alias), unsafe.Pointer(&b), uint8(predicate))
		if alias != out {
			t.Fatalf("predicate %d aliases input incorrectly", predicate)
		}
	}
}

func TestX86MinMaxF32x4Bits(t *testing.T) {
	// Equal zeros select the second operand, as do unordered comparisons.
	// A signaling NaN in that operand is returned without quieting its bits.
	a := [4]uint32{0, 0x80000000, 0x7fc12345, 0x3f800000}
	b := [4]uint32{0x80000000, 0, 0x40400000, 0x7f812345}
	for _, fn := range []func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer){X86MinF32x4, X86MaxF32x4} {
		var out [4]uint32
		fn(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b))
		if out != b {
			t.Fatalf("NaN/zero selection: %08x, want %08x", out, b)
		}
		alias := b
		fn(unsafe.Pointer(&alias), unsafe.Pointer(&a), unsafe.Pointer(&alias))
		if alias != b {
			t.Fatalf("second input alias: %08x, want %08x", alias, b)
		}
	}
	a = [4]uint32{0x3f800000, 0xc0000000, 0x7f800000, 0xff800000}
	b = [4]uint32{0x40000000, 0xbf800000, 0x40400000, 0xc0400000}
	var out [4]uint32
	X86MinF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b))
	if want := ([4]uint32{a[0], a[1], b[2], a[3]}); out != want {
		t.Fatalf("min: %08x, want %08x", out, want)
	}
	X86MaxF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b))
	if want := ([4]uint32{b[0], b[1], a[2], b[3]}); out != want {
		t.Fatalf("max: %08x, want %08x", out, want)
	}
}

func TestX86ConvertF32x4(t *testing.T) {
	a := [4]float32{1.5, 2.5, -1.5, -2.5}
	var out [4]int32
	X86RoundToI32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
	if want := ([4]int32{2, 2, -2, -2}); out != want {
		t.Fatalf("nearest-even: %v, want %v", out, want)
	}
	X86TruncToI32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
	if want := ([4]int32{1, 2, -1, -2}); out != want {
		t.Fatalf("truncate: %v, want %v", out, want)
	}
	invalid := [4]uint32{0x7fc12345, 0x7f800000, 0x4f000000, 0xcf000001}
	want := [4]int32{math.MinInt32, math.MinInt32, math.MinInt32, math.MinInt32}
	for _, fn := range []func(unsafe.Pointer, unsafe.Pointer){X86RoundToI32x4, X86TruncToI32x4} {
		fn(unsafe.Pointer(&out), unsafe.Pointer(&invalid))
		if out != want {
			t.Fatalf("integer indefinite: %v, want %v", out, want)
		}
	}
}

func TestX86ReciprocalF32x4(t *testing.T) {
	a := [4]uint32{0, 0x80000000, 0x7f800000, 0xff800000}
	var out [4]uint32
	X86ReciprocalF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
	if want := ([4]uint32{0x7f800000, 0xff800000, 0, 0x80000000}); out != want {
		t.Fatalf("zero/infinity: %08x, want %08x", out, want)
	}
	a = [4]uint32{1, 0x80000001, 0x7fc12345, 0x7f812345}
	X86ReciprocalF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
	if want := ([4]uint32{0x7f800000, 0xff800000, 0x7fc12345, 0x7fc12345}); out != want {
		t.Fatalf("denormal/NaN: %08x, want %08x", out, want)
	}
	// The instruction's approximation can vary by CPU; assert Intel's relative
	// error bound rather than replacing it with a particular software estimate.
	values := [4]float32{1, 3, -7, 12345}
	X86ReciprocalF32x4(unsafe.Pointer(&out), unsafe.Pointer(&values))
	for i, x := range values {
		if relative := math.Abs(float64(math.Float32frombits(out[i]))*float64(x) - 1); relative > 1.5/4096 {
			t.Fatalf("reciprocal %g: %08x exceeds relative error bound", x, out[i])
		}
	}
}

func TestX86CPUQueries(t *testing.T) {
	max, b, c, d := X86CPUID(0, 0)
	if max < 1 || b|c|d == 0 {
		t.Fatalf("CPUID vendor/max leaf: %x %x %x %x", max, b, c, d)
	}
	_, _, c, d = X86CPUID(1, 0)
	if d&(1<<26) == 0 {
		t.Fatal("AMD64 CPU did not report baseline SSE2")
	}
	if c&(1<<27) != 0 {
		if value := X86Xgetbv(0); value&1 == 0 {
			t.Fatalf("XCR0 lacks enabled x87 state: %x", value)
		}
	}
	X86Pause()
}

func TestX86FloatNoAllocs(t *testing.T) {
	a, b := [4]float32{1, 2, 3, 4}, [4]float32{2, 3, 4, 5}
	var out [4]uint32
	requireNoGoAllocations(t, 100, func() {
		X86MaxF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b))
		X86MinF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b))
		X86CompareF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a), unsafe.Pointer(&b), 2)
		X86ReciprocalF32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
		X86RoundToI32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
		X86TruncToI32x4(unsafe.Pointer(&out), unsafe.Pointer(&a))
		X86CPUID(0, 0)
		X86Pause()
	})
}
