//go:build linux && arm64

package oxide

import "unsafe"

func InstructionBarrier()

// These boundaries execute the same ARM instructions as their Rust intrinsics.
// In particular, reciprocal estimates and NaN behavior are not approximated.

//go:noescape
func NeonReciprocalF32x4(dst, a unsafe.Pointer)

//go:noescape
func NeonReciprocalStepF32x4(dst, a, b unsafe.Pointer)

//go:noescape
func NeonMaxF32x4(dst, a, b unsafe.Pointer)

//go:noescape
func NeonMinF32x4(dst, a, b unsafe.Pointer)

//go:noescape
func NeonRoundToI32x4(dst, a unsafe.Pointer)

//go:noescape
func NeonTruncToI32x4(dst, a unsafe.Pointer)

//go:noescape
func NeonRoundEvenF32x4(dst, a unsafe.Pointer)

//go:noescape
func NeonDoubleMulHighI32x2(dst, a, b unsafe.Pointer)

//go:noescape
func NeonDoubleMulHighI32x4(dst, a, b unsafe.Pointer)

//go:noescape
func NeonDoubleMulHighI16x8(dst, a, b unsafe.Pointer)

//go:noescape
func NeonNarrowI32x4(dst, a unsafe.Pointer)

//go:noescape
func NeonNarrowU16x8(dst, a unsafe.Pointer)

//go:noescape
func NeonPairwiseMaxU8x16(dst, a, b unsafe.Pointer)

//go:noescape
func NeonTableU8x16(dst, a, b unsafe.Pointer)

//go:noescape
func NeonShiftNarrowI32x4(dst, a unsafe.Pointer, shift uint32)
