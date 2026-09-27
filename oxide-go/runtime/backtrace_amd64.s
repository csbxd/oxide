//go:build linux && amd64

#include "textflag.h"

// The saved BP and return PC occupy the two words below the caller's SP.
TEXT ·captureFrames(SB), NOSPLIT|NOFRAME, $0-24
	MOVQ dst+0(FP), AX
	MOVQ capacity+8(FP), CX
	MOVQ BP, DX
	XORQ R8, R8
	// Go 1.27: g starts with stack.lo and stack.hi (runtime/runtime2.go).
	MOVQ (TLS), R9
	MOVQ 0(R9), R10
	MOVQ 8(R9), R9
	LEAQ -16(R9), R11
loop:
	CMPQ R8, CX
	JE done
	TESTQ DX, DX
	JE done
	CMPQ DX, R10
	JB done
	CMPQ DX, R11
	JA done
	MOVQ (DX), SI
	CMPQ SI, DX
	JBE done
	CMPQ SI, R11
	JA done
	MOVQ 8(DX), DI
	MOVQ DI, (AX)
	LEAQ 16(DX), DI
	NEGQ DI
	ADDQ R9, DI
	MOVQ DI, 8(AX)
	ADDQ $16, AX
	INCQ R8
	MOVQ SI, DX
	JMP loop
done:
	MOVQ R8, ret+16(FP)
	RET

TEXT ·frameStackPointer(SB), NOSPLIT|NOFRAME, $0-16
	MOVQ (TLS), AX
	MOVQ 8(AX), AX
	SUBQ offset+0(FP), AX
	MOVQ AX, ret+8(FP)
	RET

TEXT ·UnwindGetCFA(SB), NOSPLIT|NOFRAME, $0-24
	MOVQ frame+8(FP), CX
	MOVQ (TLS), AX
	MOVQ 8(AX), AX
	SUBQ 8(CX), AX
	MOVQ AX, ret+16(FP)
	RET
