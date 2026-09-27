package oxide

import (
	"encoding/binary"
	"unsafe"
)

// The AVX2 pack and shuffle instructions operate on independent 128-bit
// halves. LLVM documents their element order in avx2intrin.h:
// https://clang.llvm.org/doxygen/avx2intrin_8h.html
// Snapshot inputs before writes, since MIR destinations may alias operands.
func x86IntegerInputs(a, b unsafe.Pointer, width int) (x, y [32]byte) {
	copy(x[:width], unsafe.Slice((*byte)(a), width))
	copy(y[:width], unsafe.Slice((*byte)(b), width))
	return
}

func X86PackI32ToI16(dst, a, b unsafe.Pointer) {
	x86PackI32(dst, a, b, -32768, 32767)
}

func X86PackI32ToU16(dst, a, b unsafe.Pointer) {
	x86PackI32(dst, a, b, 0, 65535)
}

func x86PackI32(dst, a, b unsafe.Pointer, lo, hi int32) {
	x, y := x86IntegerInputs(a, b, 32)
	var out [32]byte
	for half := 0; half < 32; half += 16 {
		for i := range 4 {
			av := int32(binary.LittleEndian.Uint32(x[half+i*4:]))
			bv := int32(binary.LittleEndian.Uint32(y[half+i*4:]))
			binary.LittleEndian.PutUint16(out[half+i*2:], uint16(max(lo, min(hi, av))))
			binary.LittleEndian.PutUint16(out[half+8+i*2:], uint16(max(lo, min(hi, bv))))
		}
	}
	copy(unsafe.Slice((*byte)(dst), 32), out[:])
}

func X86PackI16ToU8(dst, a, b unsafe.Pointer) {
	x, y := x86IntegerInputs(a, b, 32)
	var out [32]byte
	for half := 0; half < 32; half += 16 {
		for i := range 8 {
			av := int16(binary.LittleEndian.Uint16(x[half+i*2:]))
			bv := int16(binary.LittleEndian.Uint16(y[half+i*2:]))
			out[half+i] = byte(max(0, min(255, av)))
			out[half+8+i] = byte(max(0, min(255, bv)))
		}
	}
	copy(unsafe.Slice((*byte)(dst), 32), out[:])
}

// PMADDWD adds adjacent signed products with 32-bit wrapping. In particular,
// (-32768 * -32768) + (-32768 * -32768) produces 0x80000000.
func X86MaddI16(dst, a, b unsafe.Pointer, width int) {
	x, y := x86IntegerInputs(a, b, width)
	var out [32]byte
	for i := 0; i < width; i += 4 {
		a0 := int32(int16(binary.LittleEndian.Uint16(x[i:])))
		a1 := int32(int16(binary.LittleEndian.Uint16(x[i+2:])))
		b0 := int32(int16(binary.LittleEndian.Uint16(y[i:])))
		b1 := int32(int16(binary.LittleEndian.Uint16(y[i+2:])))
		binary.LittleEndian.PutUint32(out[i:], uint32(a0*b0+a1*b1))
	}
	copy(unsafe.Slice((*byte)(dst), width), out[:width])
}

// PMADDUBSW treats only its first input as unsigned; adjacent products are
// added before signed saturation to 16 bits.
func X86MaddU8I8(dst, a, b unsafe.Pointer, width int) {
	x, y := x86IntegerInputs(a, b, width)
	var out [32]byte
	for i := 0; i < width; i += 2 {
		v := int32(x[i])*int32(int8(y[i])) + int32(x[i+1])*int32(int8(y[i+1]))
		binary.LittleEndian.PutUint16(out[i:], uint16(max(-32768, min(32767, v))))
	}
	copy(unsafe.Slice((*byte)(dst), width), out[:width])
}

func X86SADU8(dst, a, b unsafe.Pointer, width int) {
	x, y := x86IntegerInputs(a, b, width)
	var out [32]byte
	for i := 0; i < width; i += 8 {
		var sum uint64
		for j := range 8 {
			v := int(x[i+j]) - int(y[i+j])
			if v < 0 {
				v = -v
			}
			sum += uint64(v)
		}
		binary.LittleEndian.PutUint64(out[i:], sum)
	}
	copy(unsafe.Slice((*byte)(dst), width), out[:width])
}

func X86ShuffleU8(dst, a, b unsafe.Pointer, width int) {
	x, y := x86IntegerInputs(a, b, width)
	var out [32]byte
	for i := range width {
		if y[i]&0x80 == 0 {
			out[i] = x[(i&^15)+int(y[i]&15)]
		}
	}
	copy(unsafe.Slice((*byte)(dst), width), out[:width])
}

// PSRLD reads the full low 64 bits of the count operand. Counts of 32 or
// greater clear every lane; the high 64 bits do not affect the result.
func X86ShiftRightU32(dst, a, b unsafe.Pointer) {
	x, y := x86IntegerInputs(a, b, 16)
	count := binary.LittleEndian.Uint64(y[:8])
	var out [16]byte
	for i := 0; i < 16; i += 4 {
		binary.LittleEndian.PutUint32(out[i:], binary.LittleEndian.Uint32(x[i:])>>count)
	}
	copy(unsafe.Slice((*byte)(dst), 16), out[:])
}

// PCLMULQDQ multiplies GF(2) polynomials without a reduction modulus. Only
// immediate bits 0 and 4 select the input halves; all other bits are ignored.
// https://clang.llvm.org/doxygen/____wmmintrin__pclmul_8h.html
func X86CarrylessMul64(dst, a, b unsafe.Pointer, immediate byte) {
	x, y := x86IntegerInputs(a, b, 16)
	av := binary.LittleEndian.Uint64(x[int(immediate&1)*8:])
	bv := binary.LittleEndian.Uint64(y[int((immediate>>4)&1)*8:])
	var lo, hi uint64
	for i := uint(0); i < 64; i++ {
		if bv&(uint64(1)<<i) != 0 {
			lo ^= av << i
			hi ^= av >> (64 - i)
		}
	}
	var out [16]byte
	binary.LittleEndian.PutUint64(out[:8], lo)
	binary.LittleEndian.PutUint64(out[8:], hi)
	copy(unsafe.Slice((*byte)(dst), 16), out[:])
}
