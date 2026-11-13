#include "textflag.h"

// func currentG() unsafe.Pointer
//
// On arm64 the runtime keeps the current goroutine pointer in the dedicated g
// register (R28). Reading it here avoids formatting the whole goroutine stack
// through runtime.Stack on every call; the returned pointer is a stable,
// unique identity for the lifetime of the goroutine, exactly like the amd64
// implementation.
TEXT ·currentG(SB),NOSPLIT,$0-8
	MOVD g, R0
	MOVD R0, ret+0(FP)
	RET
