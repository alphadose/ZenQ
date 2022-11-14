#include "textflag.h"
#include "go_asm.h"

TEXT ·GetG(SB), NOSPLIT, $0-8
    MOVD    g, R8
    MOVD    R8, ret+0(FP)
    RET
