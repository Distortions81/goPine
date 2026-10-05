/* InfiniTime bootloader 1.0.1: 32-byte header at 0x8000, vectors at 0x8020.
 * Reserve the final slot sector for swap status/trailer and 40 bytes for SHA.
 */
MEMORY
{
    FLASH_TEXT (rw) : ORIGIN = 0x00008020, LENGTH = 0x73000 - 0x20 - 0x28
    RAM (xrw)       : ORIGIN = 0x20000000, LENGTH = 64K
}

_stack_size = 2K;

INCLUDE "targets/arm.ld"
