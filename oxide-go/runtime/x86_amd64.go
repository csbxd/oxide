//go:build linux && amd64

package oxide

import "unsafe"

// These use the original SSE instructions, including their MXCSR rounding,
// NaN, signed-zero and reciprocal-estimate behavior. All pointers address
// sixteen bytes; the destination may alias either input.

//go:noescape
func X86MaxF32x4(dst, a, b unsafe.Pointer)

//go:noescape
func X86MinF32x4(dst, a, b unsafe.Pointer)

//go:noescape
func X86ReciprocalF32x4(dst, a unsafe.Pointer)

//go:noescape
func X86RoundToI32x4(dst, a unsafe.Pointer)

//go:noescape
func X86TruncToI32x4(dst, a unsafe.Pointer)

// X86CompareF32x4 takes a CMPPS/VCMPPS immediate predicate (0..31).
// Predicates 8..31 require AVX support, as in the corresponding Rust intrinsic.
//
//go:noescape
func X86CompareF32x4(dst, a, b unsafe.Pointer, predicate uint8)

func X86Pause()

// X86CPUID queries the actual CPU; callers retain Rust's feature checks before
// invoking target-feature operations. XGETBV likewise requires OSXSAVE support.
func X86CPUID(leaf, subleaf uint32) (eax, ebx, ecx, edx uint32)

func X86Xgetbv(index uint32) uint64
