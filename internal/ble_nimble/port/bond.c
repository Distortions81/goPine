// Use the pinned upstream key matching and CCCD store, with a durable snapshot.
#include "ble_store_ram.c"
#include "bond_journal.h"
struct bond_state {
    uint32_t version, our_count, peer_count, cccd_count;
    struct ble_store_value_sec our[MYNEWT_VAL(BLE_STORE_MAX_BONDS)];
    struct ble_store_value_sec peer[MYNEWT_VAL(BLE_STORE_MAX_BONDS)];
    struct ble_store_value_cccd cccd[MYNEWT_VAL(BLE_STORE_MAX_CCCDS)];
};
_Static_assert(sizeof(struct bond_state)<=BOND_PAYLOAD_SIZE,"bond snapshot too large");
static struct bond_state snapshot;
static unsigned next_record;
static bool dirty, failed;
static uint32_t save_after;
static void capture(void) {
    snapshot.version=1;
    snapshot.our_count=ble_store_ram_num_our_secs;
    snapshot.peer_count=ble_store_ram_num_peer_secs;
    snapshot.cccd_count=ble_store_ram_num_cccds;
    memcpy(snapshot.our,ble_store_ram_our_secs,sizeof(snapshot.our));
    memcpy(snapshot.peer,ble_store_ram_peer_secs,sizeof(snapshot.peer));
    memcpy(snapshot.cccd,ble_store_ram_cccds,sizeof(snapshot.cccd));
}
static void changed(void) {dirty=true;save_after=ble_npl_time_get()+1000;}
static int write_bond(int type, const union ble_store_value *value) {
    int rc=ble_store_ram_write(type,value);if(!rc)changed();else failed=true;return rc;
}
static int delete_bond(int type, const union ble_store_key *key) {
    int rc=ble_store_ram_delete(type,key);if(!rc)changed();return rc;
}
void gopine_bond_init(void) {
    ble_store_ram_init();
    if(bond_journal_load(&snapshot,sizeof(snapshot),&next_record) && snapshot.version==1 &&
       snapshot.our_count<=MYNEWT_VAL(BLE_STORE_MAX_BONDS) &&
       snapshot.peer_count<=MYNEWT_VAL(BLE_STORE_MAX_BONDS) &&
       snapshot.cccd_count<=MYNEWT_VAL(BLE_STORE_MAX_CCCDS)) {
        ble_store_ram_num_our_secs=snapshot.our_count;
        ble_store_ram_num_peer_secs=snapshot.peer_count;
        ble_store_ram_num_cccds=snapshot.cccd_count;
        memcpy(ble_store_ram_our_secs,snapshot.our,sizeof(snapshot.our));
        memcpy(ble_store_ram_peer_secs,snapshot.peer,sizeof(snapshot.peer));
        memcpy(ble_store_ram_cccds,snapshot.cccd,sizeof(snapshot.cccd));
    }
    ble_hs_cfg.store_write_cb=write_bond;ble_hs_cfg.store_delete_cb=delete_bond;
    capture();
}
// Reserve space before radio startup. Losing power during page reclamation can
// lose the bond; it cannot restore partial keys or touch firmware/settings.
bool gopine_bond_prepare(uint8_t battery) {
    if(next_record < BOND_PAGE_SIZE-BOND_RECORD_SIZE && !failed)return true;
    if(battery<20)return false;
    capture();
    if(!bond_flash_erase())return false;
    next_record=0;failed=!bond_journal_save(&snapshot,sizeof(snapshot),&next_record);
    dirty=false;return !failed;
}
void gopine_bond_poll(bool stopped) {
    if(!dirty || failed || (!stopped && (int32_t)(ble_npl_time_get()-save_after)<0))return;
    // Repeated CCCD writes with identical values must not wear the journal.
    uint32_t before=bond_crc(&snapshot,sizeof(snapshot));capture();dirty=false;
    if(before!=bond_crc(&snapshot,sizeof(snapshot)))
        failed=!bond_journal_save(&snapshot,sizeof(snapshot),&next_record);
}
int gopine_bond_status(void) {
    if(failed)return -1;
    if(dirty)return 1;
    return ble_store_ram_num_our_secs?2:0;
}
bool gopine_bond_exists(void) {return ble_store_ram_num_our_secs!=0;}
uint32_t gopine_bond_delay(void) {
    if(!dirty || failed)return 240000;
    int32_t left=(int32_t)(save_after-ble_npl_time_get());return left>0?(uint32_t)left:0;
}
bool gopine_bond_forget(void) {
    if(!bond_flash_erase()){failed=true;return false;}
    memset(ble_store_ram_our_secs,0,sizeof(ble_store_ram_our_secs));
    memset(ble_store_ram_peer_secs,0,sizeof(ble_store_ram_peer_secs));
    memset(ble_store_ram_cccds,0,sizeof(ble_store_ram_cccds));
    ble_store_ram_num_our_secs=ble_store_ram_num_peer_secs=ble_store_ram_num_cccds=0;
    next_record=0;dirty=failed=false;capture();return true;
}
