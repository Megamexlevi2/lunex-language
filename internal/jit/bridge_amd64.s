#include "textflag.h"

TEXT ·callNativeEntry(SB), NOSPLIT, $0-16
    MOVQ code+0(FP), AX
    MOVQ ctx+8(FP), R10
    JMP AX
