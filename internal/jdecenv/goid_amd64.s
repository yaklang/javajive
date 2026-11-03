#include "textflag.h"

// func currentG() unsafe.Pointer
TEXT ·currentG(SB),NOSPLIT,$0-8
	MOVQ g, AX
	MOVQ AX, ret+0(FP)
	RET
