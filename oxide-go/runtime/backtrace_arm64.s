//go:build linux && arm64

#include "textflag.h"

// R29 points eight bytes below RSP in Go frames. On stepping to the caller,
// its saved FP therefore recovers its SP as previousFP+8.
TEXT ·captureFrames(SB), NOSPLIT|NOFRAME, $0-24
	MOVD dst+0(FP), R0
	MOVD capacity+8(FP), R1
	MOVD R29, R2
	MOVD $0, R5
	// Go 1.27: g starts with stack.lo and stack.hi (runtime/runtime2.go).
	MOVD 0(g), R7
	MOVD 8(g), R8
	SUB $16, R8, R9
loop:
	CMP R1, R5
	BEQ done
	CBZ R2, done
	CMP R7, R2
	BLO done
	CMP R9, R2
	BHI done
	LDP (R2), (R3, R4)
	CMP R2, R3
	BLS done
	CMP R9, R3
	BHI done
	MOVD R4, (R0)
	ADD $8, R3, R6
	SUB R6, R8, R6
	MOVD R6, 8(R0)
	ADD $16, R0
	ADD $1, R5
	MOVD R3, R2
	B loop
done:
	MOVD R5, ret+16(FP)
	RET

TEXT ·frameStackPointer(SB), NOSPLIT|NOFRAME, $0-16
	MOVD 8(g), R0
	MOVD offset+0(FP), R1
	SUB R1, R0, R0
	MOVD R0, ret+8(FP)
	RET

TEXT ·UnwindGetCFA(SB), NOSPLIT|NOFRAME, $0-24
	MOVD frame+8(FP), R0
	MOVD 8(R0), R1
	MOVD 8(g), R0
	SUB R1, R0, R0
	MOVD R0, ret+16(FP)
	RET
