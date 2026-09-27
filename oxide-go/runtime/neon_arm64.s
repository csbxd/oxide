//go:build linux && arm64

#include "textflag.h"

TEXT ·InstructionBarrier(SB), NOSPLIT, $0-0
	WORD $0xd5033fdf // isb sy
	RET

TEXT ·NeonReciprocalF32x4(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x4ea1d800 // frecpe v0.4s, v0.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonReciprocalStepF32x4(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4e21fc00 // frecps v0.4s, v0.4s, v1.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonMaxF32x4(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4e21f400 // fmax v0.4s, v0.4s, v1.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonMinF32x4(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4ea1f400 // fmin v0.4s, v0.4s, v1.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonRoundToI32x4(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x4e21a800 // fcvtns v0.4s, v0.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonTruncToI32x4(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x4ea1b800 // fcvtzs v0.4s, v0.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonRoundEvenF32x4(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x4e218800 // frintn v0.4s, v0.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonDoubleMulHighI32x2(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D1]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D1]
	WORD $0x0ea1b400 // sqdmulh v0.2s, v0.2s, v1.2s
	VST1 [V0.D1], (R0)
	RET

TEXT ·NeonDoubleMulHighI32x4(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4ea1b400 // sqdmulh v0.4s, v0.4s, v1.4s
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonDoubleMulHighI16x8(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4e61b400 // sqdmulh v0.8h, v0.8h, v1.8h
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonNarrowI32x4(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x0e614800 // sqxtn v0.4h, v0.4s
	VST1 [V0.D1], (R0)
	RET

TEXT ·NeonNarrowU16x8(SB), NOSPLIT, $0-16
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	WORD $0x2e214800 // uqxtn v0.8b, v0.8h
	VST1 [V0.D1], (R0)
	RET

TEXT ·NeonPairwiseMaxU8x16(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x6e21a400 // umaxp v0.16b, v0.16b, v1.16b
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonTableU8x16(SB), NOSPLIT, $0-24
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	VLD1 (R1), [V0.D2]
	MOVD b+16(FP), R2
	VLD1 (R2), [V1.D2]
	WORD $0x4e010000 // tbl v0.16b, {v0.16b}, v1.16b
	VST1 [V0.D2], (R0)
	RET

TEXT ·NeonShiftNarrowI32x4(SB), NOSPLIT, $0-20
	MOVD dst+0(FP), R0
	MOVD a+8(FP), R1
	MOVWU shift+16(FP), R2
	VLD1 (R1), [V0.D2]
	WORD $0x4b0203e2 // neg w2, w2
	WORD $0x4e040c41 // dup v1.4s, w2
	WORD $0x4ea14400 // sshl v0.4s, v0.4s, v1.4s
	WORD $0x2e612800 // sqxtun v0.4h, v0.4s
	VST1 [V0.D1], (R0)
	RET
