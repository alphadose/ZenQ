#include "textflag.h"
#include "go_asm.h"

#define    get_tls(r)    MOVD g, r

TEXT ·GetG(SB),NOSPLIT,$0-4
    get_tls(R1)
    MOVD    R1, gp+0(FP)
    RET
