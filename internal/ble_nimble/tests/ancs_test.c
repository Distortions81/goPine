#include <assert.h>
#include "../port/ancs.c"

static uint32_t now;
static unsigned calls;
static uint16_t last_handle;
static uint8_t last_write[16];
static unsigned last_size;
static ble_gatt_disc_svc_fn *service_callback;
static ble_gatt_chr_fn *character_callback;
static ble_gatt_dsc_fn *descriptor_callback;
static ble_gatt_attr_fn *write_callback;
static void *callback_arg;
static bool want_gatt;
uint32_t ble_npl_time_get(void) { return now; }
int ble_gattc_disc_svc_by_uuid(uint16_t conn,const ble_uuid_t *uuid,ble_gatt_disc_svc_fn *cb,void *arg) {
    assert(conn==7); calls++; want_gatt=!ble_uuid_cmp(uuid,BLE_UUID16_DECLARE(0x1801));
    service_callback=cb; callback_arg=arg; return 0;
}
int ble_gattc_disc_all_chrs(uint16_t conn,uint16_t start,uint16_t end,ble_gatt_chr_fn *cb,void *arg) {
    assert(conn==7 && start<end); calls++; character_callback=cb;callback_arg=arg;return 0;
}
int ble_gattc_disc_all_dscs(uint16_t conn,uint16_t start,uint16_t end,ble_gatt_dsc_fn *cb,void *arg) {
    assert(conn==7 && start<end);calls++;last_handle=start;descriptor_callback=cb;callback_arg=arg;return 0;
}
int ble_gattc_write_flat(uint16_t conn,uint16_t handle,const void *data,uint16_t size,ble_gatt_attr_fn *cb,void *arg) {
    assert(conn==7 && size<=sizeof(last_write));calls++;last_handle=handle;last_size=size;
    memcpy(last_write,data,size);write_callback=cb;callback_arg=arg;return 0;
}
int ble_uuid_cmp(const ble_uuid_t *a,const ble_uuid_t *b) {
    if(a->type!=b->type)return 1;
    if(a->type==BLE_UUID_TYPE_16)return BLE_UUID16(a)->value!=BLE_UUID16(b)->value;
    return memcmp(BLE_UUID128(a)->value,BLE_UUID128(b)->value,16);
}
int os_mbuf_copydata(const struct os_mbuf *om,int offset,int len,void *out) {
    if(offset<0 || len<0 || (unsigned)(offset+len)>OS_MBUF_PKTLEN(om))return -1;
    memcpy(out,om->om_data+offset,len);return 0;
}
static const struct ble_gatt_error ok={0}, done={.status=BLE_HS_EDONE};
static void service_result(bool exists) {
    struct ble_gatt_svc svc={.start_handle=want_gatt?1:10,.end_handle=want_gatt?9:40};
    if(exists)service_callback(7,&ok,&svc,callback_arg);
    service_callback(7,&done,NULL,callback_arg);
}
static void characteristic(uint16_t def,uint16_t value,const ble_uuid_t *uuid,uint8_t properties) {
    struct ble_gatt_chr chr={.def_handle=def,.val_handle=value,.properties=properties};
    if(uuid->type==BLE_UUID_TYPE_16)chr.uuid.u16=*BLE_UUID16(uuid);else chr.uuid.u128=*BLE_UUID128(uuid);
    character_callback(7,&ok,&chr,callback_arg);
}
static void descriptor_result(void) {
    struct ble_gatt_dsc dsc={.handle=last_handle+1};dsc.uuid.u16=(ble_uuid16_t)BLE_UUID16_INIT(0x2902);
    descriptor_callback(7,&ok,last_handle,&dsc,callback_arg);
    descriptor_callback(7,&done,last_handle,NULL,callback_arg);
}
static void write_result(int status) {
    struct ble_gatt_error error={.status=status};write_callback(7,&error,NULL,callback_arg);
}
static void notify(uint16_t attr,const uint8_t *data,unsigned size) {
    struct {struct os_mbuf om;struct os_mbuf_pkthdr header;} packet={0};
    packet.om.om_data=(uint8_t*)data;packet.header.omp_len=size;
    gopine_ancs_notify(7,attr,&packet.om);
}
static void setup(void) {
    gopine_ancs_reset();calls=0;now=0;
    gopine_ancs_poll(7,false);assert(!calls); // No discovery before trust.
    gopine_ancs_poll(7,true);assert(want_gatt);service_result(true);
    gopine_ancs_poll(7,true);characteristic(3,4,BLE_UUID16_DECLARE(0x2a05),BLE_GATT_CHR_PROP_INDICATE);
    character_callback(7,&done,NULL,callback_arg);
    gopine_ancs_poll(7,true);descriptor_result();gopine_ancs_poll(7,true);
    assert(last_handle==5 && last_write[0]==2);write_result(0);
    gopine_ancs_poll(7,true);assert(!want_gatt);service_result(true);
    gopine_ancs_poll(7,true);
    characteristic(11,12,&source_uuid.u,BLE_GATT_CHR_PROP_NOTIFY);
    characteristic(20,21,&control_uuid.u,BLE_GATT_CHR_PROP_WRITE);
    characteristic(30,31,&data_uuid.u,BLE_GATT_CHR_PROP_NOTIFY);
    character_callback(7,&done,NULL,callback_arg);
    assert(client.source.end==19 && client.data.end==40);
    gopine_ancs_poll(7,true);assert(last_handle==12);descriptor_result();
    gopine_ancs_poll(7,true);assert(last_handle==31);descriptor_result();
    gopine_ancs_poll(7,true);assert(last_handle==32 && last_write[0]==1);write_result(0);
    gopine_ancs_poll(7,true);assert(last_handle==13);write_result(0);
    assert(client.ready && client.stage==READY);
    uint8_t out[ANCS_RECORD_SIZE];assert(gopine_ancs_take(out,true) && out[4]==3);
    assert(!gopine_ancs_take(out,true));
}
static void source(uint32_t uid,uint8_t event,uint8_t flags) {
    uint8_t data[]={event,flags,4,1,uid,uid>>8,uid>>16,uid>>24};notify(12,data,8);
}
static void response(uint32_t uid) {
    uint8_t data[]={0,uid,uid>>8,uid>>16,uid>>24,1,3,0,'A','p','p',3,5,0,'H','e','l','l','o'};
    // Split every header and length across ATT notifications.
    for(unsigned i=0;i<sizeof(data);i++)notify(31,data+i,1);
}
static void transfer_and_session(void) {
    setup();source(0x12345678,0,4);gopine_ancs_poll(7,true);
    assert(last_handle==21 && last_size==11 && last_write[5]==1 && last_write[8]==3);
    write_result(0);response(0x12345678);
    uint8_t out[ANCS_RECORD_SIZE];assert(gopine_ancs_take(out,true)==ANCS_RECORD_SIZE);
    assert(ancs_u32(out)==0x12345678 && out[4]==0 && out[5]==4 && out[6]==4);
    assert(!memcmp(out+7,"App",3) && !memcmp(out+47,"Hello",5));
    source(12,0,0);gopine_ancs_poll(7,true);write_result(0);
    source(12,2,0);response(12);assert(gopine_ancs_take(out,true) && out[4]==2);
    assert(!gopine_ancs_take(out,true)); // Removed in-flight attributes cannot resurrect it.
    source(19,0,0);gopine_ancs_poll(7,true);
    ble_gatt_attr_fn *stale=write_callback;void *arg=callback_arg;
    gopine_ancs_reset();gopine_ancs_poll(7,true);
    stale(7,&ok,NULL,arg);assert(client.stage==GATT_SERVICE && client.busy);
    assert(gopine_ancs_take(out,false) && out[4]==3);
}
static void bounds_and_deadlines(void) {
    setup();source(1,0,0);gopine_ancs_poll(7,true);write_result(0);
    const uint8_t bad[]={0,1,0,0,0,1,41,0};notify(31,bad,sizeof(bad));assert(client.stage==FAILED);
    unsigned before=calls;now+=60000;gopine_ancs_poll(7,true);assert(calls==before);
    const uint8_t changed[]={1,0,40,0};notify(4,changed,4);gopine_ancs_poll(7,true);
    assert(want_gatt && client.stage==GATT_SERVICE);
    gopine_ancs_reset();gopine_ancs_poll(7,true);service_result(false);
    gopine_ancs_poll(7,true);service_result(false);assert(client.stage==WAIT_SERVICE);
    before=calls;gopine_ancs_poll(7,true);assert(calls==before && gopine_ancs_delay()==30000);
    now+=30000;gopine_ancs_poll(7,true);assert(calls==before+1);
    now+=15000;gopine_ancs_poll(7,true);assert(client.stage==FAILED);
    assert(gopine_ancs_delay()==240000);
    setup();for(uint32_t i=1;i<12;i++)source(i,0,0);assert(client.pending_count==4);
    gopine_ancs_poll(7,true);assert(client.response.uid==8); // Bound bursts to newest four.
    write_result(BLE_HS_ATT_ERR(0xa2));assert(!client.response.active && client.ready);
    gopine_ancs_poll(7,true);assert(client.response.uid==9);
    setup();source(1,0,0);gopine_ancs_poll(7,true);write_result(0);
    uint8_t full[ANCS_RESPONSE_SIZE]={0,1,0,0,0,1,40,0};
    memset(full+8,'T',40);full[48]=3;full[49]=100;memset(full+51,'B',100);
    notify(31,full,sizeof(full));uint8_t out[ANCS_RECORD_SIZE];
    assert(gopine_ancs_take(out,true) && out[46]=='T' && out[146]=='B');
    setup();now=UINT32_MAX-5000;source(1,0,0);gopine_ancs_poll(7,true);write_result(0);
    now+=14999;gopine_ancs_poll(7,true);assert(client.ready && client.response.active);
    now++;gopine_ancs_poll(7,true);assert(client.stage==FAILED); // Deadline wrap.
    setup();source(1,0,0);gopine_ancs_poll(7,true);write_result(0);
    const uint8_t extra[]={0,1,0,0,0,1,0,0,3,9,0,'1','2','3','4','5','6','7','8','9',0};
    notify(31,extra,sizeof(extra));assert(client.stage==FAILED && !client.mailbox.count);
}
static void parser_fragmentation(void) {
    uint8_t input[]={0,8,0,0,0,1,3,0,'A','p','p',3,5,0,'H','e','l','l','o'};
    uint8_t source_data[]={0,0,0,1,8,0,0,0};
    for(unsigned split=1;split<sizeof(input);split++) {
        struct ancs_response r;struct ancs_mailbox box={0};uint8_t out[ANCS_RECORD_SIZE];
        ancs_response_begin(&r,source_data);
        assert(!ancs_response_feed(&r,&box,input,split));
        assert(ancs_response_feed(&r,&box,input+split,sizeof(input)-split)==1);
        assert(ancs_take(&box,out) && !memcmp(out+47,"Hello",5));
    }
}
int main(void) { transfer_and_session();bounds_and_deadlines();parser_fragmentation(); }
