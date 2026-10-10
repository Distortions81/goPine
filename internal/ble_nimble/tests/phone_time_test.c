#include "../port/phone_time.h"
#include <assert.h>

static struct ble_gap_conn_desc peer;
static bool connected, saved_key;
static unsigned key_queries;
static uint8_t packet[10] = {0xea, 0x07, 10, 9, 16, 7, 8, 5, 0, 1};

int ble_gap_conn_find(uint16_t handle, struct ble_gap_conn_desc *out) {
    if (!connected || handle != 42) return BLE_HS_ENOTCONN;
    *out = peer;
    return 0;
}
int ble_store_read_peer_sec(const struct ble_store_key_sec *key,
                            struct ble_store_value_sec *out) {
    key_queries++;
    assert(!memcmp(&key->peer_addr, &peer.peer_id_addr, sizeof(key->peer_addr)));
    memset(out, 0, sizeof(*out));
    out->ltk_present = saved_key;
    return saved_key ? 0 : BLE_HS_ENOENT;
}
static void secure(void) {
    connected = true;
    peer.sec_state.encrypted = 1;
    peer.sec_state.authenticated = 1;
    peer.sec_state.bonded = 1;
    peer.sec_state.key_size = 16;
}
static void permission_and_retry(void) {
    struct phone_time_mailbox m = {0};
    uint8_t out[10] = {0};
    uint32_t age = 123;
    connected = true;
    peer.peer_id_addr.type = BLE_ADDR_RANDOM;
    peer.peer_id_addr.val[0] = 91;
    // InfiniLink writes CTS before reading Battery. Request pairing through
    // this write; the original unauthenticated payload must never be retained.
    assert(phone_time_receive(&m, 42, 1, packet, 10, 100) == BLE_ATT_ERR_INSUFFICIENT_AUTHEN);
    assert(!m.size);
    secure();
    assert(phone_time_take(&m, 42, 1, 120, out, &age) == 0);
    assert(age == 123 && !out[0]);
    assert(phone_time_receive(&m, 42, 1, packet, 10, 125) == 0);
    assert(phone_time_take(&m, 42, 1, 300, out, &age) == 10);
    assert(age == 175 && !memcmp(out, packet, 10));
    assert(phone_time_take(&m, 42, 1, 301, out, &age) == 0);
    // Taking time leaves the connection usable for future automatic updates.
    assert(phone_time_receive(&m, 42, 1, packet, 9, 350) == 0);
    assert(phone_time_take(&m, 42, 1, 350, out, &age) == 9 && !age);

    peer.sec_state.encrypted = 0;
    saved_key = true;
    assert(phone_time_receive(&m, 42, 1, packet, 10, 400) == BLE_ATT_ERR_INSUFFICIENT_ENC);
    assert(!m.size);
    secure();
    peer.sec_state.authenticated = 0;
    assert(phone_link_permission(42) == BLE_ATT_ERR_INSUFFICIENT_AUTHEN);
    secure();
    peer.sec_state.bonded = 0;
    assert(phone_link_permission(42) == 0 && !phone_link_ready(42));
    assert(!phone_time_receive(&m, 42, 1, packet, 10, 400));
    assert(!phone_time_take(&m, 42, 1, 410, out, &age) && m.size);
    peer.sec_state.bonded = 1;
    assert(phone_time_take(&m, 42, 1, 420, out, &age)==10 && age==20);
    secure();
    peer.sec_state.key_size = 7;
    assert(phone_link_permission(42) == BLE_ATT_ERR_INSUFFICIENT_KEY_SZ);
    assert(key_queries == 2); // Encrypted links use their actual security state.
    assert(phone_link_permission(BLE_HS_CONN_HANDLE_NONE) == BLE_ATT_ERR_UNLIKELY);
    connected = false;
    assert(phone_link_permission(42) == BLE_ATT_ERR_UNLIKELY);
}
static void discard_stale_peer(void) {
    struct phone_time_mailbox m = {0};
    uint8_t out[10] = {0};
    uint32_t age = 0;
    secure();
    assert(!phone_time_receive(&m, 42, 9, packet, 10, 100));
    connected = false;
    assert(!phone_time_take(&m, 42, 9, 101, out, &age) && !m.size);
    secure();
    assert(!phone_time_take(&m, 42, 9, 102, out, &age));
    // A new connection may reuse the numeric BLE handle; generation fences it.
    assert(!phone_time_receive(&m, 42, 9, packet, 10, 100));
    assert(!phone_time_take(&m, 42, 10, 101, out, &age) && !m.size);
    assert(!phone_time_receive(&m, 42, 10, packet, 10, 100));
    assert(!phone_time_take(&m, 43, 10, 101, out, &age) && !m.size);
    assert(!phone_time_receive(&m, 42, 10, packet, 10, 100));
    peer.sec_state.encrypted = 0;
    assert(!phone_time_take(&m, 42, 10, 101, out, &age) && !m.size);
    secure();
    assert(!phone_time_receive(&m, 42, 10, packet, 10, 100));
    phone_time_clear(&m); // Stop, disconnect, and restart all clear the mailbox.
    assert(!phone_time_take(&m, 42, 10, 101, out, &age));
}
static void bounds_and_age(void) {
    struct phone_time_mailbox m = {0};
    uint8_t out[10] = {0};
    uint32_t age = 0;
    secure();
    for (unsigned size = 0; size < 9; size++)
        assert(phone_time_receive(&m, 42, 1, packet, size, 0) == BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN);
    assert(phone_time_receive(&m, 42, 1, packet, 11, 0) == BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN);
    assert(!m.size);
    assert(!phone_time_receive(&m, 42, 1, packet, 10, UINT32_MAX - 99));
    assert(phone_time_take(&m, 42, 1, 100, out, &age) == 10 && age == 200);
    assert(!phone_time_receive(&m, 42, 1, packet, 10, 0));
    assert(phone_time_take(&m, 42, 1, PHONE_TIME_MAX_AGE_MS, out, &age) == 10);
    assert(!phone_time_receive(&m, 42, 1, packet, 10, 0));
    assert(!phone_time_take(&m, 42, 1, PHONE_TIME_MAX_AGE_MS + 1, out, &age) && !m.size);
    // Coalesce rapid writes to the newest fully authenticated value.
    assert(!phone_time_receive(&m, 42, 1, packet, 10, 0));
    packet[6]++;
    assert(!phone_time_receive(&m, 42, 1, packet, 10, 5));
    assert(phone_time_take(&m, 42, 1, 10, out, &age) == 10 && age == 5);
    assert(!memcmp(out, packet, 10));
}
static void authentication_and_subscription_order(void) {
    connected = false;
    memset(&peer, 0, sizeof(peer));
    assert(phone_link_snapshot(BLE_HS_CONN_HANDLE_NONE, false) == PHONE_LINK_DISCONNECTED);
    assert(phone_link_snapshot(BLE_HS_CONN_HANDLE_NONE, true) == PHONE_LINK_DISCONNECTED);
    connected = true;
    // A central can subscribe before pairing; that must not enable controls.
    assert(phone_link_snapshot(42, false) == PHONE_LINK_SECURING);
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    peer.sec_state.encrypted = 1;
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    peer.sec_state.authenticated = 1;
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    peer.sec_state.bonded = 1;
    peer.sec_state.key_size = 7;
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    peer.sec_state.key_size = 16;
    assert(phone_link_snapshot(42, true) == PHONE_LINK_MUSIC_READY);
    // Removing music notifications does not disconnect time/weather services.
    assert(phone_link_snapshot(42, false) == PHONE_LINK_CONNECTED);
    // The opposite order is common when restoring a saved connection: security
    // completes first, then the host restores a saved music CCCD subscription.
    assert(phone_link_snapshot(42, true) == PHONE_LINK_MUSIC_READY);
    peer.sec_state.encrypted = 0;
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    secure();
    assert(phone_link_snapshot(42, true) == PHONE_LINK_MUSIC_READY);
    connected = false;
    assert(phone_link_snapshot(BLE_HS_CONN_HANDLE_NONE, true) == PHONE_LINK_DISCONNECTED);
    // Reusing the same numeric handle does not reuse the old peer's trust.
    connected = true;
    memset(&peer, 0, sizeof(peer));
    assert(phone_link_snapshot(42, true) == PHONE_LINK_SECURING);
    secure();
    assert(phone_link_snapshot(42, false) == PHONE_LINK_CONNECTED);
}
int main(void) {
    permission_and_retry();
    discard_stale_peer();
    bounds_and_age();
    authentication_and_subscription_order();
    return 0;
}
