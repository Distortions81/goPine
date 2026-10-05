#ifndef GOPINE_TEST_NRF_H
#define GOPINE_TEST_NRF_H
#include <stdint.h>
#include <stdlib.h>
typedef struct { uint32_t COUNTER; } test_rtc;
typedef struct { uintptr_t VTOR; } test_scb;
extern test_rtc test_rtc0;
extern test_scb test_scb0;
extern uint32_t test_primask;
#define NRF_RTC0 (&test_rtc0)
#define SCB (&test_scb0)
static inline uint32_t __get_PRIMASK(void) { return test_primask; }
static inline void __disable_irq(void) { test_primask = 1; }
static inline void __set_PRIMASK(uint32_t n) { test_primask = n; }
static inline void __DSB(void) {}
static inline void __ISB(void) {}
static inline void NVIC_SystemReset(void) { abort(); }
#endif
