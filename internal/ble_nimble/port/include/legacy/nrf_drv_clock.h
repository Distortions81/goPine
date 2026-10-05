#ifndef GOPINE_CLOCK_H
#define GOPINE_CLOCK_H
#include <nrf.h>
static inline void nrf_drv_clock_hfclk_request(void *unused) {
    (void)unused;
    NRF_CLOCK->TASKS_HFCLKSTART=1;
    while ((NRF_CLOCK->HFCLKSTAT & (CLOCK_HFCLKSTAT_STATE_Msk | CLOCK_HFCLKSTAT_SRC_Msk)) !=
           (CLOCK_HFCLKSTAT_STATE_Msk | (CLOCK_HFCLKSTAT_SRC_Xtal << CLOCK_HFCLKSTAT_SRC_Pos))) {}
}
// Audited TinyGo 0.42.0 PineTime SPI/runtime do not request HFXO. They use the
// automatically managed HF clock; stopping the radio's crystal request does
// not stop HFCLK for the CPU/SPI. Re-audit if another HFXO consumer is added.
static inline void nrf_drv_clock_hfclk_release(void) { NRF_CLOCK->TASKS_HFCLKSTOP=1; }
#endif
