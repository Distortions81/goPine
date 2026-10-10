// goPine's cooperative CTS bridge. NimBLE callbacks only copy bounded values;
// the Go UI validates them outside interrupt context. Automatic clock writes
// require an authenticated bonded connection; one-shot sync asks for approval.
#include <string.h>
#include <nrf.h>
#include "nimble/nimble_port.h"
#include "nimble/ble_hci_trans.h"
#include "host/ble_hs.h"
#include "host/ble_hs_id.h"
#include "host/ble_sm.h"
#include "services/gap/ble_svc_gap.h"
#include "services/gatt/ble_svc_gatt.h"
#include "controller/ble_phy.h"
#include "controller/ble_ll.h"
#include "controller/ble_ll_hci.h"
#include "controller/ble_hw.h"
#include "weather_mailbox.h"
#include "music_mailbox.h"
#include "notification_mailbox.h"
#include "update_mailbox.h"
#include "battery_security.h"
#include "phone_time.h"
#include "ancs.h"

extern int ble_ll_hci_cmd_rx(uint8_t *, void *);
extern int ble_ll_hci_acl_rx(struct os_mbuf *, void *);
extern void ble_gap_reset_state(int reason);

static bool initialized, synced, window, pending;
static uint16_t connection = BLE_HS_CONN_HANDLE_NONE;
static uint8_t address_type, incoming[10], incoming_len, current[10], battery_level;
static uint16_t battery_handle;
static uint32_t received_at, expires_at;
static uint32_t disconnect_after;
static uint32_t fast_until;
static int failure;
static int init_failure;
static bool host_up, stopping, stop_complete;
static bool lifecycle_changed;
static bool weather_window, phone_window, unbounded, slow_advertising;
static bool music_subscribed;
static uint16_t music_handle;
static uint32_t peer_generation;
static struct music_mailbox music_incoming;
static struct weather_mailbox weather_incoming;
static struct notification_mailbox notification_incoming;
static struct update_mailbox update_incoming;
static struct phone_time_mailbox phone_time_incoming;
static bool update_window;
static uint32_t pairing_code; // Code + 1; zero means no active prompt.
static bool pairing_io_pending, security_pending;
static uint16_t pairing_connection;
static uint16_t last_security_status, last_disconnect_reason;
static uint16_t pairing_progress;
extern uint16_t gopine_sm_progress(uint16_t conn_handle);
extern void gopine_bond_init(void);
extern bool gopine_bond_prepare(uint8_t battery);
extern void gopine_bond_poll(bool stopped);
extern int gopine_bond_status(void);
extern bool gopine_bond_exists(void);
extern uint32_t gopine_bond_delay(void);
extern bool gopine_bond_forget(void);
extern int gopine_ble_restore_privacy(void);
static uint8_t update_status[16];
static struct ble_hs_stop_listener stop_listener;

static bool expired(void) {
    return !unbounded && (int32_t)(ble_npl_time_get()-expires_at)>=0;
}
static void stopped(int status, void *arg) {
    (void)arg;
    if (status) failure = status;
    stop_complete = true;
}
static void finish_stop(void) {
    // The completion callback can run inside the host's disconnect dispatch.
    // Reset only after that dispatch and all nested controller work unwind.
    if (!stop_complete) return;
    stop_complete = false;
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
    lifecycle_changed = true;
}
void gopine_ble_stop(void);

static int time_access(uint16_t conn, uint16_t attr,
                       struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)attr; (void)arg;
    if (ctx->op == BLE_GATT_ACCESS_OP_READ_CHR) {
        return os_mbuf_append(ctx->om, current, 10) ? BLE_ATT_ERR_INSUFFICIENT_RES : 0;
    }
    if (!window || update_window || expired())
        return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    if (phone_window) {
        if (conn != connection) return BLE_ATT_ERR_UNLIKELY;
        uint8_t value[10];
        unsigned len = OS_MBUF_PKTLEN(ctx->om);
        if (len < 9 || len > sizeof(value)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
        if (os_mbuf_copydata(ctx->om, 0, len, value)) return BLE_ATT_ERR_UNLIKELY;
        return phone_time_receive(&phone_time_incoming, conn, peer_generation,
                                  value, len, ble_npl_time_get());
    }
    if (pending) return BLE_ATT_ERR_INSUFFICIENT_RES;
    uint16_t len = OS_MBUF_PKTLEN(ctx->om);
    if (len < 9 || len > 10) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    if (os_mbuf_copydata(ctx->om, 0, len, incoming)) return BLE_ATT_ERR_UNLIKELY;
    // Companions commonly send CTS before weather. A transport ACK must not
    // close this window or silently apply time without on-watch approval.
    if (weather_window) return 0;
    incoming_len = len;
    received_at = ble_npl_time_get();
    pending = true;
    return 0; // Transport receipt only, not approval of a clock change.
}

static int battery_access(uint16_t conn, uint16_t attr,
                          struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)attr; (void)arg;
    return battery_read_access(conn,ctx,window,update_window,battery_level);
}

void gopine_ble_update_battery(uint8_t value) {
    if(value>100) value=100;
    if(value==battery_level) return;
    battery_level=value;
    // Uses the last UI sample, never an additional ADC read or sleep poll.
    // NimBLE coalesces updates and only notifies subscribed peers.
    if(window && connection!=BLE_HS_CONN_HANDLE_NONE)
        ble_gatts_chr_updated(battery_handle);
}

static int weather_access(uint16_t conn, uint16_t attr,
                          struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)attr; (void)arg;
    if (ctx->op != BLE_GATT_ACCESS_OP_WRITE_CHR || !window || !(weather_window || phone_window) || expired()) return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    if (phone_window) {
        int rc = phone_link_permission(conn);
        if (rc) return rc;
    }
    uint16_t len = OS_MBUF_PKTLEN(ctx->om);
    uint8_t data[53];
    if (len > sizeof(data)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    if (os_mbuf_copydata(ctx->om, 0, len, data)) return BLE_ATT_ERR_UNLIKELY;
    if (!weather_store(&weather_incoming, data, len)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    disconnect_after = ble_npl_time_get() + 200; // Preserve the last ATT ACK.
    return 0;
}

static const ble_uuid128_t weather_uuid = BLE_UUID128_INIT(
    0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,0x00,0x00,0x05,0x00);
static const ble_uuid128_t weather_data_uuid = BLE_UUID128_INIT(
    0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,0x01,0x00,0x05,0x00);

// Protocol declarations, independently implemented from InfiniTime's UUIDs.
#define MUSIC_UUID(id) BLE_UUID128_INIT(0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,id,0,0,0)
static const ble_uuid128_t music_uuids[] = {
    MUSIC_UUID(0), MUSIC_UUID(1), MUSIC_UUID(2), MUSIC_UUID(3),
    MUSIC_UUID(4), MUSIC_UUID(5), MUSIC_UUID(6), MUSIC_UUID(7),
    MUSIC_UUID(8), MUSIC_UUID(9), MUSIC_UUID(10), MUSIC_UUID(11), MUSIC_UUID(12)
};
static int music_access(uint16_t conn, uint16_t attr,
                        struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)attr;
    unsigned id=(uintptr_t)arg, len;
    uint8_t *field=music_field(&music_incoming,id,&len);
    if(!field) return BLE_ATT_ERR_UNLIKELY;
    if(ctx->op==BLE_GATT_ACCESS_OP_READ_CHR)
        return os_mbuf_append(ctx->om,field,len)?BLE_ATT_ERR_INSUFFICIENT_RES:0;
    if(ctx->op!=BLE_GATT_ACCESS_OP_WRITE_CHR || !window || !phone_window || expired())
        return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    int rc = phone_link_permission(conn);
    if(rc) return rc;
    unsigned size=OS_MBUF_PKTLEN(ctx->om);
    uint8_t data[40];
    if(os_mbuf_copydata(ctx->om,0,size>40?40:size,data)) return BLE_ATT_ERR_UNLIKELY;
    if(!music_store(&music_incoming,id,data,size)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    disconnect_after=ble_npl_time_get()+200;
    return 0;
}
#define MUSIC_CHAR(id) {.uuid=&music_uuids[id].u, .access_cb=music_access, .arg=(void *)(uintptr_t)id, .flags=BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_WRITE}

static int notification_access(uint16_t conn, uint16_t attr,
                               struct ble_gatt_access_ctxt *ctx, void *arg) {
    (void)attr; (void)arg;
    if(ctx->op!=BLE_GATT_ACCESS_OP_WRITE_CHR || !window || !phone_window || expired())
        return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    int rc = phone_link_permission(conn);
    if(rc) return rc;
    unsigned size=OS_MBUF_PKTLEN(ctx->om);
    if(size<4) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    uint8_t data[NOTIFICATION_PACKET_SIZE];
    unsigned used=size>sizeof(data)?sizeof(data):size;
    if(os_mbuf_copydata(ctx->om,0,used,data)) return BLE_ATT_ERR_UNLIKELY;
    if(size>used) memcpy(data+used-3,"...",3);
    if(!notification_store(&notification_incoming,data,used)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    disconnect_after=ble_npl_time_get()+200;
    return 0;
}

// goPine foreground update service; no command can activate an image remotely.
static const ble_uuid128_t update_uuids[]={
    BLE_UUID128_INIT(0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,0,0,6,0),
    BLE_UUID128_INIT(0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,1,0,6,0),
    BLE_UUID128_INIT(0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,2,0,6,0),
    BLE_UUID128_INIT(0xd0,0x42,0x19,0x3a,0x3b,0x43,0x23,0x8e,0xfe,0x48,0xfc,0x78,3,0,6,0)
};
static int update_access(uint16_t conn,uint16_t attr,struct ble_gatt_access_ctxt *ctx,void *arg) {
    (void)conn;(void)attr;
    if(ctx->op==BLE_GATT_ACCESS_OP_READ_CHR && (uintptr_t)arg==3) {
        update_status[3]=connection==BLE_HS_CONN_HANDLE_NONE?0:(uint8_t)(ble_att_mtu(connection)-3);
        return os_mbuf_append(ctx->om,update_status,16)?BLE_ATT_ERR_INSUFFICIENT_RES:0;
    }
    if(ctx->op!=BLE_GATT_ACCESS_OP_WRITE_CHR || !window || !update_window || expired()) return BLE_ATT_ERR_WRITE_NOT_PERMITTED;
    unsigned n=OS_MBUF_PKTLEN(ctx->om);
    uint8_t data[200];
    if(n>sizeof(data)) return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    if(os_mbuf_copydata(ctx->om,0,n,data))return BLE_ATT_ERR_UNLIKELY;
    int rc=update_store(&update_incoming,(uintptr_t)arg,data,n);
    if(rc<0)return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    if(!rc)return BLE_ATT_ERR_INSUFFICIENT_RES;
    return 0;
}

static const struct ble_gatt_svc_def services[] = {
    {.type=BLE_GATT_SVC_TYPE_PRIMARY,.uuid=&update_uuids[0].u,
     .characteristics=(struct ble_gatt_chr_def[]){
         {.uuid=&update_uuids[1].u,.access_cb=update_access,.arg=(void*)1,.flags=BLE_GATT_CHR_F_WRITE},
         {.uuid=&update_uuids[2].u,.access_cb=update_access,.arg=(void*)2,.flags=BLE_GATT_CHR_F_WRITE},
         {.uuid=&update_uuids[3].u,.access_cb=update_access,.arg=(void*)3,.flags=BLE_GATT_CHR_F_READ}, {0}}},
    {.type=BLE_GATT_SVC_TYPE_PRIMARY, .uuid=BLE_UUID16_DECLARE(0x1811),
     .characteristics=(struct ble_gatt_chr_def[]) {
         {.uuid=BLE_UUID16_DECLARE(0x2a46), .access_cb=notification_access,
          .flags=BLE_GATT_CHR_F_WRITE}, {0}}},
    {.type = BLE_GATT_SVC_TYPE_PRIMARY, .uuid = BLE_UUID16_DECLARE(0x1805),
     .characteristics = (struct ble_gatt_chr_def[]) {
         {.uuid = BLE_UUID16_DECLARE(0x2a2b), .access_cb = time_access,
          .flags = BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_WRITE},
         {0}}},
    {.type = BLE_GATT_SVC_TYPE_PRIMARY, .uuid = BLE_UUID16_DECLARE(0x180f),
     .characteristics = (struct ble_gatt_chr_def[]) {
         {.uuid = BLE_UUID16_DECLARE(0x2a19), .access_cb = battery_access,
          .flags = GOPINE_BATTERY_FLAGS,
          .val_handle = &battery_handle},
         {0}}},
    {.type = BLE_GATT_SVC_TYPE_PRIMARY, .uuid = &weather_uuid.u,
     .characteristics = (struct ble_gatt_chr_def[]) {
         {.uuid = &weather_data_uuid.u, .access_cb = weather_access,
          .flags = BLE_GATT_CHR_F_WRITE},
         {0}}},
    {.type=BLE_GATT_SVC_TYPE_PRIMARY, .uuid=&music_uuids[0].u,
     .characteristics=(struct ble_gatt_chr_def[]) {
         {.uuid=&music_uuids[1].u, .access_cb=music_access,
          .flags=BLE_GATT_CHR_F_NOTIFY, .val_handle=&music_handle},
         MUSIC_CHAR(2), MUSIC_CHAR(3), MUSIC_CHAR(4), MUSIC_CHAR(5),
         MUSIC_CHAR(6), MUSIC_CHAR(7), MUSIC_CHAR(8), MUSIC_CHAR(9),
         MUSIC_CHAR(10), MUSIC_CHAR(11), MUSIC_CHAR(12), {0}}},
    {0}
};

static void advertise(void);
static int gap_event(struct ble_gap_event *event, void *arg) {
    (void)arg;
    switch (event->type) {
    case BLE_GAP_EVENT_PASSKEY_ACTION: {
        if(event->passkey.params.action!=BLE_SM_IOACT_DISP || !window ||
           update_window || gopine_bond_exists()) {
            ble_gap_terminate(event->passkey.conn_handle,BLE_ERR_REM_USER_CONN_TERM);
            break;
        }
        // Unwind the security state machine before injecting application IO.
        // This avoids nesting a second SM execution under its own callback.
        pairing_connection=event->passkey.conn_handle; pairing_io_pending=true;
        break;
    }
    case BLE_GAP_EVENT_ENC_CHANGE:
        last_security_status=event->enc_change.status;
        if(!event->enc_change.status)last_disconnect_reason=0;
        if(!event->enc_change.status && phone_link_ready(connection))
            pairing_progress=6u<<8;
        pairing_code=0;pairing_io_pending=security_pending=false;lifecycle_changed=true;
        // The central may subscribe before pairing finishes. Publish the
        // ready state only after authentication, even if CCCD never changes.
        music_incoming.dirty=true;
        if(event->enc_change.status || !phone_link_ready(connection)) {
            gopine_ancs_reset();
            phone_time_clear(&phone_time_incoming);
            if(phone_window) {
                music_clear(&music_incoming);
                memset(&weather_incoming,0,sizeof(weather_incoming));
                memset(&notification_incoming,0,sizeof(notification_incoming));
            }
        }
        break;
    case BLE_GAP_EVENT_REPEAT_PAIRING:
        // Replacing a saved phone requires the on-watch Forget Phone action.
        return BLE_GAP_REPEAT_PAIRING_IGNORE;
    case BLE_GAP_EVENT_CONNECT:
        if (!event->connect.status) {
            pairing_code=0;pairing_io_pending=false;
            security_pending=phone_window;
            gopine_ancs_reset();
            connection = event->connect.conn_handle;
            peer_generation++;
            phone_time_clear(&phone_time_incoming);
            if(phone_window) memset(&weather_incoming,0,sizeof(weather_incoming));
            music_subscribed=false;
            music_clear(&music_incoming);
            memset(&notification_incoming,0,sizeof(notification_incoming));
        }
        else advertise();
        break;
    case BLE_GAP_EVENT_DISCONNECT:
        // Retain the actual reason across automatic advertising/reconnects;
        // otherwise the UI hides every failed attempt as "Waiting for phone".
        if(phone_window)last_disconnect_reason=event->disconnect.reason;
        pairing_code=0;pairing_io_pending=security_pending=false;lifecycle_changed=true;
        gopine_ancs_reset();
        connection = BLE_HS_CONN_HANDLE_NONE;
        peer_generation++;
        phone_time_clear(&phone_time_incoming);
        if(phone_window) memset(&weather_incoming,0,sizeof(weather_incoming));
        music_subscribed=false;
        music_clear(&music_incoming);
        memset(&update_incoming,0,sizeof(update_incoming));
        memset(&notification_incoming,0,sizeof(notification_incoming));
        if(phone_window) slow_advertising=true;
        advertise();
        break;
    case BLE_GAP_EVENT_ADV_COMPLETE:
        if(phone_window && window) { slow_advertising=true; advertise(); }
        break;
    case BLE_GAP_EVENT_SUBSCRIBE:
        if(event->subscribe.attr_handle==music_handle) {
            music_subscribed=event->subscribe.cur_notify;
            music_incoming.dirty=true;
        }
        break;
    case BLE_GAP_EVENT_NOTIFY_RX:
        if(window && phone_window && !expired() && phone_link_ready(event->notify_rx.conn_handle))
            gopine_ancs_notify(event->notify_rx.conn_handle,event->notify_rx.attr_handle,event->notify_rx.om);
        break;
    }
    return 0;
}

static void advertise(void) {
    if (!window || pending || !synced || connection != BLE_HS_CONN_HANDLE_NONE) return;
    if(expired()) return;
    int32_t remaining = unbounded ? BLE_HS_FOREVER : (int32_t)(expires_at - ble_npl_time_get());
    if(phone_window && !slow_advertising) {
        int32_t fast_left=(int32_t)(fast_until-ble_npl_time_get());
        if(fast_left<=0) slow_advertising=true;
        else if(unbounded || remaining>fast_left) remaining=fast_left;
    }
    struct ble_hs_adv_fields fields = {0};
    static const ble_uuid16_t uuids[] = {BLE_UUID16_INIT(0x1805), BLE_UUID16_INIT(0x180f)};
    fields.flags = BLE_HS_ADV_F_DISC_GEN | BLE_HS_ADV_F_BREDR_UNSUP;
    fields.name = (uint8_t *)"InfiniTime"; // InfiniLink discovery filter.
    fields.name_len = 10;
    fields.name_is_complete = 1;
    fields.uuids16 = uuids;
    fields.num_uuids16 = 2;
    fields.uuids16_is_complete = 1;
    struct ble_hs_adv_fields response={0};
    if(update_window) {
        fields.name=NULL;fields.name_len=0;fields.name_is_complete=0;
        fields.uuids16=NULL;fields.num_uuids16=0;fields.uuids16_is_complete=0;
        fields.uuids128=&update_uuids[0];fields.num_uuids128=1;fields.uuids128_is_complete=1;
        response.name=(uint8_t*)"goPine Update";response.name_len=13;response.name_is_complete=1;
    }
    int rc=ble_gap_adv_rsp_set_fields(&response);
    if(rc){failure=rc;return;}
    rc = ble_gap_adv_set_fields(&fields);
    if (rc) { failure = rc; return; }
    struct ble_gap_adv_params params = {0};
    params.conn_mode = BLE_GAP_CONN_MODE_UND;
    params.disc_mode = BLE_GAP_DISC_MODE_GEN;
    params.itvl_min = slow_advertising ? 1600 : 160;
    params.itvl_max = slow_advertising ? 2400 : 240;
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
    if(!failure)failure=gopine_ble_restore_privacy();
    synced = failure == 0;
    if(failure && phone_window) {
        last_security_status=failure;lifecycle_changed=true;
    }
    if(synced)advertise();
}
static void on_reset(int reason) { synced = false; failure = reason ? reason : -1; }

static int start_window(const uint8_t *value, uint8_t battery, uint32_t window_ms, bool weather, bool phone) {
    // Signed deadline arithmetic requires a positive window below 2^31 ms.
    if ((!window_ms && !phone) || window_ms > INT32_MAX) return BLE_HS_EINVAL;
    if (init_failure) return init_failure;
    if (stopping || host_up) return BLE_HS_EBUSY;
    if (!initialized) {
        nimble_port_init();
        gopine_ble_set_host_queue(nimble_port_get_dflt_eventq());
        gopine_bond_init();
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
    if(!gopine_bond_prepare(battery)) {
        ble_phy_disable();ble_hw_rng_stop();ble_phy_rfclk_disable();
        return BLE_HS_ESTORE_CAP;
    }
    // The controller RF manager owns HFXO start/stop around scheduled events.
    memcpy(current, value, 10);
    battery_level = battery > 100 ? 100 : battery;
    pending = false; window = true; failure = 0; disconnect_after = 0;
    phone_time_clear(&phone_time_incoming);
    weather_window = weather; phone_window=phone;
    unbounded=phone && !window_ms; slow_advertising=false;
    music_subscribed=false;
    music_clear(&music_incoming);
    memset(&weather_incoming, 0, sizeof(weather_incoming));
    memset(&notification_incoming,0,sizeof(notification_incoming));
    expires_at = ble_npl_time_get() + window_ms;
    fast_until = ble_npl_time_get() + 30000;
    if (!host_up) { host_up = true; ble_hs_sched_start(); }
    else advertise();
    return failure;
}

int gopine_ble_start(const uint8_t *value, uint8_t battery, uint32_t window_ms) {
    return start_window(value, battery, window_ms, false, false);
}
int gopine_ble_start_weather(const uint8_t *value, uint8_t battery, uint32_t window_ms) {
    return start_window(value, battery, window_ms, true, false);
}
int gopine_ble_take_weather(uint8_t *value) {
    if (!window || !(weather_window || phone_window)) return 0;
    if (phone_window && phone_link_permission(connection)) {
        memset(&weather_incoming,0,sizeof(weather_incoming));
        return 0;
    }
    if (phone_window && !phone_link_ready(connection)) return 0;
    return weather_take(&weather_incoming, value);
}

void gopine_ble_stop(void) {
    pairing_code=0;pairing_io_pending=security_pending=false;
    gopine_ancs_reset();
    window = false; pending = false; weather_window = phone_window = false;
    phone_time_clear(&phone_time_incoming);
    music_subscribed=false;
    update_window=false;
    memset(&update_incoming,0,sizeof(update_incoming));
    music_clear(&music_incoming);
    memset(&weather_incoming, 0, sizeof(weather_incoming));
    memset(&notification_incoming,0,sizeof(notification_incoming));
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
    if (!initialized || (!host_up && !stopping)) return;
    gopine_ble_pump();
    if(window && phone_window && connection!=BLE_HS_CONN_HANDLE_NONE && security_pending) {
        security_pending=false;
        int rc=ble_gap_security_initiate(connection);
        if(rc && rc!=BLE_HS_EALREADY) ble_gap_terminate(connection,BLE_ERR_REM_USER_CONN_TERM);
    }
    if(pairing_io_pending) {
        pairing_io_pending=false;
        if(window && !update_window && pairing_connection==connection && !gopine_bond_exists()) {
            uint32_t random;
            // Use hardware entropy, never the scheduling-jitter LCG.
            do {ble_ll_rand_data_get((uint8_t*)&random,sizeof(random));} while(random>=4294000000u);
            struct ble_sm_io io={.action=BLE_SM_IOACT_DISP,.passkey=random%1000000u};
            pairing_code=io.passkey+1;lifecycle_changed=true;
            if(ble_sm_inject_io(connection,&io)) {
                pairing_code=0;
                ble_gap_terminate(connection,BLE_ERR_REM_USER_CONN_TERM);
            }
        }
    }
    if(window && phone_window && connection!=BLE_HS_CONN_HANDLE_NONE) {
        uint16_t progress=gopine_sm_progress(connection);
        // Retain the final observed phase after timeout/disconnect so INFO
        // explains where a failed physical exchange stopped.
        if(progress && progress!=pairing_progress) {
            pairing_progress=progress;lifecycle_changed=true;
        }
    }
    if(window && phone_window && !expired())
        gopine_ancs_poll(connection,phone_link_ready(connection));
    if (window && expired()) gopine_ble_stop();
    if (!window && host_up && !stopping) gopine_ble_stop();
    finish_stop();
    int before=gopine_bond_status();
    gopine_bond_poll(!host_up);
    if(before!=gopine_bond_status())lifecycle_changed=true;
}
void gopine_ble_controller_poll(void) {
    if(initialized && (host_up || stopping)) gopine_ble_controller_pump();
}

uint32_t gopine_ble_pairing_code(void) {return pairing_code;}
uint32_t gopine_ble_pairing_diagnostics(void) {
    return last_security_status | ((uint32_t)last_disconnect_reason<<16);
}
uint16_t gopine_ble_pairing_progress(void) {return pairing_progress;}
int gopine_ble_bond_status(void) {return initialized?gopine_bond_status():-2;}
int gopine_ble_forget_phone(void) {
    if(host_up || stopping)return BLE_HS_EBUSY;
    if(!gopine_bond_forget())return BLE_HS_ESTORE_FAIL;
    last_security_status=last_disconnect_reason=0;
    pairing_progress=0;
    return 0;
}

int gopine_ble_busy(void) { return initialized && (host_up || stopping); }

int gopine_ble_take(uint8_t *value, uint32_t *age) {
    if (window && phone_window && !expired())
        return phone_time_take(&phone_time_incoming, connection, peer_generation,
                               ble_npl_time_get(), value, age);
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

int gopine_ble_start_phone(const uint8_t *value, uint8_t battery, uint32_t window_ms) {
    return start_window(value,battery,window_ms,false,true);
}
int gopine_ble_start_update(uint8_t battery) {
    // Set before start_window because an already-synced host advertises there.
    update_window=true;
    memset(&update_incoming,0,sizeof(update_incoming));
    memset(update_status,0,sizeof(update_status));update_status[0]=1;update_status[1]=1;
    uint8_t time[10]={0};
    int rc=start_window(time,battery,600000,false,false);
    if(rc)update_window=false;
    return rc;
}
void gopine_ble_update_status(const uint8_t *status) {
    memcpy(update_status,status,16);
    if(window && update_window)expires_at=ble_npl_time_get()+600000;
}
int gopine_ble_take_update(uint8_t *out,unsigned *size) {
    if(!window || !update_window){*size=0;return 0;}
    return update_take(&update_incoming,out,size);
}
int gopine_ble_take_notification(uint8_t *out) {
    if(!window || !phone_window || expired()) return 0;
    if(phone_link_permission(connection)) {
        memset(&notification_incoming,0,sizeof(notification_incoming));
        return 0;
    }
    if(!phone_link_ready(connection))return 0;
    return notification_take(&notification_incoming,out);
}
int gopine_ble_take_apple_notification(uint8_t *out) {
    bool trusted=window && phone_window && !expired() && phone_link_ready(connection);
    return gopine_ancs_take(out,trusted);
}
int gopine_ble_take_music(uint8_t *out) {
    if(!phone_window || !music_incoming.dirty) return 0;
    enum phone_link_state link = phone_link_snapshot(connection,music_subscribed);
    bool trusted = link==PHONE_LINK_CONNECTED || link==PHONE_LINK_MUSIC_READY;
    if(!trusted && phone_link_permission(connection)) music_clear(&music_incoming);
    // Preserve authenticated metadata received before phase 3, while exposing
    // only the securing state to Go until the shared bond is complete.
    memset(out,0,83);
    if(trusted) {
        memcpy(out,music_incoming.track,40);
        memcpy(out+40,music_incoming.artist,40);
        out[80]=music_incoming.status;
        out[82]=music_incoming.status_known;
    }
    out[81]=link;
    for(unsigned i=0;i<4;i++) out[83+i]=(uint8_t)(peer_generation>>(i*8));
    music_incoming.dirty=false;
    return 1;
}
int gopine_ble_music_command(uint8_t command, uint32_t generation) {
    if(!music_command_valid(command)) return BLE_HS_EINVAL;
    if(!window || !phone_window || expired() || connection==BLE_HS_CONN_HANDLE_NONE || !music_subscribed || generation!=peer_generation)
        return BLE_HS_ENOTCONN;
    if(!phone_link_ready(connection)) return BLE_HS_ENOTCONN;
    struct os_mbuf *om=ble_hs_mbuf_from_flat(&command,1);
    if(!om) return BLE_HS_ENOMEM;
    // NimBLE consumes the mbuf on success AND error. Handle zero is valid.
    return ble_gattc_notify_custom(connection,music_handle,om);
}
int gopine_ble_updates(void) {
    bool ready=window && phone_window && phone_link_ready(connection);
    return lifecycle_changed || (gopine_ble_busy() && gopine_ble_host_work_pending()) ||
           gopine_ancs_updates() || (window && (failure || pending || update_incoming.kind ||
                      ((!phone_window || ready) && (weather_incoming.sizes[0] || weather_incoming.sizes[1])) ||
                      (phone_window && (music_incoming.dirty ||
                          (ready && (phone_time_incoming.size || notification_incoming.count))))));
}
void gopine_ble_ack_updates(void) { lifecycle_changed=false; }
uint32_t gopine_ble_idle_ms(void) {
    if(!gopine_ble_busy()) return 240000;
    uint32_t delay=gopine_ble_next_work(), now=ble_npl_time_get();
    if(window && phone_window && phone_link_ready(connection)) {
        uint32_t ancs_delay=gopine_ancs_delay();
        if(ancs_delay<delay)delay=ancs_delay;
    }
    uint32_t bond_delay=gopine_bond_delay();
    if(bond_delay<delay)delay=bond_delay;
    if(window && !unbounded) {
        int32_t left=(int32_t)(expires_at-now);
        uint32_t due=left>0?(uint32_t)left:0;
        if(due<delay) delay=due;
    }
    if(!window && !stopping) {
        int32_t left=(int32_t)(disconnect_after-now);
        uint32_t due=disconnect_after && left>0?(uint32_t)left:0;
        if(due<delay) delay=due;
    }
    return delay;
}
