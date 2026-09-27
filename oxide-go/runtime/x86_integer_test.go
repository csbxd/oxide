package oxide

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"
)

func x86TestBytes[T any](v *T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(v)), int(unsafe.Sizeof(*v)))
}

func checkX86Integer(t *testing.T, fn func(dst, a, b unsafe.Pointer), a, b, want []byte) {
	t.Helper()
	for _, offset := range []int{0, 1, 32, 64} {
		var memory [96]byte
		copy(memory[:32], a)
		copy(memory[32:64], b)
		fn(unsafe.Pointer(&memory[offset]), unsafe.Pointer(&memory[0]), unsafe.Pointer(&memory[32]))
		if got := memory[offset : offset+len(want)]; !bytes.Equal(got, want) {
			t.Fatalf("destination offset %d: %x, want %x", offset, got, want)
		}
	}
}

func TestX86IntegerPack(t *testing.T) {
	t.Run("signed dword", func(t *testing.T) {
		a := [8]int32{-32769, -32768, -1, 32767, 32768, -2147483648, 2147483647, 17}
		b := [8]int32{3, 4, 5, 6, -7, -8, -9, -10}
		want := [16]int16{-32768, -32768, -1, 32767, 3, 4, 5, 6, 32767, -32768, 32767, 17, -7, -8, -9, -10}
		checkX86Integer(t, X86PackI32ToI16, x86TestBytes(&a), x86TestBytes(&b), x86TestBytes(&want))
	})
	t.Run("unsigned word", func(t *testing.T) {
		a := [8]int32{-1, 0, 1, 65535, 65536, 2147483647, -2147483648, 99}
		b := [8]int32{65534, -2, 23, 24, 25, 26, 27, 28}
		want := [16]uint16{0, 0, 1, 65535, 65534, 0, 23, 24, 65535, 65535, 0, 99, 25, 26, 27, 28}
		checkX86Integer(t, X86PackI32ToU16, x86TestBytes(&a), x86TestBytes(&b), x86TestBytes(&want))
	})
	t.Run("unsigned byte", func(t *testing.T) {
		a := [16]int16{-1, 0, 1, 254, 255, 256, -32768, 32767, 16, 17, 18, 19, 20, 21, 22, 23}
		b := [16]int16{128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143}
		want := [32]byte{0, 0, 1, 254, 255, 255, 0, 255, 128, 129, 130, 131, 132, 133, 134, 135, 16, 17, 18, 19, 20, 21, 22, 23, 136, 137, 138, 139, 140, 141, 142, 143}
		checkX86Integer(t, X86PackI16ToU8, x86TestBytes(&a), x86TestBytes(&b), want[:])
	})
}

func TestX86IntegerMadd(t *testing.T) {
	for _, width := range []int{16, 32} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := [16]int16{-32768, -32768, -2, 3, 32767, 32767, 0, 1, 1, 2, 3, 4, -5, 6, 7, 8}
			b := [16]int16{-32768, -32768, 5, -7, 32767, 32767, 0, -1, 10, 20, 30, 40, 50, -60, 70, 80}
			want := [8]int32{-2147483648, -31, 2147352578, -1, 50, 250, -610, 1130}
			checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86MaddI16(dst, a, b, width) }, x86TestBytes(&a)[:width], x86TestBytes(&b)[:width], x86TestBytes(&want)[:width])

			ua := [32]byte{255, 255, 255, 255, 128, 128, 2, 3, 1, 0, 10, 20, 255, 1, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			sb := [32]int8{-128, -128, 127, 127, -128, 127, -7, 9, -128, 127, 2, -3, 127, -128, 127, -128, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
			words := [16]int16{-32768, 32767, -128, 13, -128, -40, 32257, 0, -3, -7, -11, -15, -19, -23, -27, -31}
			checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86MaddU8I8(dst, a, b, width) }, ua[:width], x86TestBytes(&sb)[:width], x86TestBytes(&words)[:width])
		})
	}
}

func TestX86IntegerSADAndShuffle(t *testing.T) {
	var a, b [32]byte
	for i := range a {
		a[i], b[i] = byte(i), byte(255-i)
	}
	wantSAD := [4]uint64{1984, 1856, 1728, 1600}
	control := [16]byte{15, 0, 0x80, 0xff, 0x10, 0x2e, 0x7d, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	wantShuffle := [32]byte{16, 1, 0, 0, 1, 15, 14, 2, 3, 4, 5, 6, 7, 8, 9, 10, 32, 17, 0, 0, 17, 31, 30, 18, 19, 20, 21, 22, 23, 24, 25, 26}
	for _, width := range []int{16, 32} {
		checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86SADU8(dst, a, b, width) }, a[:width], b[:width], x86TestBytes(&wantSAD)[:width])
	}
	for i := range a {
		a[i], b[i] = byte(i+1), control[i%16]
	}
	for _, width := range []int{16, 32} {
		checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86ShuffleU8(dst, a, b, width) }, a[:width], b[:width], wantShuffle[:width])
	}
}

func TestX86IntegerShift(t *testing.T) {
	a := [4]uint32{0xffffffff, 0x80000000, 3, 0}
	for _, tc := range []struct {
		count uint64
		want  [4]uint32
	}{
		{0, a}, {1, [4]uint32{0x7fffffff, 0x40000000, 1, 0}}, {31, [4]uint32{1, 1, 0, 0}},
		{32, [4]uint32{}}, {33, [4]uint32{}}, {1 << 32, [4]uint32{}}, {^uint64(0), [4]uint32{}},
	} {
		b := [2]uint64{tc.count, ^uint64(0)}
		checkX86Integer(t, X86ShiftRightU32, x86TestBytes(&a), x86TestBytes(&b), x86TestBytes(&tc.want))
	}
}

func TestX86IntegerCarrylessMultiply(t *testing.T) {
	// Intel's CLMUL whitepaper vectors, also used by Rust stdarch's test.
	a := [2]uint64{0x63746f725d53475d, 0x7b5b546573745665}
	b := [2]uint64{0x5b477565726f6e5d, 0x4869285368617929}
	want := [4][2]uint64{
		{0x929633d5d36f0451, 0x1d4d84c85c3440c0},
		{0xbabf262df4b7d5c9, 0x1a2bf6db3a30862f},
		{0x7fa540ac2a281315, 0x1bd17c8d556ab5a1},
		{0xd66ee03e410fd4ed, 0x1d1e1f2c592e7c45},
	}
	for immediate := range 256 {
		selection := (immediate & 1) | ((immediate >> 3) & 2)
		checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86CarrylessMul64(dst, a, b, byte(immediate)) }, x86TestBytes(&a), x86TestBytes(&b), x86TestBytes(&want[selection]))
	}
	for _, tc := range []struct{ a, b, lo, hi uint64 }{
		{0, ^uint64(0), 0, 0}, {0x123456789abcdef0, 1, 0x123456789abcdef0, 0},
		{1 << 63, 1 << 63, 0, 1 << 62}, {^uint64(0), ^uint64(0), 0x5555555555555555, 0x5555555555555555},
	} {
		x, y, expected := [2]uint64{tc.a, 0}, [2]uint64{tc.b, 0}, [2]uint64{tc.lo, tc.hi}
		checkX86Integer(t, func(dst, a, b unsafe.Pointer) { X86CarrylessMul64(dst, a, b, 0) }, x86TestBytes(&x), x86TestBytes(&y), x86TestBytes(&expected))
	}
}

func TestX86IntegerNoAllocations(t *testing.T) {
	var a, b, dst [32]byte
	x, y, out := unsafe.Pointer(&a), unsafe.Pointer(&b), unsafe.Pointer(&dst)
	requireNoGoAllocations(t, 100, func() {
		X86PackI32ToI16(out, x, y)
		X86PackI32ToU16(out, x, y)
		X86PackI16ToU8(out, x, y)
		X86MaddI16(out, x, y, 32)
		X86MaddU8I8(out, x, y, 32)
		X86SADU8(out, x, y, 32)
		X86ShuffleU8(out, x, y, 32)
		X86ShiftRightU32(out, x, y)
		X86CarrylessMul64(out, x, y, 0)
	})
}
