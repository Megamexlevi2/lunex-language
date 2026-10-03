#include "textflag.h"

TEXT ·callNativeEntry(SB), NOSPLIT, $0-16
    MOVD code+0(FP), R1
    MOVD ctx+8(FP), R0
    JMP (R1)
