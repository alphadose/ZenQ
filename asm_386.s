#include "textflag.h"
#include "go_asm.h"

#define	get_tls(r)	MOVL TLS, r
#define	g(r)	0(r)(TLS*1)

TEXT ·GetG(SB),NOSPLIT,$0-4
	get_tls(CX)
	MOVL	g(CX), AX
	MOVL	AX, gp+0(FP)
	RET
