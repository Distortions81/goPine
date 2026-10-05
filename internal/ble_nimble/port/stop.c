// The pinned NimBLE host leaves its termination count nonzero after a stop
// timeout. Reset it for each new stop procedure, so a disconnected/out-of-range
// phone does not poison later sync windows. Keep upstream code and license in
// its original tree; this wrapper is tied to that exact pinned revision.
#define ble_hs_stop gopine_upstream_hs_stop
#include "ble_hs_stop.c"
#undef ble_hs_stop

int ble_hs_stop(struct ble_hs_stop_listener *listener, ble_hs_stop_fn *fn, void *arg) {
    if (ble_hs_enabled_state == BLE_HS_ENABLED_STATE_ON) ble_hs_stop_conn_cnt = 0;
    return gopine_upstream_hs_stop(listener, fn, arg);
}
