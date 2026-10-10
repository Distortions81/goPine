#ifndef GOPINE_BATTERY_SECURITY_H
#define GOPINE_BATTERY_SECURITY_H

#include <stdbool.h>
#include "host/ble_hs.h"

// Security is checked at read time: the GATT table is shared by phone and
// foreground update sessions. BlueZ probes Battery automatically on connection.
#define GOPINE_BATTERY_FLAGS (BLE_GATT_CHR_F_READ | BLE_GATT_CHR_F_NOTIFY)

static inline int battery_read_permission(uint16_t conn, bool window_open,
                                          bool update_mode) {
    // NimBLE uses its local-read handle when fetching notification values.
    // Its normal ATT permission checks also bypass link security for that read;
    // it is not a remote connection and must not trigger peer/key lookup.
    if (conn == BLE_HS_CONN_HANDLE_NONE) return 0;

    // Update mode deliberately rejects pairing and cannot replace a phone bond.
    // Its harmless battery probe must therefore succeed without starting SMP.
    if (window_open && update_mode) return 0;

    struct ble_gap_conn_desc desc;
    if (ble_gap_conn_find(conn, &desc)) return BLE_ATT_ERR_UNLIKELY;
    if (desc.sec_state.encrypted) return 0;

    // Match NimBLE's READ_ENC response outside update mode: an existing peer
    // key requests encryption; a new peer requests authentication/pairing.
    struct ble_store_key_sec key = {.peer_addr = desc.peer_id_addr};
    struct ble_store_value_sec value;
    if (!ble_store_read_peer_sec(&key, &value) && value.ltk_present)
        return BLE_ATT_ERR_INSUFFICIENT_ENC;
    return BLE_ATT_ERR_INSUFFICIENT_AUTHEN;
}

static inline int battery_read_access(uint16_t conn,
                                      struct ble_gatt_access_ctxt *ctx,
                                      bool window_open, bool update_mode,
                                      uint8_t level) {
    if (ctx->op != BLE_GATT_ACCESS_OP_READ_CHR) return BLE_ATT_ERR_UNLIKELY;
    int rc = battery_read_permission(conn, window_open, update_mode);
    if (rc) return rc;
    return os_mbuf_append(ctx->om, &level, 1) ? BLE_ATT_ERR_INSUFFICIENT_RES : 0;
}

#endif
