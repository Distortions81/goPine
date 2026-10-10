#include "../port/update_mailbox.h"
#include "../port/battery_security.h"
#include <assert.h>

static struct ble_gap_conn_desc peer;
static bool connected, saved_key;
static unsigned connection_queries, key_queries;
static struct os_mbuf output;
static unsigned appended;
static uint8_t battery_byte;
static bool no_buffer;

int ble_gap_conn_find(uint16_t handle,struct ble_gap_conn_desc *out) {
    connection_queries++;
    if(!connected || handle!=42) return BLE_HS_ENOTCONN;
    *out=peer;return 0;
}
int ble_store_read_peer_sec(const struct ble_store_key_sec *key,struct ble_store_value_sec *out) {
    key_queries++;
    assert(!memcmp(&key->peer_addr,&peer.peer_id_addr,sizeof(key->peer_addr)));
    memset(out,0,sizeof(*out));
    out->ltk_present=saved_key;
    return saved_key?0:BLE_HS_ENOENT;
}
int os_mbuf_append(struct os_mbuf *om,const void *data,uint16_t size) {
    assert(om==&output && size==1);
    if(no_buffer) return -1;
    appended++;
    battery_byte=*(const uint8_t*)data;
    return 0;
}
static void battery_update_discovery(void) {
    // An OS Battery probe shares the updater's unencrypted connection. No
    // security error means it never escalates to the pairing request which
    // update mode rejects, even when the watch already stores a phone's bond.
    assert(!(GOPINE_BATTERY_FLAGS & BLE_GATT_CHR_F_READ_ENC));
    connected=true;
    peer.peer_id_addr.type=BLE_ADDR_RANDOM;
    peer.peer_id_addr.val[0]=19;
    struct ble_gatt_access_ctxt ctx={.op=BLE_GATT_ACCESS_OP_READ_CHR,.om=&output};
    for(unsigned existing_key=0;existing_key<2;existing_key++) {
        saved_key=existing_key;
        assert(battery_read_access(42,&ctx,true,true,79)==0);
        assert(battery_byte==79 && appended==existing_key+1);
    }
    assert(connection_queries==0 && key_queries==0);

    // Leaving update mode restores the same READ_ENC policy for this peer.
    saved_key=false;
    assert(battery_read_access(42,&ctx,true,false,79)==BLE_ATT_ERR_INSUFFICIENT_AUTHEN);
    assert(battery_read_access(42,&ctx,false,true,79)==BLE_ATT_ERR_INSUFFICIENT_AUTHEN);
    saved_key=true;
    assert(battery_read_access(42,&ctx,true,false,79)==BLE_ATT_ERR_INSUFFICIENT_ENC);
    assert(appended==2); // Failed security checks must not expose a value.
    peer.sec_state.encrypted=1;
    peer.sec_state.authenticated=0;
    peer.sec_state.key_size=7; // READ_ENC had no authentication/min-key policy.
    unsigned previous_queries=key_queries;
    assert(battery_read_access(42,&ctx,true,false,68)==0);
    assert(key_queries==previous_queries && battery_byte==68 && appended==3);
    connected=false;
    assert(battery_read_access(42,&ctx,true,false,79)==BLE_ATT_ERR_UNLIKELY);

    // ble_gatts_chr_updated -> ble_gattc_notify_custom(NULL) fetches the
    // characteristic through a local read, with no peer connection handle.
    // Preserve that path outside update mode without querying link security.
    unsigned previous_connections=connection_queries;
    previous_queries=key_queries;
    assert(battery_read_access(BLE_HS_CONN_HANDLE_NONE,&ctx,true,false,57)==0);
    assert(battery_byte==57 && appended==4);
    assert(battery_read_access(BLE_HS_CONN_HANDLE_NONE,&ctx,false,false,56)==0);
    assert(battery_byte==56 && appended==5);
    assert(connection_queries==previous_connections && key_queries==previous_queries);

    ctx.op=BLE_GATT_ACCESS_OP_WRITE_CHR;
    assert(battery_read_access(42,&ctx,true,true,79)==BLE_ATT_ERR_UNLIKELY);
    assert(battery_read_access(BLE_HS_CONN_HANDLE_NONE,&ctx,true,false,79)==BLE_ATT_ERR_UNLIKELY);
    assert(appended==5);
    ctx.op=BLE_GATT_ACCESS_OP_READ_CHR;
    no_buffer=true;
    assert(battery_read_access(42,&ctx,true,true,79)==BLE_ATT_ERR_INSUFFICIENT_RES);
}

int main(void) {
    battery_update_discovery();
    struct update_mailbox m={0};uint8_t data[201]={1},out[200];unsigned size;
    for(unsigned n=0;n<9;n++)assert(update_store(&m,1,data,n)==-1);
    assert(update_store(&m,1,data,9)==1);
    assert(update_store(&m,2,data,200)==0);
    assert(update_take(&m,out,&size)==1 && size==9 && out[0]==1);
    assert(update_take(&m,out,&size)==0 && size==0);
    assert(update_store(&m,2,data,201)==-1);
    for(unsigned i=0;i<10000;i++) {
        assert(update_store(&m,2,data,200)==1);
        assert(update_take(&m,out,&size)==2 && size==200);
    }
    data[0]=2;assert(update_store(&m,1,data,5)==1);
    return 0;
}
