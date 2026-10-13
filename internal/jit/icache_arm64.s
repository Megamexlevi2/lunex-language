#include "textflag.h"

TEXT ·icacheSyncRange(SB), NOSPLIT, $0-16
    MOVD addr+0(FP), R0
    MOVD length+8(FP), R1
    CBZ  R1, done
    ADD  R0, R1, R2
    MOVD R0, R3

clean_loop:
    WORD $0xD50B7B23
    ADD  $64, R3, R3
    CMP  R2, R3
    BLO  clean_loop

    WORD $0xD5033BBF

    MOVD R0, R3
inval_loop:
    WORD $0xD50B7523
    ADD  $64, R3, R3
    CMP  R2, R3
    BLO  inval_loop

    WORD $0xD5033BBF
    WORD $0xD5033FDF

done:
    RET
