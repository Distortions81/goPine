#include <stdbool.h>
#include <stdint.h>
#include <nrf.h>
// MCUboot scratch ends at 0x7cfff; clock/settings use 0x7e000..0x7ffff.
#define BOND_BASE UINT32_C(0x7d000)
uint32_t bond_flash_word(unsigned offset) {
    if(offset>=4096 || offset%4) return 0;
    return *(volatile uint32_t*)(BOND_BASE+offset);
}
static void ready(void) {while(!NRF_NVMC->READY) {}}
bool bond_flash_write(unsigned offset, uint32_t word) {
    if(offset>=4096 || offset%4 || (bond_flash_word(offset)&word)!=word)return false;
    // A word write stalls flash for microseconds. Restore interrupts between
    // words; never mask them for a complete snapshot while connected.
    uint32_t mask=__get_PRIMASK(); __disable_irq();
    NRF_NVMC->CONFIG=1; ready();
    *(volatile uint32_t*)(BOND_BASE+offset)=word; ready();
    NRF_NVMC->CONFIG=0; ready(); __set_PRIMASK(mask);
    return bond_flash_word(offset)==word;
}
bool bond_flash_erase(void) {
    NRF_WDT->RR[0]=0x6e524635;
    uint32_t mask=__get_PRIMASK(); __disable_irq();
    NRF_NVMC->CONFIG=2;ready();NRF_NVMC->ERASEPAGE=BOND_BASE;ready();
    NRF_NVMC->CONFIG=0;ready();__set_PRIMASK(mask);
    for(unsigned i=0;i<4096;i+=4)if(bond_flash_word(i)!=UINT32_MAX)return false;
    return true;
}
