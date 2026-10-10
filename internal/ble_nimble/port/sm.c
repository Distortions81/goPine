// The pinned host updates the connection's bonded state after fresh key
// exchange, but omits that bit from the result passed to GAP. Also, the newly
// learned peer identity is installed only later, while persisting keys. Save
// pre-pairing subscriptions after that identity and both keys are stored, not
// under the temporary address used for initial pairing. Keep vendor sources
// intact and preserve GAP's callback-before-GATT ordering.
#include "ble_hs_priv.h"

static void gopine_sm_gap_enc_event(uint16_t conn_handle, int status,
                                    int security_restored, int bonded);
static int gopine_sm_store_our_sec(const struct ble_store_value_sec *value);
static int gopine_sm_store_peer_sec(const struct ble_store_value_sec *value);
#define ble_gap_enc_event gopine_sm_gap_enc_event
#define ble_store_write_our_sec gopine_sm_store_our_sec
#define ble_store_write_peer_sec gopine_sm_store_peer_sec
#include "ble_sm.c"
#undef ble_gap_enc_event
#undef ble_store_write_our_sec
#undef ble_store_write_peer_sec

// The port supports exactly one connection, and the pinned result handler
// calls these hooks synchronously in ENC_CHANGE -> our key -> peer key order.
static uint16_t pending_bond = BLE_HS_CONN_HANDLE_NONE;
static bool pending_our_key;
static ble_addr_t pending_identity;

// Read only the protocol phase and verified passkey-round count. Never export
// keys, nonces or the private scalar. Called after the host pump has unwound.
// The high byte is our stable UI phase, not NimBLE's private state numbering.
uint16_t gopine_sm_progress(uint16_t conn_handle) {
    uint16_t progress = 0;
    ble_hs_lock();
    struct ble_sm_proc *proc = ble_sm_proc_find(conn_handle,
                                               BLE_SM_PROC_STATE_NONE, -1, NULL);
    if (proc) {
        unsigned phase = 1;
        switch (proc->state) {
        case BLE_SM_PROC_STATE_CONFIRM:
        case BLE_SM_PROC_STATE_RANDOM: phase = 2; break;
        case BLE_SM_PROC_STATE_DHKEY_CHECK: phase = 3; break;
        case BLE_SM_PROC_STATE_LTK_START:
        case BLE_SM_PROC_STATE_LTK_RESTORE:
        case BLE_SM_PROC_STATE_ENC_START:
        case BLE_SM_PROC_STATE_ENC_RESTORE: phase = 4; break;
        case BLE_SM_PROC_STATE_KEY_EXCH: phase = 5; break;
        }
        unsigned rounds = proc->passkey_bits_exchanged;
        // The pinned host increments when it generates a confirm, before
        // verifying the peer random. Count only finished rounds in the UI.
        if (proc->state == BLE_SM_PROC_STATE_RANDOM && rounds) rounds--;
        progress = (phase << 8) | rounds;
        if (proc->state == BLE_SM_PROC_STATE_RANDOM) progress |= 0x80;
    }
    ble_hs_unlock();
    return progress;
}

static bool gopine_sm_bond_connection(struct ble_gap_conn_desc *desc) {
    return pending_bond != BLE_HS_CONN_HANDLE_NONE &&
        !ble_gap_conn_find(pending_bond, desc) && desc->sec_state.encrypted &&
        desc->sec_state.authenticated && desc->sec_state.bonded &&
        desc->sec_state.key_size == 16;
}

static void gopine_sm_gap_enc_event(uint16_t conn_handle, int status,
                                    int security_restored, int bonded) {
    pending_bond = BLE_HS_CONN_HANDLE_NONE;
    pending_our_key = false;
    if (!status && !security_restored) {
        struct ble_gap_conn_desc desc;
        pending_bond = conn_handle;
        if (!gopine_sm_bond_connection(&desc))
            pending_bond = BLE_HS_CONN_HANDLE_NONE;
        // GATT establishment must wait for identity/key persistence. Restore
        // and failure calls retain the exact upstream arguments and behavior.
        bonded = 0;
    }
    ble_gap_enc_event(conn_handle, status, security_restored, bonded);
}

static int gopine_sm_store_our_sec(const struct ble_store_value_sec *value) {
    int rc = ble_store_write_our_sec(value);
    struct ble_gap_conn_desc desc;
    if (!rc && gopine_sm_bond_connection(&desc) &&
        !ble_addr_cmp(&desc.peer_id_addr, &value->peer_addr)) {
        pending_identity = value->peer_addr;
        pending_our_key = true;
    } else {
        pending_bond = BLE_HS_CONN_HANDLE_NONE;
        pending_our_key = false;
    }
    return rc;
}

static int gopine_sm_store_peer_sec(const struct ble_store_value_sec *value) {
    int rc = ble_store_write_peer_sec(value);
    struct ble_gap_conn_desc desc;
    bool establish = !rc && pending_our_key &&
        gopine_sm_bond_connection(&desc) &&
        !ble_addr_cmp(&pending_identity, &value->peer_addr) &&
        !ble_addr_cmp(&desc.peer_id_addr, &value->peer_addr);
    uint16_t conn_handle = pending_bond;
    pending_bond = BLE_HS_CONN_HANDLE_NONE;
    pending_our_key = false;
    if (establish) ble_gatts_bonding_established(conn_handle);
    return rc;
}
