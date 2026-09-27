//go:build linux && amd64

#include "textflag.h"

TEXT ·X86MaxF32x4(SB), NOSPLIT, $0-24
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVQ b+16(FP), DX
	MOVUPS (CX), X0
	MOVUPS (DX), X1
	MAXPS X1, X0
	MOVUPS X0, (AX)
	RET

TEXT ·X86MinF32x4(SB), NOSPLIT, $0-24
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVQ b+16(FP), DX
	MOVUPS (CX), X0
	MOVUPS (DX), X1
	MINPS X1, X0
	MOVUPS X0, (AX)
	RET

TEXT ·X86ReciprocalF32x4(SB), NOSPLIT, $0-16
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVUPS (CX), X0
	RCPPS X0, X0
	MOVUPS X0, (AX)
	RET

TEXT ·X86RoundToI32x4(SB), NOSPLIT, $0-16
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVUPS (CX), X0
	CVTPS2PL X0, X0
	MOVUPS X0, (AX)
	RET

TEXT ·X86TruncToI32x4(SB), NOSPLIT, $0-16
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVUPS (CX), X0
	CVTTPS2PL X0, X0
	MOVUPS X0, (AX)
	RET

TEXT ·X86CompareF32x4(SB), NOSPLIT, $0-25
	MOVQ dst+0(FP), AX
	MOVQ a+8(FP), CX
	MOVQ b+16(FP), DX
	MOVUPS (CX), X0
	MOVUPS (DX), X1
	MOVBLZX predicate+24(FP), CX
	CMPL CX, $7
	JA cmpavx
	CMPL CX, $0
	JE cmp0
	CMPL CX, $1
	JE cmp1
	CMPL CX, $2
	JE cmp2
	CMPL CX, $3
	JE cmp3
	CMPL CX, $4
	JE cmp4
	CMPL CX, $5
	JE cmp5
	CMPL CX, $6
	JE cmp6
	CMPPS X1, X0, $7
	JMP cmpstore
cmp0:
	CMPPS X1, X0, $0
	JMP cmpstore
cmp1:
	CMPPS X1, X0, $1
	JMP cmpstore
cmp2:
	CMPPS X1, X0, $2
	JMP cmpstore
cmp3:
	CMPPS X1, X0, $3
	JMP cmpstore
cmp4:
	CMPPS X1, X0, $4
	JMP cmpstore
cmp5:
	CMPPS X1, X0, $5
	JMP cmpstore
cmp6:
	CMPPS X1, X0, $6
cmpstore:
	MOVUPS X0, (AX)
	RET

// Predicates 8..31 require AVX, matching stdarch _mm_cmp_ps.
cmpavx:
	CMPL CX, $8
	JE cmp8
	CMPL CX, $9
	JE cmp9
	CMPL CX, $10
	JE cmp10
	CMPL CX, $11
	JE cmp11
	CMPL CX, $12
	JE cmp12
	CMPL CX, $13
	JE cmp13
	CMPL CX, $14
	JE cmp14
	CMPL CX, $15
	JE cmp15
	CMPL CX, $16
	JE cmp16
	CMPL CX, $17
	JE cmp17
	CMPL CX, $18
	JE cmp18
	CMPL CX, $19
	JE cmp19
	CMPL CX, $20
	JE cmp20
	CMPL CX, $21
	JE cmp21
	CMPL CX, $22
	JE cmp22
	CMPL CX, $23
	JE cmp23
	CMPL CX, $24
	JE cmp24
	CMPL CX, $25
	JE cmp25
	CMPL CX, $26
	JE cmp26
	CMPL CX, $27
	JE cmp27
	CMPL CX, $28
	JE cmp28
	CMPL CX, $29
	JE cmp29
	CMPL CX, $30
	JE cmp30
	VCMPPS $31, X1, X0, X0
	JMP cmpavxstore
cmp8:
	VCMPPS $8, X1, X0, X0
	JMP cmpavxstore
cmp9:
	VCMPPS $9, X1, X0, X0
	JMP cmpavxstore
cmp10:
	VCMPPS $10, X1, X0, X0
	JMP cmpavxstore
cmp11:
	VCMPPS $11, X1, X0, X0
	JMP cmpavxstore
cmp12:
	VCMPPS $12, X1, X0, X0
	JMP cmpavxstore
cmp13:
	VCMPPS $13, X1, X0, X0
	JMP cmpavxstore
cmp14:
	VCMPPS $14, X1, X0, X0
	JMP cmpavxstore
cmp15:
	VCMPPS $15, X1, X0, X0
	JMP cmpavxstore
cmp16:
	VCMPPS $16, X1, X0, X0
	JMP cmpavxstore
cmp17:
	VCMPPS $17, X1, X0, X0
	JMP cmpavxstore
cmp18:
	VCMPPS $18, X1, X0, X0
	JMP cmpavxstore
cmp19:
	VCMPPS $19, X1, X0, X0
	JMP cmpavxstore
cmp20:
	VCMPPS $20, X1, X0, X0
	JMP cmpavxstore
cmp21:
	VCMPPS $21, X1, X0, X0
	JMP cmpavxstore
cmp22:
	VCMPPS $22, X1, X0, X0
	JMP cmpavxstore
cmp23:
	VCMPPS $23, X1, X0, X0
	JMP cmpavxstore
cmp24:
	VCMPPS $24, X1, X0, X0
	JMP cmpavxstore
cmp25:
	VCMPPS $25, X1, X0, X0
	JMP cmpavxstore
cmp26:
	VCMPPS $26, X1, X0, X0
	JMP cmpavxstore
cmp27:
	VCMPPS $27, X1, X0, X0
	JMP cmpavxstore
cmp28:
	VCMPPS $28, X1, X0, X0
	JMP cmpavxstore
cmp29:
	VCMPPS $29, X1, X0, X0
	JMP cmpavxstore
cmp30:
	VCMPPS $30, X1, X0, X0
	JMP cmpavxstore
cmpavxstore:
	MOVUPS X0, (AX)
	VZEROUPPER
	RET

TEXT ·X86Pause(SB), NOSPLIT, $0-0
	PAUSE
	RET

TEXT ·X86CPUID(SB), NOSPLIT, $0-24
	MOVL leaf+0(FP), AX
	MOVL subleaf+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

TEXT ·X86Xgetbv(SB), NOSPLIT, $0-16
	MOVL index+0(FP), CX
	XGETBV
	SHLQ $32, DX
	ORQ DX, AX
	MOVQ AX, ret+8(FP)
	RET
