package oxide

import (
	"math"
	"math/bits"
	"sync"
	"sync/atomic"
	"unsafe"
)

func CRC32Byte(crc, value uint32) uint32 {
	crc ^= value & 0xff
	for i := 0; i < 8; i++ {
		if crc&1 != 0 {
			crc = crc>>1 ^ 0xedb88320
		} else {
			crc >>= 1
		}
	}
	return crc
}
func CRC32X(crc uint32, value uint64) uint32 {
	for i := 0; i < 8; i++ {
		crc = CRC32Byte(crc, uint32(value>>uint(8*i)))
	}
	return crc
}

func FloatToInt[T Integer](x float64) T {
	if math.IsNaN(x) {
		return 0
	}
	bits := uint(unsafe.Sizeof(T(0)) * 8)
	if ^T(0) > 0 {
		limit := math.Ldexp(1, int(bits))
		if x <= 0 {
			return 0
		}
		if x >= limit {
			return ^T(0)
		}
		return T(x)
	}
	limit := math.Ldexp(1, int(bits-1))
	if x >= limit {
		return ^minValue[T]()
	}
	if x <= -limit {
		return minValue[T]()
	}
	return T(x)
}

// These helpers are the non-allocating implementations of core intrinsics
// used by MIR. Their signatures follow the Rust intrinsic ABI rather than the
// more convenient (and differently typed) Go library APIs.
func LeadingZeros[T Integer](v T) uint32 {
	u := uint64(v)
	switch unsafe.Sizeof(v) {
	case 1:
		return uint32(bits.LeadingZeros8(uint8(u)))
	case 2:
		return uint32(bits.LeadingZeros16(uint16(u)))
	case 4:
		return uint32(bits.LeadingZeros32(uint32(u)))
	default:
		return uint32(bits.LeadingZeros64(u))
	}
}

func TrailingZeros[T Integer](v T) uint32 {
	u := uint64(v)
	switch unsafe.Sizeof(v) {
	case 1:
		return uint32(bits.TrailingZeros8(uint8(u)))
	case 2:
		return uint32(bits.TrailingZeros16(uint16(u)))
	case 4:
		return uint32(bits.TrailingZeros32(uint32(u)))
	default:
		return uint32(bits.TrailingZeros64(u))
	}
}

func OnesCount[T Integer](v T) uint32 {
	u := uint64(v)
	switch unsafe.Sizeof(v) {
	case 1:
		return uint32(bits.OnesCount8(uint8(u)))
	case 2:
		return uint32(bits.OnesCount16(uint16(u)))
	case 4:
		return uint32(bits.OnesCount32(uint32(u)))
	default:
		return uint32(bits.OnesCount64(u))
	}
}

func ByteSwap[T Integer](v T) T {
	u := uint64(v)
	switch unsafe.Sizeof(v) {
	case 1:
		return v
	case 2:
		return T(bits.ReverseBytes16(uint16(u)))
	case 4:
		return T(bits.ReverseBytes32(uint32(u)))
	default:
		return T(bits.ReverseBytes64(u))
	}
}

func BitReverse[T Integer](v T) T {
	return T(bits.Reverse64(uint64(v)) >> (64 - unsafe.Sizeof(v)*8))
}

func RotateLeft[T Integer](v T, n uint32) T {
	u := uint64(v)
	var r uint64
	switch unsafe.Sizeof(v) {
	case 1:
		r = uint64(bits.RotateLeft8(uint8(u), int(n)&7))
	case 2:
		r = uint64(bits.RotateLeft16(uint16(u), int(n)&15))
	case 4:
		r = uint64(bits.RotateLeft32(uint32(u), int(n)&31))
	default:
		r = bits.RotateLeft64(u, int(n)&63)
	}
	return T(r)
}

func RotateRight[T Integer](v T, n uint32) T { return RotateLeft(v, uint32(0)-n) }

func IntegerMin[T Integer](a, b T) T {
	if a < b {
		return a
	}
	return b
}
func IntegerMax[T Integer](a, b T) T {
	if a > b {
		return a
	}
	return b
}
func Select[T any](cond bool, yes, no T) T {
	if cond {
		return yes
	}
	return no
}

func SaturatingAdd[T Integer](a, b T) T {
	r, ov := AddWithOverflow(a, b)
	if !ov {
		return r
	}
	if ^T(0) > 0 {
		return ^T(0)
	}
	if a < 0 {
		return minValue[T]()
	}
	return ^minValue[T]()
}
func SaturatingSub[T Integer](a, b T) T {
	r, ov := SubWithOverflow(a, b)
	if !ov {
		return r
	}
	if ^T(0) > 0 {
		return 0
	}
	if a < 0 {
		return minValue[T]()
	}
	return ^minValue[T]()
}

func MemoryEqual(a, b unsafe.Pointer, n uintptr) bool {
	for i := uintptr(0); i < n; i++ {
		if *(*byte)(unsafe.Add(a, i)) != *(*byte)(unsafe.Add(b, i)) {
			return false
		}
	}
	return true
}
func CompareMemory(a, b unsafe.Pointer, n uintptr) int32 {
	for i := uintptr(0); i < n; i++ {
		x, y := *(*byte)(unsafe.Add(a, i)), *(*byte)(unsafe.Add(b, i))
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func WriteBytes(dst unsafe.Pointer, value byte, n uintptr) {
	for i := uintptr(0); i < n; i++ {
		*(*byte)(unsafe.Add(dst, i)) = value
	}
}

// AtomicFence is a full compiler and hardware barrier for the supported
// targets. Rust's fence intrinsic has no value; SeqCst is the strongest
// mapping and is therefore valid for every weaker ordering at this boundary.
var fenceWord uintptr

func AtomicFence() { atomic.AddUintptr(&fenceWord, 0) }

var narrowAtomicMu sync.Mutex

func AtomicLoad(p unsafe.Pointer, size uintptr) uint64 {
	switch size {
	case 1:
		narrowAtomicMu.Lock()
		defer narrowAtomicMu.Unlock()
		return uint64(*(*uint8)(p))
	case 2:
		narrowAtomicMu.Lock()
		defer narrowAtomicMu.Unlock()
		return uint64(*(*uint16)(p))
	case 4:
		return uint64(atomic.LoadUint32((*uint32)(p)))
	case 8:
		return atomic.LoadUint64((*uint64)(p))
	default:
		panic("Rust atomic width")
	}
}
func AtomicStore(p unsafe.Pointer, v uint64, size uintptr) {
	switch size {
	case 1:
		narrowAtomicMu.Lock()
		*(*uint8)(p) = uint8(v)
		narrowAtomicMu.Unlock()
	case 2:
		narrowAtomicMu.Lock()
		*(*uint16)(p) = uint16(v)
		narrowAtomicMu.Unlock()
	case 4:
		atomic.StoreUint32((*uint32)(p), uint32(v))
	case 8:
		atomic.StoreUint64((*uint64)(p), v)
	default:
		panic("Rust atomic width")
	}
}
func AtomicSwap(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	switch size {
	case 1:
		narrowAtomicMu.Lock()
		old := uint64(*(*uint8)(p))
		*(*uint8)(p) = uint8(v)
		narrowAtomicMu.Unlock()
		return old
	case 2:
		narrowAtomicMu.Lock()
		old := uint64(*(*uint16)(p))
		*(*uint16)(p) = uint16(v)
		narrowAtomicMu.Unlock()
		return old
	case 4:
		return uint64(atomic.SwapUint32((*uint32)(p), uint32(v)))
	case 8:
		return atomic.SwapUint64((*uint64)(p), v)
	default:
		panic("Rust atomic width")
	}
}
func AtomicAdd(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	switch size {
	case 4:
		return uint64(atomic.AddUint32((*uint32)(p), uint32(v)) - uint32(v))
	case 8:
		return atomic.AddUint64((*uint64)(p), v) - v
	default:
		narrowAtomicMu.Lock()
		var old uint64
		if size == 1 {
			old = uint64(*(*uint8)(p))
			*(*uint8)(p) = uint8(old + v)
		} else if size == 2 {
			old = uint64(*(*uint16)(p))
			*(*uint16)(p) = uint16(old + v)
		} else {
			panic("Rust atomic width")
		}
		narrowAtomicMu.Unlock()
		return old
	}
}
func AtomicSub(p unsafe.Pointer, v uint64, size uintptr) uint64 { return AtomicAdd(p, ^v+1, size) }
func AtomicBit(p unsafe.Pointer, v uint64, size uintptr, op byte) uint64 {
	for {
		old := AtomicLoad(p, size)
		var next uint64
		switch op {
		case '&':
			next = old & v
		case '|':
			next = old | v
		case '^':
			next = old ^ v
		default:
			next = ^(old & v)
		}
		if got, ok := AtomicCompareExchange(p, old, next, size); ok {
			return got
		}
	}
}
func AtomicAnd(p unsafe.Pointer, v uint64, size uintptr) uint64  { return AtomicBit(p, v, size, '&') }
func AtomicOr(p unsafe.Pointer, v uint64, size uintptr) uint64   { return AtomicBit(p, v, size, '|') }
func AtomicXor(p unsafe.Pointer, v uint64, size uintptr) uint64  { return AtomicBit(p, v, size, '^') }
func AtomicNand(p unsafe.Pointer, v uint64, size uintptr) uint64 { return AtomicBit(p, v, size, '~') }
func AtomicMin(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	return atomicMinMax(p, v, size, true, false)
}
func AtomicMax(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	return atomicMinMax(p, v, size, false, false)
}
func AtomicUMin(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	return atomicMinMax(p, v, size, true, true)
}
func AtomicUMax(p unsafe.Pointer, v uint64, size uintptr) uint64 {
	return atomicMinMax(p, v, size, false, true)
}
func atomicMinMax(p unsafe.Pointer, v uint64, size uintptr, min, uns bool) uint64 {
	v &= ^uint64(0) >> (64 - size*8)
	for {
		old := AtomicLoad(p, size)
		var less bool
		if uns {
			less = old < v
		} else {
			bits := size * 8
			mask := uint64(1) << (bits - 1)
			oa, va := old^mask, v^mask
			less = oa < va
		}
		replace := less
		if min {
			replace = !less && old != v
		}
		next := old
		if replace {
			next = v
		}
		if got, ok := AtomicCompareExchange(p, old, next, size); ok {
			return got
		}
	}
}
func AtomicCompareExchange(p unsafe.Pointer, old, next uint64, size uintptr) (uint64, bool) {
	if size < 8 {
		old &= (uint64(1) << (size * 8)) - 1
	}
	switch size {
	case 4:
		for {
			got := atomic.LoadUint32((*uint32)(p))
			if got != uint32(old) {
				return uint64(got), false
			}
			if atomic.CompareAndSwapUint32((*uint32)(p), uint32(old), uint32(next)) {
				return uint64(got), true
			}
		}
	case 8:
		for {
			got := atomic.LoadUint64((*uint64)(p))
			if got != old {
				return got, false
			}
			if atomic.CompareAndSwapUint64((*uint64)(p), old, next) {
				return got, true
			}
		}
	default:
		narrowAtomicMu.Lock()
		var got uint64
		if size == 1 {
			got = uint64(*(*uint8)(p))
			if got == old {
				*(*uint8)(p) = uint8(next)
			}
		} else if size == 2 {
			got = uint64(*(*uint16)(p))
			if got == old {
				*(*uint16)(p) = uint16(next)
			}
		} else {
			panic("Rust atomic width")
		}
		narrowAtomicMu.Unlock()
		return got, got == old
	}
}
