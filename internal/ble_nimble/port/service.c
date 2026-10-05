// goPine's cooperative CTS bridge. NimBLE callbacks only copy bounded values;
// the Go UI validates and confirms them outside interrupt context.
#include <string.h>
#include <nrf.h>
#include "nimble/nimble_port.h"
#include "nimble/ble_hci_trans.h"
#include "host/ble_hs.h"
#include "host/ble_hs_id.h"
#include "services/gap/ble_svc_gap.h"
#include "services/gatt/ble_svc_gatt.h"
#include "controller/ble_phy.h"
#include "controller/ble_ll.h"
#include "controller/ble_ll_hci.h"
#include "controller/ble_hw.h"

extern int ble_ll_hci_cmd_rx(uint8_t *, void *);
extern int ble_ll_hci_acl_rx(struct os_mbuf *, void *);
extern void ble_gap_reset_state(int reason);

static bool initialized, synced, window, pending;
static uint16_t connection = BLE_HS_CONN_HANDLE_NONE;
static uint8_t address_type, incoming[10], incoming_len, current[10];
static uint32_t received_at, expires_at;
static uint32_t disconnect_after;
static int failure;
static int init_failure;
static bool host_up, stopping;
static struct ble_hs_stop_listener stop_listener;

static void stopped(int status, void *arg) {
    (void)arg;
    // Also covers an out-of-range peer which never acknowledged disconnect.
    ble_ll_reset();
    // A timed-out peer may still have host-side connection records. Clear
    // them through the same GAP cleanup used after a controller reset.
    ble_gap_reset_state(BLE_HS_ECONTROLLER);
    ble_phy_disable();
    ble_hw_rng_stop();
    ble_phy_rfclk_disable();
    connection = BLE_HS_CONN_HANDLE_NONE;
    synced = host_up = stopping = false;
    if (status) failure = status;
}
void gopine_ble_stop(void);

static int time_access(uint16_t conn, uint16_t attr,
                       struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)conn; (void)attr; (void)arg;
    if (ctx->op == BLE_GATT_ACCESS_OP_READ_CHR) {
        return os_mbuf_append(ctx->om, current, 10) ? BLE_ATT_ERR_INSUFFICIENT_RES : 0;
    }
    if (!window || (int32_t)(ble_npl_time_get() - expires_at) >= 0)
        return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    if (pending) return BLE_ATT_ERR_INSUFFICIENT_RES;
    uint16_t len = OS_MBUF_PKTLEN(ctx->om);
    if (len < 9 || len > 10) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    if (os_mbuf_copydata(ctx->om, 0, len, incoming)) return BLE_ATT_ERR_UNLIKELY;
    incoming_len = len;
    received_at = ble_npl_time_get();
    pending = true;
    return 0; // Transport receipt only, not approval of a clock change.
}

static const struct ble_gatt_svc_def services[] = {
    {.type = BLE_GATT_SVC_TYPE_PRIMARY, .uuid = BLE_UUID16_DECLARE(0x1805),
     .characteristics = (struct ble_gatt_chr_def[]) {
         {.uuid = BLE_UUID16_DECLARE(0x2a2b), .access_cb = time_access,
          .flags = BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_WRITE},
         {0}}},
    {0}
};

static void advertise(void);
static int gap_event(struct ble_gap_event *event, void *arg) {
    (void)arg;
    switch (event->type) {
    case BLE_GAP_EVENT_CONNECT:
        if (!event->connect.status) connection = event->connect.conn_handle;
        else advertise();
        break;
    case BLE_GAP_EVENT_DISCONNECT:
        connection = BLE_HS_CONN_HANDLE_NONE;
        advertise();
        break;
    case BLE_GAP_EVENT_ADV_COMPLETE:
        // The UI owns expiry; do not restart a completed advertisement here.
        break;
    }
    return 0;
}

static void advertise(void) {
    if (!window || pending || !synced || connection != BLE_HS_CONN_HANDLE_NONE) return;
    int32_t remaining = (int32_t)(expires_at - ble_npl_time_get());
    if (remaining <= 0) return;
    struct ble_hs_adv_fields fields = {0};
    static const ble_uuid16_t uuid = BLE_UUID16_INIT(0x1805);
    fields.flags = BLE_HS_ADV_F_DISC_GEN | BLE_HS_ADV_F_BREDR_UNSUP;
    fields.name = (uint8_t *)"InfiniTime"; // InfiniLink discovery filter.
    fields.name_len = 10;
    fields.name_is_complete = 1;
    fields.uuids16 = &uuid;
    fields.num_uuids16 = 1;
    fields.uuids16_is_complete = 1;
    int rc = ble_gap_adv_set_fields(&fields);
    if (rc) { failure = rc; return; }
    struct ble_gap_adv_params params = {0};
    params.conn_mode = BLE_GAP_CONN_MODE_UND;
    params.disc_mode = BLE_GAP_DISC_MODE_GEN;
    params.itvl_min = 160; params.itvl_max = 240;
    failure = ble_gap_adv_start(address_type, NULL, remaining, &params, gap_event, NULL);
}

static void on_sync(void) {
    // Use a separate stable identity from InfiniTime recovery, avoiding stale
    // phone GATT caches when switching firmware. The manufacturer identity
    // remains recognizable, but the low address byte differs from recovery.
    uint8_t address[6];
    uint32_t lo = NRF_FICR->DEVICEADDR[0], hi = NRF_FICR->DEVICEADDR[1];
    memcpy(address, &lo, 4); memcpy(address + 4, &hi, 2);
    address[0] ^= 1; address[5] |= 0xc0;
    failure = ble_hs_id_set_rnd(address);
    address_type = BLE_OWN_ADDR_RANDOM;
    synced = failure == 0;
    advertise();
}
static void on_reset(int reason) { synced = false; failure = reason ? reason : -1; }

int gopine_ble_start(const uint8_t *value, uint32_t window_ms) {
    // Signed deadline arithmetic requires a positive window below 2^31 ms.
    if (!window_ms || window_ms > INT32_MAX) return BLE_HS_EINVAL;
    if (init_failure) return init_failure;
    if (stopping) return BLE_HS_EBUSY;
    if (!initialized) {
        nimble_port_init();
        ble_svc_gap_init();
        ble_svc_gatt_init();
        int rc = ble_gatts_count_cfg(services);
        if (rc) { ble_phy_disable(); ble_hw_rng_stop(); ble_phy_rfclk_disable(); init_failure = rc; return rc; }
        rc = ble_gatts_add_svcs(services);
        if (rc) { ble_phy_disable(); ble_hw_rng_stop(); ble_phy_rfclk_disable(); init_failure = rc; return rc; }
        ble_svc_gap_device_name_set("InfiniTime");
        ble_svc_gap_device_appearance_set(0x00c2); // Sports watch.
        ble_hs_cfg.sync_cb = on_sync;
        ble_hs_cfg.reset_cb = on_reset;
        // These are the initialization steps of NimBLE's ble_ll_task;
        // event queues are subsequently drained by our cooperative pump.
        ble_phy_init();
        ble_phy_txpwr_set(0);
        ble_hci_trans_cfg_ll(ble_ll_hci_cmd_rx, NULL, ble_ll_hci_acl_rx, NULL);
        ble_ll_hci_send_noop();
        initialized = true;
    }
    // With RFMGMT_ENABLE_TIME=0, NimBLE enables HFXO only once during
    // ble_ll_rfmgmt_init(). Our stop path releases it, so every new window
    // must reacquire it before starting the host/controller again.
    ble_phy_rfclk_enable();
    memcpy(current, value, 10);
    pending = false; window = true; failure = 0; disconnect_after = 0;
    expires_at = ble_npl_time_get() + window_ms;
    if (!host_up) { host_up = true; ble_hs_sched_start(); }
    else advertise();
    return failure;
}

void gopine_ble_stop(void) {
    window = false; pending = false;
    if (!initialized || !host_up || stopping) return;
    // Allow the ATT Write Response to leave the controller before teardown.
    // The window is already closed: no new write or reconnection is accepted.
    if (disconnect_after && (int32_t)(ble_npl_time_get() - disconnect_after) < 0) return;
    disconnect_after = 0;
    stopping = true;
    int rc = ble_hs_stop(&stop_listener, stopped, NULL);
    if (rc == BLE_HS_EALREADY) {
        // Start may still be queued. Let it finish, then stop from poll.
        stopping = false;
    } else if (rc) {
        failure = rc;
        stopped(rc, NULL);
    }
}

void gopine_ble_poll(void) {
    if (!initialized) return;
    gopine_ble_pump();
    if (window && (int32_t)(ble_npl_time_get() - expires_at) >= 0) gopine_ble_stop();
    if (!window && host_up && !stopping) gopine_ble_stop();
}

int gopine_ble_take(uint8_t *value, uint32_t *age) {
    if (!pending || !window) return 0;
    memcpy(value, incoming, incoming_len);
    *age = ble_npl_time_get() - received_at;
    // Keep the mailbox locked until Stop/Start. A second sender cannot replace
    // the proposal while the user is reading the confirmation screen.
    window = false;
    disconnect_after = ble_npl_time_get() + 200;
    return incoming_len;
}
int gopine_ble_error(void) { return failure; }
