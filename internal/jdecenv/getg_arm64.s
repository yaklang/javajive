#include "textflag.h"

// func getg() unsafe.Pointer
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD g, R8
	MOVD R8, ret+0(FP)
	RET
