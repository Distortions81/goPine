// iOS ANCS consumer, separate from the watch's ANS server. All procedures run
// cooperatively after the host callback unwinds; callbacks only advance state.
#include "ancs.h"

static const ble_uuid128_t service_uuid=BLE_UUID128_INIT(0xd0,0x00,0x2d,0x12,0x1e,0x4b,0x0f,0xa4,0x99,0x4e,0xce,0xb5,0x31,0xf4,0x05,0x79);
static const ble_uuid128_t source_uuid=BLE_UUID128_INIT(0xbd,0x1d,0xa2,0x99,0xe6,0x25,0x58,0x8c,0xd9,0x42,0x01,0x63,0x0d,0x12,0xbf,0x9f);
static const ble_uuid128_t control_uuid=BLE_UUID128_INIT(0xd9,0xd9,0xaa,0xfd,0xbd,0x9b,0x21,0x98,0xa8,0x49,0xe1,0x45,0xf3,0xd8,0xd1,0x69);
static const ble_uuid128_t data_uuid=BLE_UUID128_INIT(0xfb,0x7b,0x7c,0xce,0x6a,0xb3,0x44,0xbe,0xb5,0x4b,0xd6,0x24,0xe9,0xc6,0xea,0x22);
enum stage { IDLE, GATT_SERVICE, GATT_CHARS, GATT_DESC, GATT_SUB,
    ANCS_SERVICE, ANCS_CHARS, SOURCE_DESC, DATA_DESC, DATA_SUB, SOURCE_SUB, READY, WAIT_SERVICE, FAILED };
struct characteristic { uint16_t value,end,cccd; };
static struct {
    uint16_t conn,start,end,control;
    struct characteristic changed,source,data;
    enum stage stage;
    uint32_t epoch,deadline,retry_at;
    unsigned retries;
    bool busy,ready,restart;
    uint8_t pending[4][8],pending_count;
    struct ancs_response response;
    struct ancs_mailbox mailbox;
} client={.conn=BLE_HS_CONN_HANDLE_NONE};

static void *token(void) { return (void*)(uintptr_t)client.epoch; }
static bool current(uint16_t conn,void *arg) {
    return conn==client.conn && (uintptr_t)arg==client.epoch;
}
static void fail(void) {
    client.busy=client.ready=false; client.stage=FAILED;
    client.pending_count=0; memset(&client.response,0,sizeof(client.response));
}
void gopine_ancs_reset(void) {
    uint32_t epoch=client.epoch+1;
    if(!epoch) epoch=1;
    memset(&client,0,sizeof(client)); client.epoch=epoch;
    client.conn=BLE_HS_CONN_HANDLE_NONE; ancs_clear(&client.mailbox);
}
static void retry_service(void) {
    client.busy=false; client.stage=WAIT_SERVICE;
    client.retry_at=ble_npl_time_get()+30000;
}
static int service_cb(uint16_t conn,const struct ble_gatt_error *error,
                       const struct ble_gatt_svc *svc,void *arg) {
    if(!current(conn,arg)) return 0;
    if(!error->status && svc) { client.start=svc->start_handle; client.end=svc->end_handle; return 0; }
    client.busy=false;
    if(error->status!=BLE_HS_EDONE) { fail(); return 0; }
    if(client.stage==GATT_SERVICE) client.stage=client.start?GATT_CHARS:ANCS_SERVICE;
    else if(client.stage==ANCS_SERVICE) {
        if(client.start) client.stage=ANCS_CHARS; else retry_service();
    }
    return 0;
}
static void finish_characteristic(struct characteristic *c,uint16_t next) {
    if(c->value && !c->end && next>c->value) c->end=next-1;
}
static int chr_cb(uint16_t conn,const struct ble_gatt_error *error,
                   const struct ble_gatt_chr *chr,void *arg) {
    if(!current(conn,arg)) return 0;
    if(!error->status && chr) {
        finish_characteristic(&client.changed,chr->def_handle);
        finish_characteristic(&client.source,chr->def_handle);
        finish_characteristic(&client.data,chr->def_handle);
        if(client.stage==GATT_CHARS) {
            if(!ble_uuid_cmp(&chr->uuid.u,BLE_UUID16_DECLARE(0x2a05)) && (chr->properties & BLE_GATT_CHR_PROP_INDICATE)) client.changed.value=chr->val_handle;
        } else {
            if(!ble_uuid_cmp(&chr->uuid.u,&source_uuid.u) && (chr->properties & BLE_GATT_CHR_PROP_NOTIFY)) client.source.value=chr->val_handle;
            if(!ble_uuid_cmp(&chr->uuid.u,&data_uuid.u) && (chr->properties & BLE_GATT_CHR_PROP_NOTIFY)) client.data.value=chr->val_handle;
            if(!ble_uuid_cmp(&chr->uuid.u,&control_uuid.u) && (chr->properties & BLE_GATT_CHR_PROP_WRITE)) client.control=chr->val_handle;
        }
        return 0;
    }
    client.busy=false;
    if(error->status!=BLE_HS_EDONE) { fail(); return 0; }
    struct characteristic *chars[]={&client.changed,&client.source,&client.data};
    for(unsigned i=0;i<3;i++) if(chars[i]->value && !chars[i]->end) chars[i]->end=client.end;
    if(client.stage==GATT_CHARS) client.stage=client.changed.value?GATT_DESC:ANCS_SERVICE;
    else if(client.source.value && client.data.value && client.control) client.stage=SOURCE_DESC;
    else fail();
    return 0;
}
static struct characteristic *descriptor_target(void) {
    if(client.stage==GATT_DESC || client.stage==GATT_SUB) return &client.changed;
    if(client.stage==SOURCE_DESC || client.stage==SOURCE_SUB) return &client.source;
    return &client.data;
}
static int desc_cb(uint16_t conn,const struct ble_gatt_error *error,uint16_t value,
                    const struct ble_gatt_dsc *dsc,void *arg) {
    if(!current(conn,arg)) return 0;
    struct characteristic *target=descriptor_target();
    if(!error->status && dsc) {
        if(value==target->value && !ble_uuid_cmp(&dsc->uuid.u,BLE_UUID16_DECLARE(0x2902))) target->cccd=dsc->handle;
        return 0;
    }
    client.busy=false;
    if(error->status!=BLE_HS_EDONE || !target->cccd) {
        if(client.stage==GATT_DESC) client.stage=ANCS_SERVICE; else fail();
        return 0;
    }
    switch(client.stage) {
    case GATT_DESC: client.stage=GATT_SUB; break;
    case SOURCE_DESC: client.stage=DATA_DESC; break;
    case DATA_DESC: client.stage=DATA_SUB; break;
    default: fail();
    }
    return 0;
}
static int write_cb(uint16_t conn,const struct ble_gatt_error *error,
                     struct ble_gatt_attr *attr,void *arg) {
    (void)attr;
    if(!current(conn,arg)) return 0;
    client.busy=false;
    if(error->status) {
        if(client.stage==GATT_SUB) client.stage=ANCS_SERVICE;
        else if(!client.ready) retry_service();
        else if(error->status==BLE_HS_ATT_ERR(0xa2)) {
            // The notification disappeared before its attributes were read.
            // ANCS sends no data for a rejected command; move to the next UID.
            memset(&client.response,0,sizeof(client.response));
        }
        else fail(); // Late attribute fragments must not be reused for a new UID.
        return 0;
    }
    switch(client.stage) {
    case GATT_SUB: client.stage=ANCS_SERVICE; break;
    case DATA_SUB: client.stage=SOURCE_SUB; break;
    case SOURCE_SUB: client.stage=READY; client.ready=true; break;
    case READY: break;
    default: fail();
    }
    return 0;
}
static bool due(uint32_t now,uint32_t deadline) { return (int32_t)(now-deadline)>=0; }
void gopine_ancs_poll(uint16_t conn,bool trusted) {
    if(conn==BLE_HS_CONN_HANDLE_NONE || !trusted) return;
    if(client.conn!=conn) {
        gopine_ancs_reset(); client.conn=conn; client.stage=GATT_SERVICE;
    }
    uint32_t now=ble_npl_time_get();
    if(client.restart && !client.busy) {
        gopine_ancs_reset(); client.conn=conn; client.stage=GATT_SERVICE;
    }
    if(client.busy) { if(due(now,client.deadline)) fail(); return; }
    if(client.response.active) { if(due(now,client.deadline)) fail(); return; }
    if(client.stage==WAIT_SERVICE) {
        if(client.retries>=3 || !due(now,client.retry_at)) return;
        client.retries++; client.stage=ANCS_SERVICE;
        memset(&client.source,0,sizeof(client.source)); memset(&client.data,0,sizeof(client.data)); client.control=0;
    }
    int rc=0;
    switch(client.stage) {
    case GATT_SERVICE: case ANCS_SERVICE:
        client.start=client.end=0;
        rc=ble_gattc_disc_svc_by_uuid(conn,client.stage==GATT_SERVICE?BLE_UUID16_DECLARE(0x1801):&service_uuid.u,service_cb,token()); break;
    case GATT_CHARS: case ANCS_CHARS:
        rc=ble_gattc_disc_all_chrs(conn,client.start,client.end,chr_cb,token()); break;
    case GATT_DESC: case SOURCE_DESC: case DATA_DESC: {
        struct characteristic *c=descriptor_target();
        if(c->value>=c->end) { if(client.stage==GATT_DESC) client.stage=ANCS_SERVICE; else fail(); return; }
        rc=ble_gattc_disc_all_dscs(conn,c->value,c->end,desc_cb,token()); break;
    }
    case GATT_SUB: case DATA_SUB: case SOURCE_SUB: {
        const uint8_t enable[]={client.stage==GATT_SUB?2:1,0};
        rc=ble_gattc_write_flat(conn,descriptor_target()->cccd,enable,sizeof(enable),write_cb,token()); break;
    }
    case READY: {
        if(!client.pending_count) return;
        uint8_t source[8]; memcpy(source,client.pending[0],8);
        memmove(client.pending,client.pending+1,(--client.pending_count)*8);
        ancs_response_begin(&client.response,source);
        uint8_t request[]={0,source[4],source[5],source[6],source[7],1,40,0,3,100,0};
        rc=ble_gattc_write_flat(conn,client.control,request,sizeof(request),write_cb,token()); break;
    }
    default: return;
    }
    if(rc) { fail(); return; }
    client.busy=true; client.deadline=now+15000;
}
void gopine_ancs_notify(uint16_t conn,uint16_t attr,struct os_mbuf *om) {
    if(conn!=client.conn || !om) return;
    unsigned n=OS_MBUF_PKTLEN(om);
    if(attr==client.changed.value && n==4) { client.restart=true; return; }
    if(attr==client.source.value && (client.stage==SOURCE_SUB || client.ready) && n==8) {
        uint8_t source[8]; if(os_mbuf_copydata(om,0,8,source)) return;
        if(source[0]>2 || source[2]>11) return;
        uint32_t uid=ancs_u32(source+4);
        for(unsigned i=0;i<client.pending_count;) {
            if(ancs_u32(client.pending[i]+4)==uid) {
                memmove(client.pending+i,client.pending+i+1,(--client.pending_count-i)*8);
            } else i++;
        }
        if(source[0]==2) {
            if(client.response.active && client.response.uid==uid) client.response.removed=true;
            uint8_t record[ANCS_RECORD_SIZE]={0}; memcpy(record,source+4,4); record[4]=2;
            ancs_store(&client.mailbox,record); return;
        }
        if(client.pending_count==4) { memmove(client.pending,client.pending+1,3*8); client.pending_count--; }
        memcpy(client.pending[client.pending_count++],source,8);
    } else if(attr==client.data.value && client.ready && client.response.active) {
        // Validate the entire ATT fragment before publishing a completed record;
        // an extra byte after its final tuple must not escape as a valid message.
        uint8_t bytes[ANCS_RESPONSE_SIZE];
        if(n>sizeof(bytes) || os_mbuf_copydata(om,0,n,bytes) ||
           ancs_response_feed(&client.response,&client.mailbox,bytes,n)<0) fail();
    }
}
int gopine_ancs_take(uint8_t *out,bool trusted) {
    if(!trusted && !client.mailbox.clear) return 0;
    return ancs_take(&client.mailbox,out);
}
bool gopine_ancs_updates(void) { return client.mailbox.clear || client.mailbox.count; }
uint32_t gopine_ancs_delay(void) {
    uint32_t now=ble_npl_time_get();
    if(client.busy || client.response.active) return due(now,client.deadline)?0:client.deadline-now;
    if(client.stage==WAIT_SERVICE) return client.retries>=3?240000:(due(now,client.retry_at)?0:client.retry_at-now);
    if(client.stage==FAILED || client.stage==IDLE || (client.stage==READY && !client.pending_count && !client.restart)) return 240000;
    return 0;
}
