#include <assert.h>
#include <nrf.h>
#include "nimble/nimble_npl.h"
test_rtc test_rtc0;
test_scb test_scb0;
uint32_t test_primask;
static int runs;
static void callback(struct ble_npl_event *ev) { (void)ev; runs++; gopine_ble_pump(); }
int main(void) {
    struct ble_npl_eventq q = {0};
    struct ble_npl_event ev = {0};
    ble_npl_eventq_init(&q);
    ble_npl_eventq_init(&q); // No duplicate queue-list links.
    ble_npl_event_init(&ev, callback, NULL);
    ble_npl_eventq_put(&q, &ev); ble_npl_eventq_put(&q, &ev);
    gopine_ble_pump();
    assert(runs == 1 && ble_npl_eventq_is_empty(&q));
    ble_npl_eventq_put(&q, &ev); ble_npl_eventq_remove(&q, &ev);
    assert(!ble_npl_event_is_queued(&ev));
    struct ble_npl_callout co = {0};
    ble_npl_callout_init(&co, &q, callback, NULL);
    ble_npl_callout_reset(&co, 1000);
    test_rtc0.COUNTER = 16384; gopine_ble_pump(); assert(runs == 1);
    test_rtc0.COUNTER = 32768; gopine_ble_pump(); assert(runs == 2);
    ble_npl_callout_reset(&co, 1000); ble_npl_callout_stop(&co);
    test_rtc0.COUNTER = 65536; gopine_ble_pump(); assert(runs == 2);
    ble_npl_callout_init(&co, &q, callback, NULL); // Safe reinitialization.
    ble_npl_callout_reset(&co, 0); gopine_ble_pump(); assert(runs == 3);
    struct ble_npl_sem sem;
    ble_npl_sem_init(&sem, 1);
    assert(ble_npl_sem_pend(&sem, 0) == 0);
    assert(ble_npl_sem_pend(&sem, 0) == BLE_NPL_TIMEOUT);
    ble_npl_sem_release(&sem); assert(ble_npl_sem_get_count(&sem) == 1);
    test_primask = 1; ble_npl_eventq_put(&q, &ev); assert(test_primask == 1);
    test_primask = 0; gopine_ble_pump(); assert(runs == 4 && !test_primask);
    test_rtc0.COUNTER = 0xfffff0; uint32_t before = ble_npl_time_get();
    test_rtc0.COUNTER = 0x8010;
    assert(ble_npl_time_get() - before >= 1000); // 24-bit RTC wrap.
}
