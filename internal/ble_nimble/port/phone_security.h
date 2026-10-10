#ifndef GOPINE_PHONE_SECURITY_H
#define GOPINE_PHONE_SECURITY_H

#include "host/ble_hs.h"

// Internal byte 81 of the 87-byte music snapshot. Preserve the existing ready
// value while separating phone authentication from the optional music service.
enum phone_link_state {
    PHONE_LINK_DISCONNECTED = 0,
    PHONE_LINK_SECURING = 1,
    PHONE_LINK_MUSIC_READY = 2,
    PHONE_LINK_CONNECTED = 3,
};

// ATT authentication is established before SMP identity/key distribution
// finishes. A secure retry must not request authentication again just because
// the bond is still pending. Feature delivery separately waits for that bond.
static inline int phone_link_permission(uint16_t conn) {
    struct ble_gap_conn_desc desc;
    if (conn == BLE_HS_CONN_HANDLE_NONE || ble_gap_conn_find(conn, &desc))
        return BLE_ATT_ERR_UNLIKELY;
    if (!desc.sec_state.encrypted) {
        // Return ATT security errors so the central secures the link and
        // retries its operation, including CTS writes before Battery reads.
        struct ble_store_key_sec key = {.peer_addr = desc.peer_id_addr};
        struct ble_store_value_sec value;
        if (!ble_store_read_peer_sec(&key, &value) && value.ltk_present)
            return BLE_ATT_ERR_INSUFFICIENT_ENC;
        return BLE_ATT_ERR_INSUFFICIENT_AUTHEN;
    }
    if (!desc.sec_state.authenticated)
        return BLE_ATT_ERR_INSUFFICIENT_AUTHEN;
    if (desc.sec_state.key_size != 16) return BLE_ATT_ERR_INSUFFICIENT_KEY_SZ;
    return 0;
}

static inline bool phone_link_ready(uint16_t conn) {
    struct ble_gap_conn_desc desc;
    return conn != BLE_HS_CONN_HANDLE_NONE && !ble_gap_conn_find(conn, &desc) &&
        desc.sec_state.encrypted && desc.sec_state.authenticated &&
        desc.sec_state.bonded && desc.sec_state.key_size == 16;
}

static inline enum phone_link_state phone_link_snapshot(uint16_t conn,
                                                        bool music_subscribed) {
    if (conn == BLE_HS_CONN_HANDLE_NONE) return PHONE_LINK_DISCONNECTED;
    if (!phone_link_ready(conn)) return PHONE_LINK_SECURING;
    return music_subscribed ? PHONE_LINK_MUSIC_READY : PHONE_LINK_CONNECTED;
}

#endif
