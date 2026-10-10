#ifndef GOPINE_NPL_H
#define GOPINE_NPL_H
#include <stdint.h>
#include <stdbool.h>
#define BLE_NPL_OS_ALIGNMENT 4
#define BLE_NPL_TIME_FOREVER UINT32_MAX
typedef uint32_t ble_npl_time_t;
typedef int32_t ble_npl_stime_t;
struct ble_npl_event { bool queued; ble_npl_event_fn *fn; void *arg; struct ble_npl_event *next; };
struct ble_npl_eventq { struct ble_npl_event *head,*tail; struct ble_npl_eventq *next; bool busy; };
struct ble_npl_callout { struct ble_npl_event ev; struct ble_npl_eventq *evq; struct ble_npl_callout *next; uint32_t deadline; bool active; };
struct ble_npl_mutex { unsigned depth; };
struct ble_npl_sem { uint16_t count; };
void gopine_ble_pump(void);
void gopine_ble_set_host_queue(struct ble_npl_eventq *q);
uint32_t gopine_ble_next_work(void);
#endif
