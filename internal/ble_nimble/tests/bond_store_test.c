#include <assert.h>
#include "../port/bond.c"
struct ble_hs_cfg ble_hs_cfg;
static uint32_t now;
uint32_t ble_npl_time_get(void){return now;}
void ble_store_key_from_value_sec(struct ble_store_key_sec *k,const struct ble_store_value_sec *v) {
    memset(k,0,sizeof(*k));k->peer_addr=v->peer_addr;
}
void ble_store_key_from_value_cccd(struct ble_store_key_cccd *k,const struct ble_store_value_cccd *v) {
    memset(k,0,sizeof(*k));k->peer_addr=v->peer_addr;k->chr_val_handle=v->chr_val_handle;
}
static uint32_t flash[BOND_PAGE_SIZE/4];
uint32_t bond_flash_word(unsigned off){assert(off<sizeof(flash));return flash[off/4];}
bool bond_flash_write(unsigned off,uint32_t v){assert(off<sizeof(flash));flash[off/4]&=v;return flash[off/4]==v;}
bool bond_flash_erase(void){memset(flash,0xff,sizeof(flash));return true;}
int main(void) {
    bond_flash_erase();gopine_bond_init();assert(gopine_bond_status()==0);
    union ble_store_value our={0},peer={0},cccd={0},out={0};
    our.sec.peer_addr.type=BLE_ADDR_PUBLIC;our.sec.peer_addr.val[0]=19;
    our.sec.ltk_present=1;our.sec.key_size=16;memset(our.sec.ltk,0x56,16);
    peer=our;peer.sec.irk_present=1;memset(peer.sec.irk,0x78,16);
    cccd.cccd.peer_addr=our.sec.peer_addr;cccd.cccd.chr_val_handle=27;cccd.cccd.flags=1;
    assert(!ble_hs_cfg.store_write_cb(BLE_STORE_OBJ_TYPE_OUR_SEC,&our));
    assert(!ble_hs_cfg.store_write_cb(BLE_STORE_OBJ_TYPE_PEER_SEC,&peer));
    assert(!ble_hs_cfg.store_write_cb(BLE_STORE_OBJ_TYPE_CCCD,&cccd));
    assert(gopine_bond_status()==1);now=1000;gopine_bond_poll(false);
    assert(gopine_bond_status()==2);
    unsigned written=next_record;
    // Simulate reboot, then look up the actual key material and subscription.
    gopine_bond_init();union ble_store_key key={0};key.sec.peer_addr=our.sec.peer_addr;
    assert(!ble_hs_cfg.store_read_cb(BLE_STORE_OBJ_TYPE_OUR_SEC,&key,&out));
    assert(!memcmp(&out.sec,&our.sec,sizeof(our.sec)));
    assert(!ble_hs_cfg.store_read_cb(BLE_STORE_OBJ_TYPE_PEER_SEC,&key,&out));
    assert(!memcmp(&out.sec,&peer.sec,sizeof(peer.sec)));
    key.cccd.peer_addr=our.sec.peer_addr;key.cccd.chr_val_handle=27;
    assert(!ble_hs_cfg.store_read_cb(BLE_STORE_OBJ_TYPE_CCCD,&key,&out));
    assert(out.cccd.flags==1);
    assert(!ble_hs_cfg.store_write_cb(BLE_STORE_OBJ_TYPE_CCCD,&cccd));
    gopine_bond_poll(true);assert(next_record==written); // Identical subscriptions don't wear flash.
    assert(gopine_bond_forget());gopine_bond_init();
    assert(gopine_bond_status()==0);
    key.sec.peer_addr=our.sec.peer_addr;
    assert(ble_hs_cfg.store_read_cb(BLE_STORE_OBJ_TYPE_OUR_SEC,&key,&out)==BLE_HS_ENOENT);
}
void ble_hs_log_flat_buf(const void *data,int len){(void)data;(void)len;}
