#include <assert.h>
#include <nrf.h>
#include "nimble/nimble_npl.h"
test_rtc test_rtc0;
test_scb test_scb0;
uint32_t test_primask;
static int runs;
static bool command_active;
static int host_runs;
static struct ble_npl_sem ack;
static struct ble_npl_eventq host;
static struct ble_npl_event disconnected;
static void disconnect_callback(struct ble_npl_event *ev) {
    (void)ev;
    assert(!command_active); // Host teardown must not reenter the HCI caller.
    host_runs++;
}
static void controller_callback(struct ble_npl_event *ev) {
    (void)ev;
    // Controller produces command-complete and a disconnect in the same pass.
    ble_npl_eventq_put(&host, &disconnected);
    ble_npl_sem_release(&ack);
}
static void callback(struct ble_npl_event *ev) { (void)ev; runs++; gopine_ble_pump(); }
int main(void) {
    struct ble_npl_eventq q = {0};
    struct ble_npl_event ev = {0};
    ble_npl_eventq_init(&q);
    ble_npl_eventq_init(&q); // No duplicate queue-list links.
    ble_npl_event_init(&ev, callback, NULL);
    ble_npl_eventq_put(&q, &ev); ble_npl_eventq_put(&q, &ev);
    assert(gopine_ble_next_work()==0);
    gopine_ble_pump();
    assert(runs == 1 && ble_npl_eventq_is_empty(&q));
    assert(gopine_ble_next_work()==240000);
    ble_npl_eventq_put(&q, &ev); ble_npl_eventq_remove(&q, &ev);
    assert(!ble_npl_event_is_queued(&ev));
    struct ble_npl_callout co = {0};
    ble_npl_callout_init(&co, &q, callback, NULL);
    ble_npl_callout_reset(&co, 1000);
    test_rtc0.COUNTER = 16384; gopine_ble_pump(); assert(runs == 1);
    assert(gopine_ble_next_work()==500);
    test_rtc0.COUNTER = 32768; assert(gopine_ble_next_work()==0);
    gopine_ble_pump(); assert(runs == 2);
    ble_npl_callout_reset(&co, 1000); ble_npl_callout_stop(&co);
    assert(gopine_ble_next_work()==240000);
    test_rtc0.COUNTER = 65536; gopine_ble_pump(); assert(runs == 2);
    ble_npl_callout_init(&co, &q, callback, NULL); // Safe reinitialization.
    ble_npl_callout_reset(&co, 0); gopine_ble_pump(); assert(runs == 3);
    struct ble_npl_sem sem;
    ble_npl_sem_init(&sem, 1);
    assert(ble_npl_sem_pend(&sem, 0) == 0);
    assert(ble_npl_sem_pend(&sem, 0) == BLE_NPL_TIMEOUT);
    ble_npl_sem_release(&sem); assert(ble_npl_sem_get_count(&sem) == 1);
    test_primask = 1; ble_npl_eventq_put(&q, &ev); assert(test_primask == 1);
    assert(gopine_ble_next_work()==0 && test_primask==1);
    test_primask = 0; gopine_ble_pump(); assert(runs == 4 && !test_primask);
    struct ble_npl_eventq controller = {0};
    struct ble_npl_event command = {0};
    struct ble_npl_callout host_timeout = {0};
    ble_npl_eventq_init(&host);
    ble_npl_eventq_init(&controller);
    gopine_ble_set_host_queue(&host);
    assert(!gopine_ble_host_work_pending());
    ble_npl_event_init(&disconnected, disconnect_callback, NULL);
    ble_npl_event_init(&command, controller_callback, NULL);
    ble_npl_callout_init(&host_timeout, &host, disconnect_callback, NULL);
    ble_npl_sem_init(&ack, 0);
    ble_npl_eventq_put(&host,&disconnected);
    test_primask=1;
    assert(gopine_ble_host_work_pending() && test_primask==1);
    test_primask=0;
    command_active=true;
    gopine_ble_controller_pump();
    assert(host_runs==0 && !ble_npl_eventq_is_empty(&host));
    assert(gopine_ble_host_work_pending()); // Wait must yield, not spin until UI refresh.
    command_active=false;
    gopine_ble_pump();assert(host_runs==1);
    assert(!gopine_ble_host_work_pending());
    ble_npl_callout_reset(&host_timeout,1000);
    assert(!gopine_ble_host_work_pending());
    test_rtc0.COUNTER+=32768;
    assert(gopine_ble_host_work_pending()); // Includes due host timers before pumping.
    ble_npl_callout_stop(&host_timeout);
    assert(!gopine_ble_host_work_pending());
    host_runs=0;
    for (int i=0; i<3; i++) {
        ble_npl_eventq_put(&controller, &command);
        ble_npl_callout_reset(&host_timeout, 0);
        command_active=true;
        assert(ble_npl_sem_pend(&ack, 100)==0);
        assert(host_runs==i*2 && ble_npl_sem_get_count(&ack)==0);
        assert(!ble_npl_eventq_is_empty(&host));
        command_active=false;
        gopine_ble_pump();
        assert(host_runs==(i+1)*2 && ble_npl_eventq_is_empty(&host));
    }
    test_rtc0.COUNTER = 0xfffff0; uint32_t before = ble_npl_time_get();
    test_rtc0.COUNTER = 0x8010;
    assert(ble_npl_time_get() - before >= 1000); // 24-bit RTC wrap.
}
