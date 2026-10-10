// Real pinned host + production service; controller capabilities, HCI, hardware
// and flash are simulated. No physical radio timing or Go stack is exercised.
#include <assert.h>
#include <stdio.h>
#include "../port/service.c"
#include "ble_hs_priv.h"
#include "ble_sm_priv.h"
#include "../port/bond_journal.h"
#include "transport/ram/ble_hci_ram.h"
#include "tinycrypt/aes.h"
#include "tinycrypt/ecc.h"
#include "tinycrypt/ecc_dh.h"
#include "companion_fixtures.h"

test_rtc test_rtc0;
test_scb test_scb0;
test_ficr test_ficr0 = {{0x157a69b5, 0xc99e}};
uint32_t test_primask;
static struct ble_npl_eventq test_hostq;
static unsigned commands;
static uint8_t reply[512];
static unsigned reply_len;
static uint32_t flash[BOND_PAGE_SIZE/4];
static uint8_t controller_ltk[16];
static unsigned ltk_replies;
static uint8_t peer_address[6]={1,2,3,4,5,0xc0};
static bool controller_resolving;
static struct ble_hci_le_add_resolv_list_cp resolving_entries[2];
static unsigned resolving_count, controller_resets;
static unsigned flash_writes;
static bool reject_peer_irk;
// Capture real outgoing client requests separately from the watch's server
// responses. A fake iOS GATT server answers these without calling ANCS callbacks.
struct client_request { uint8_t data[247]; unsigned size; };
static struct client_request client_requests[8];
static unsigned client_request_count, outgoing_start, indication_confirmations;
static bool retain_apple;
static struct ble_npl_eventq crypto_controller;
static struct ble_npl_event crypto_event, crypto_host_event;
static unsigned crypto_yields, crypto_controller_runs;
static bool in_crypto;
static void crypto_controller_callback(struct ble_npl_event *ev) {
    (void)ev;assert(in_crypto);crypto_controller_runs++;
}
static void crypto_host_callback(struct ble_npl_event *ev) {
    (void)ev;assert(!in_crypto);
}
void gopine_test_ecc_yield(void) {
    in_crypto=true;crypto_yields++;
    // Model an IRQ queuing link-layer work during scalar multiplication,
    // together with host data that must remain deferred until SM unwinds.
    ble_npl_eventq_put(&crypto_controller,&crypto_event);
    ble_npl_eventq_put(&test_hostq,&crypto_host_event);
    gopine_ble_controller_pump();
    assert(crypto_controller_runs==crypto_yields);
    assert(ble_npl_event_is_queued(&crypto_host_event));
    in_crypto=false;
}

void nimble_port_init(void) {
    extern void os_msys_init(void);
    ble_npl_eventq_init(&test_hostq);
    os_msys_init(); ble_hs_init(); ble_hci_ram_init();
}
struct ble_npl_eventq *nimble_port_get_dflt_eventq(void) { return &test_hostq; }
int ble_phy_init(void) { return 0; }
int ble_phy_txpwr_set(int dbm) { (void)dbm; return 0; }
void ble_phy_disable(void) {}
void ble_phy_rfclk_disable(void) {}
int ble_hw_rng_stop(void) { return 0; }
int ble_hw_get_static_addr(ble_addr_t *addr) {
    addr->type=BLE_ADDR_RANDOM;memset(addr->val,0xc0,sizeof(addr->val));return 0;
}
int ble_ll_reset(void) { controller_resolving=false;resolving_count=0;return 0; }
int ble_ll_rand_data_get(uint8_t *buf,uint8_t len) { memset(buf,42,len); return 0; }
uint32_t bond_flash_word(unsigned offset) {
    assert(offset<BOND_PAGE_SIZE && offset%4==0);return flash[offset/4];
}
bool bond_flash_write(unsigned offset,uint32_t word) {
    assert(offset<BOND_PAGE_SIZE && offset%4==0);
    assert((flash[offset/4]&word)==word);flash[offset/4]=word;flash_writes++;return true;
}
bool bond_flash_erase(void) { memset(flash,0xff,sizeof(flash));return true; }
int ble_hw_encrypt_block(struct ble_encryption_block *block) {
    struct tc_aes_key_sched_struct keys;
    return tc_aes128_set_encrypt_key(&keys,block->key) &&
           tc_aes_encrypt(block->cipher_text,block->plain_text,&keys) ? 0 : -1;
}
static void send_event(const void *data,unsigned size) {
    uint8_t *buf=ble_hci_trans_buf_alloc(BLE_HCI_TRANS_BUF_EVT_HI);
    assert(buf); memcpy(buf,data,size); assert(ble_hci_trans_ll_evt_tx(buf)==0);
}
void ble_ll_hci_send_noop(void) {
    uint8_t evt[]={BLE_HCI_EVCODE_COMMAND_COMPLETE,3,1,0,0};send_event(evt,sizeof(evt));
}
int ble_ll_hci_cmd_rx(uint8_t *cmd,void *arg) {
    (void)arg;commands++;
    uint16_t opcode=get_le16(cmd);
    uint8_t evt[96]={BLE_HCI_EVCODE_COMMAND_COMPLETE,4,1,0,0,0};
    put_le16(evt+3,opcode);
    uint16_t handle=get_le16(cmd+3);
    switch(opcode) {
    case BLE_HCI_OP(BLE_HCI_OGF_CTLR_BASEBAND,BLE_HCI_OCF_CB_RESET):
        controller_resolving=false;resolving_count=0;controller_resets++;
        break;
    case BLE_HCI_OP(BLE_HCI_OGF_INFO_PARAMS,BLE_HCI_OCF_IP_RD_LOCAL_VER):
        evt[1]+=8;evt[6]=BLE_HCI_VER_BCS_5_0;break;
    case BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_RD_BUF_SIZE):
        evt[1]+=3;put_le16(evt+6,64);evt[8]=4;break;
    case BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_RD_LOC_SUPP_FEAT):
        evt[1]+=8;evt[6]=0x41; // Encryption and controller privacy.
        break;
    case BLE_HCI_OP(BLE_HCI_OGF_INFO_PARAMS,BLE_HCI_OCF_IP_RD_BD_ADDR): {
        const uint8_t address[6]={0xb5,0x69,0x7a,0x15,0x9e,0xc9};
        evt[1]+=6;memcpy(evt+6,address,6);break;
    }
    case BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_SET_ADDR_RES_EN):
        controller_resolving=cmd[3]!=0;break;
    case BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_CLR_RESOLV_LIST):
        resolving_count=0;break;
    case BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_ADD_RESOLV_LIST): {
        struct ble_hci_le_add_resolv_list_cp entry;memcpy(&entry,cmd+3,sizeof(entry));
        if(reject_peer_irk && entry.peer_id_addr[5]==0xc0) {
            evt[5]=BLE_ERR_MEM_CAPACITY;break;
        }
        unsigned at=0;
        for(;at<resolving_count;at++)
            if(entry.peer_addr_type==resolving_entries[at].peer_addr_type &&
               !memcmp(entry.peer_id_addr,resolving_entries[at].peer_id_addr,6))break;
        assert(at<ARRAY_SIZE(resolving_entries));resolving_entries[at]=entry;
        if(at==resolving_count)resolving_count++;
        break;
    }
    }
    if(opcode==BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_LT_KEY_REQ_REPLY)) {
        memcpy(controller_ltk,cmd+5,16);ltk_replies++;
        evt[1]+=2;put_le16(evt+6,handle);
    }
    if(opcode==BLE_HCI_OP(BLE_HCI_OGF_LE,BLE_HCI_OCF_LE_RAND)) {
        evt[1]+=8;memset(evt+6,42,8); // Deterministic controller entropy in tests.
    }
    ble_hci_trans_buf_free(cmd);send_event(evt,evt[1]+2);
    if(opcode==BLE_HCI_OP(BLE_HCI_OGF_LINK_CTRL,BLE_HCI_OCF_DISCONNECT_CMD)) {
        uint8_t disconnected[]={BLE_HCI_EVCODE_DISCONN_CMP,4,0,0,0,BLE_ERR_REM_USER_CONN_TERM};
        put_le16(disconnected+3,handle);send_event(disconnected,sizeof(disconnected));
    }
    return 0;
}
int ble_ll_hci_acl_rx(struct os_mbuf *om,void *arg) {
    (void)arg;
    unsigned size=OS_MBUF_PKTLEN(om);
    uint8_t header[4];assert(os_mbuf_copydata(om,0,4,header)==0);
    unsigned skip=(get_le16(header)&0x3000)==0x1000?4:0;
    if(!skip)outgoing_start=reply_len;
    assert(reply_len+size-skip<=sizeof(reply));
    assert(os_mbuf_copydata(om,skip,size-skip,reply+reply_len)==0);reply_len+=size-skip;
    if(reply_len-outgoing_start>=8 &&
       reply_len-outgoing_start==8u+get_le16(reply+outgoing_start+4) &&
       get_le16(reply+outgoing_start+6)==BLE_L2CAP_CID_ATT) {
        const uint8_t *att=reply+outgoing_start+8;
        unsigned n=get_le16(reply+outgoing_start+4);
        if(att[0]==BLE_ATT_OP_FIND_TYPE_VALUE_REQ || att[0]==BLE_ATT_OP_READ_TYPE_REQ ||
           att[0]==BLE_ATT_OP_FIND_INFO_REQ || att[0]==BLE_ATT_OP_WRITE_REQ) {
            assert(client_request_count<ARRAY_SIZE(client_requests) && n<=247);
            struct client_request *request=&client_requests[client_request_count++];
            memcpy(request->data,att,n);request->size=n;
        } else if(att[0]==BLE_ATT_OP_INDICATE_RSP)indication_confirmations++;
    }
    os_mbuf_free_chain(om);
    uint8_t completed[]={BLE_HCI_EVCODE_NUM_COMP_PKTS,5,1,1,0,1,0};
    send_event(completed,sizeof(completed));return 0;
}
static void connect_peer(void) {
    client_request_count=0;
    struct ble_hci_ev_le_subev_enh_conn_complete evt={.subev_code=BLE_HCI_LE_SUBEV_ENH_CONN_COMPLETE,
        .conn_handle=1,.role=BLE_HCI_LE_CONN_COMPLETE_ROLE_SLAVE,
        .peer_addr_type=BLE_ADDR_RANDOM,.peer_addr={1,2,3,4,5,0xc0},
        .conn_itvl=24,.supervision_timeout=200};
    memcpy(evt.peer_addr,peer_address,sizeof(peer_address));
    if((peer_address[5]&0xc0)==0x40 && controller_resolving) {
        // Model controller identity resolution using the IRK actually installed
        // by host HCI commands, rather than handing the host a known identity.
        for(unsigned i=0;i<resolving_count;i++) {
            struct ble_encryption_block ecb={0};
            for(unsigned k=0;k<16;k++)ecb.key[k]=resolving_entries[i].peer_irk[15-k];
            for(unsigned k=0;k<3;k++)ecb.plain_text[15-k]=peer_address[3+k];
            assert(!ble_hw_encrypt_block(&ecb));
            if(ecb.cipher_text[15]==peer_address[0] && ecb.cipher_text[14]==peer_address[1] &&
               ecb.cipher_text[13]==peer_address[2]) {
                evt.peer_addr_type=resolving_entries[i].peer_addr_type+2;
                memcpy(evt.peer_addr,resolving_entries[i].peer_id_addr,6);
                memcpy(evt.peer_rpa,peer_address,6);break;
            }
        }
    }
    uint8_t packet[sizeof(evt)+2]={BLE_HCI_EVCODE_LE_META,sizeof(evt)};
    for(unsigned i=0;i<8 && gopine_ble_host_work_pending();i++)gopine_ble_poll();
    uint8_t apple[147],music[87];
    if(!retain_apple)gopine_ble_take_apple_notification(apple);
    gopine_ble_take_music(music);
    gopine_ble_ack_updates();
    assert(!gopine_ble_updates());
    reply_len=0;memcpy(packet+2,&evt,sizeof(evt));send_event(packet,sizeof(packet));
    gopine_ble_controller_poll();assert(connection==BLE_HS_CONN_HANDLE_NONE);
    // Mirror Wait's controller-only pass: a raw HCI event must return control
    // to Go even before any service callback has produced an app snapshot.
    assert(gopine_ble_host_work_pending());
    assert(gopine_ble_updates());
    gopine_ble_poll();
    assert(connection==1);
}
static void receive_l2cap(uint16_t cid,const uint8_t *value,unsigned size) {
    for(unsigned i=0;i<8 && gopine_ble_host_work_pending();i++)gopine_ble_poll();
    // The production controller disables data-length extension. Its incoming
    // HCI ACL packets therefore carry at most 27 bytes, including the initial
    // L2CAP header. Exercise actual host reassembly, particularly the 65-byte
    // Secure Connections public key and long ANCS attribute responses.
    uint8_t pdu[251];assert(size+4<=sizeof(pdu));
    put_le16(pdu,size);put_le16(pdu+2,cid);memcpy(pdu+4,value,size);
    uint8_t apple[147],music[87];
    if(!retain_apple)gopine_ble_take_apple_notification(apple);
    gopine_ble_take_music(music);
    gopine_ble_ack_updates();
    assert(!gopine_ble_updates() || retain_apple);
    reply_len=0;
    for(unsigned offset=0;offset<size+4;) {
        unsigned count=size+4-offset;if(count>27)count=27;
        struct os_mbuf *om=os_msys_get_pkthdr(count+4,0);assert(om);
        uint8_t header[4];put_le16(header,1|(offset?0x1000:0x2000));
        put_le16(header+2,count);
        assert(!os_mbuf_append(om,header,sizeof(header)));
        assert(!os_mbuf_append(om,pdu+offset,count));
        assert(!ble_hci_trans_ll_acl_tx(om));
        gopine_ble_controller_poll();
        assert(gopine_ble_host_work_pending() && gopine_ble_updates());
        gopine_ble_poll();offset+=count;
    }
}
static void att(const uint8_t *value,unsigned size,uint8_t response) {
    receive_l2cap(BLE_L2CAP_CID_ATT,value,size);
    if(reply_len<9 || get_le16(reply+6)!=BLE_L2CAP_CID_ATT || reply[8]!=response)
        fprintf(stderr,"ATT request %u length %u: expected %u, reply length %u opcode %u\n",
                value[0],size,response,reply_len,reply_len>=9?reply[8]:0);
    assert(reply_len>=9 && get_le16(reply+6)==BLE_L2CAP_CID_ATT);
    assert(reply[8]==response);
}
static void discover(void) {
    const uint8_t mtu[]={2,247,0};att(mtu,sizeof(mtu),3);
    // Enumerate primary services and all characteristic declarations, as
    // BlueZ does before an updater can access its custom characteristics.
    for(unsigned kind=0;kind<2;kind++) {
        uint16_t start=1;
        do {
            uint8_t request[]={kind?8:16,0,0,255,255,kind?3:0,0x28};
            put_le16(request+1,start);
            receive_l2cap(BLE_L2CAP_CID_ATT,request,sizeof(request));
            assert(reply_len>=9);
            if(reply[8]==1) {assert(reply[12]==BLE_ATT_ERR_ATTR_NOT_FOUND);break;}
            assert(reply[8]==(kind?9:17));
            unsigned length=reply[9];assert(length>=6);
            unsigned end=8+get_le16(reply+4);assert(end<=reply_len);
            unsigned last=end-length;
            start=get_le16(reply+last+(kind?0:2))+1;
        } while(start);
    }
    uint16_t handle;
    assert(ble_gatts_find_chr(BLE_UUID16_DECLARE(0x180f),BLE_UUID16_DECLARE(0x2a19),NULL,&handle)==0);
    uint8_t read[]={10,0,0};put_le16(read+1,handle);att(read,sizeof(read),11);
    assert(reply[9]==80); // Battery must not trigger pairing in update mode.
    assert(ble_gatts_find_chr(&update_uuids[0].u,&update_uuids[3].u,NULL,&handle)==0);
    put_le16(read+1,handle);att(read,sizeof(read),11);
    assert(reply_len==25 && reply[9]==1 && reply[10]==1);
}
static void phone_pairing_code(bool timeout) {
    uint8_t time[10]={0};
    assert(gopine_ble_start_phone(time,80,0)==0);gopine_ble_poll();connect_peer();
    assert(phone_window && !update_window && !security_pending);
    assert(reply_len>=9 && get_le16(reply+6)==BLE_L2CAP_CID_SM && reply[8]==BLE_SM_OP_SEC_REQ);
    uint16_t handle;
    assert(ble_gatts_find_chr(BLE_UUID16_DECLARE(0x180f),BLE_UUID16_DECLARE(0x2a19),NULL,&handle)==0);
    uint8_t read[]={10,0,0};put_le16(read+1,handle);att(read,sizeof(read),1);
    assert(reply[12]==BLE_ATT_ERR_INSUFFICIENT_AUTHEN);
    uint8_t pairing[]={BLE_SM_OP_PAIR_REQ,BLE_SM_IO_CAP_KEYBOARD_DISP,0,0x0d,16,3,3};
    receive_l2cap(BLE_L2CAP_CID_SM,pairing,sizeof(pairing));
    assert(reply_len>=9 && reply[8]==BLE_SM_OP_PAIR_RSP);
    uint8_t private[32]={0},public[64],packet[65]={BLE_SM_OP_PAIR_PUBLIC_KEY};
    private[31]=3;
    assert(uECC_compute_public_key(private,public,uECC_secp256r1())==1);
    for(unsigned i=0;i<32;i++) {packet[1+i]=public[31-i];packet[33+i]=public[63-i];}
    receive_l2cap(BLE_L2CAP_CID_SM,packet,sizeof(packet));
    assert(pairing_code>0 && pairing_code<=1000000 && !pairing_io_pending);
    assert(connection==1);
    if(timeout) {
        assert(gopine_ble_pairing_progress()==0x200);
        gopine_ble_ack_updates();
        test_rtc0.COUNTER+=31u*32768;
        assert(gopine_ble_host_work_pending() && gopine_ble_updates());
        gopine_ble_poll();
        assert(!pairing_code && !gopine_bond_exists());
        assert(gopine_ble_pairing_diagnostics()==BLE_HS_ETIMEOUT);
        assert(gopine_ble_pairing_progress()==0x200);
    }
    gopine_ble_stop();gopine_ble_poll();
    assert(!gopine_ble_busy() && pairing_code==0);
}
static void request_encryption(const uint8_t *expected_ltk) {
    struct ble_hci_ev_le_subev_lt_key_req request={
        .subev_code=BLE_HCI_LE_SUBEV_LT_KEY_REQ,.conn_handle=1};
    uint8_t packet[sizeof(request)+2]={BLE_HCI_EVCODE_LE_META,sizeof(request)};
    memcpy(packet+2,&request,sizeof(request));
    unsigned before=ltk_replies;send_event(packet,sizeof(packet));gopine_ble_poll();
    assert(ltk_replies==before+1 && !memcmp(controller_ltk,expected_ltk,16));
    uint8_t encrypted[]={BLE_HCI_EVCODE_ENCRYPT_CHG,4,0,1,0,1};
    reply_len=0;send_event(encrypted,sizeof(encrypted));gopine_ble_poll();
}
static void assert_authenticated(void) {
    struct ble_gap_conn_desc desc;
    assert(ble_gap_conn_find(1,&desc)==0);
    assert(desc.sec_state.encrypted && desc.sec_state.authenticated &&
           desc.sec_state.bonded && desc.sec_state.key_size==16);
    assert(phone_link_permission(1)==0 && pairing_code==0);
    uint8_t read[]={BLE_ATT_OP_READ_REQ,0,0};put_le16(read+1,battery_handle);
    att(read,sizeof(read),BLE_ATT_OP_READ_RSP);assert(reply[9]==80);
}
static void write_characteristic(const ble_uuid_t *service,const ble_uuid_t *characteristic,
                                 const uint8_t *data,unsigned size,uint8_t response) {
    uint16_t handle;assert(ble_gatts_find_chr(service,characteristic,NULL,&handle)==0);
    uint8_t packet[56]={BLE_ATT_OP_WRITE_REQ};assert(size<=sizeof(packet)-3);
    put_le16(packet+1,handle);memcpy(packet+3,data,size);
    att(packet,size+3,response);
}
static void phone_feature_traffic(void) {
    uint8_t out[147];uint32_t age;
    const uint8_t *times[]={companion_time_9,companion_time_10};
    const unsigned time_sizes[]={sizeof(companion_time_9),sizeof(companion_time_10)};
    for(unsigned i=0;i<2;i++) {
        write_characteristic(BLE_UUID16_DECLARE(0x1805),BLE_UUID16_DECLARE(0x2a2b),
                             times[i],time_sizes[i],BLE_ATT_OP_WRITE_RSP);
        assert(gopine_ble_take(out,&age)==(int)time_sizes[i] && !memcmp(times[i],out,time_sizes[i]));
    }
    const uint8_t *weather[]={companion_current_v0,companion_current_v1,companion_forecast};
    const unsigned weather_sizes[]={sizeof(companion_current_v0),sizeof(companion_current_v1),sizeof(companion_forecast)};
    for(unsigned i=0;i<3;i++) {
        write_characteristic(&weather_uuid.u,&weather_data_uuid.u,weather[i],weather_sizes[i],BLE_ATT_OP_WRITE_RSP);
        assert(gopine_ble_take_weather(out)==(int)weather_sizes[i] && !memcmp(weather[i],out,weather_sizes[i]));
    }
    write_characteristic(&music_uuids[0].u,&music_uuids[2].u,companion_status,sizeof(companion_status),BLE_ATT_OP_WRITE_RSP);
    write_characteristic(&music_uuids[0].u,&music_uuids[4].u,companion_track,sizeof(companion_track),BLE_ATT_OP_WRITE_RSP);
    write_characteristic(&music_uuids[0].u,&music_uuids[3].u,companion_artist,sizeof(companion_artist),BLE_ATT_OP_WRITE_RSP);
    assert(gopine_ble_take_music(out) && out[81]==PHONE_LINK_MUSIC_READY &&
           out[80]==1 && !memcmp(out,companion_track,sizeof(companion_track)) &&
           !memcmp(out+40,companion_artist,sizeof(companion_artist)));
    const uint8_t *numbers[]={companion_position,companion_duration};
    for(unsigned i=0;i<2;i++) {
        write_characteristic(&music_uuids[0].u,&music_uuids[6+i].u,numbers[i],4,BLE_ATT_OP_WRITE_RSP);
        uint16_t handle;assert(!ble_gatts_find_chr(&music_uuids[0].u,&music_uuids[6+i].u,NULL,&handle));
        uint8_t read[]={BLE_ATT_OP_READ_REQ,0,0};put_le16(read+1,handle);
        att(read,sizeof(read),BLE_ATT_OP_READ_RSP);assert(!memcmp(reply+9,numbers[i],4));
    }
    const uint8_t controls[]={0xe0,0,1,3,4,5,6};
    for(unsigned i=0;i<sizeof(controls);i++) {
        reply_len=0;
        assert(!gopine_ble_music_command(controls[i],peer_generation));
        assert(reply_len==12 && reply[8]==BLE_ATT_OP_NOTIFY_REQ &&
               get_le16(reply+9)==music_handle && reply[11]==controls[i]);
        gopine_ble_poll();
    }
    reply_len=0;assert(gopine_ble_music_command(0,peer_generation-1)==BLE_HS_ENOTCONN && !reply_len);
    write_characteristic(BLE_UUID16_DECLARE(0x1811),BLE_UUID16_DECLARE(0x2a46),
                         companion_alert,sizeof(companion_alert),BLE_ATT_OP_WRITE_RSP);
    assert(gopine_ble_take_notification(out)==sizeof(companion_alert) &&
           !memcmp(out,companion_alert,sizeof(companion_alert)));
    assert(phone_window && window && connection==1 && !expired());
}
#include "host_ancs.h"
// Follow the peer-initiated passkey sequence in InfiniTime's pinned NimBLE
// ble_sm_test_util_peer_sc_good_once_no_init, using a dynamically calculated
// central transcript. No SM internal state is injected or bypassed.
static void phone_complete_pairing(bool wrong_code, bool fail_identity) {
    uint8_t time[10]={0};assert(!gopine_bond_exists());
    // Pair over a temporary private address, then distribute a different
    // stable identity, as an iPhone does. Store subscriptions under identity.
    peer_address[5]=0x40;
    assert(gopine_ble_start_phone(time,80,0)==0);gopine_ble_poll();connect_peer();
    uint16_t cccd_handle;
    uint8_t mtu[]={BLE_ATT_OP_MTU_REQ,185,0};att(mtu,sizeof(mtu),BLE_ATT_OP_MTU_RSP);
    assert(ble_gatts_find_dsc(&music_uuids[0].u,&music_uuids[1].u,
                            BLE_UUID16_DECLARE(0x2902),&cccd_handle)==0);
    uint8_t subscribe[]={BLE_ATT_OP_WRITE_REQ,0,0,1,0};put_le16(subscribe+1,cccd_handle);
    att(subscribe,sizeof(subscribe),BLE_ATT_OP_WRITE_RSP);
    assert(music_subscribed && phone_link_snapshot(1,true)==PHONE_LINK_SECURING);
    uint8_t early_time[10]={0xea,0x07,10,9,16,7,8,5,0,1};
    write_characteristic(BLE_UUID16_DECLARE(0x1805),BLE_UUID16_DECLARE(0x2a2b),
                         early_time,sizeof(early_time),BLE_ATT_OP_ERROR_RSP);
    assert(reply[12]==BLE_ATT_ERR_INSUFFICIENT_AUTHEN && !phone_time_incoming.size);
    uint8_t pairing[]={BLE_SM_OP_PAIR_REQ,BLE_SM_IO_CAP_KEYBOARD_DISP,0,0x0d,16,3,3};
    receive_l2cap(BLE_L2CAP_CID_SM,pairing,sizeof(pairing));
    assert(reply[8]==BLE_SM_OP_PAIR_RSP);
    uint8_t response[7];memcpy(response,reply+8,sizeof(response));
    uint8_t private_be[32]={0},private_le[32]={3},public_be[64];private_be[31]=3;
    assert(uECC_compute_public_key(private_be,public_be,uECC_secp256r1())==1);
    uint8_t public[65]={BLE_SM_OP_PAIR_PUBLIC_KEY},watch_public[64];
    for(unsigned i=0;i<32;i++) {public[1+i]=public_be[31-i];public[33+i]=public_be[63-i];}
    receive_l2cap(BLE_L2CAP_CID_SM,public,sizeof(public));
    assert(reply[8]==BLE_SM_OP_PAIR_PUBLIC_KEY && pairing_code>0);
    assert(gopine_ble_pairing_progress()==0x200);
    memcpy(watch_public,reply+9,sizeof(watch_public));
    uint32_t passkey=pairing_code-1;
    uint8_t nonce[16],watch_nonce[16],watch_confirm[16],confirm[17],random[17];
    for(unsigned round=0;round<20;round++) {
        memset(nonce,round+1,sizeof(nonce));
        uint8_t bit=0x80|((passkey>>round)&1);
        confirm[0]=BLE_SM_OP_PAIR_CONFIRM;
        assert(ble_sm_alg_f4(public+1,watch_public,nonce,bit,confirm+1)==0);
        if(wrong_code && round==0)confirm[1]^=1;
        receive_l2cap(BLE_L2CAP_CID_SM,confirm,sizeof(confirm));
        assert(reply[8]==BLE_SM_OP_PAIR_CONFIRM);
        assert(gopine_ble_pairing_progress()==(0x280|round));
        memcpy(watch_confirm,reply+9,16);
        random[0]=BLE_SM_OP_PAIR_RANDOM;memcpy(random+1,nonce,16);
        receive_l2cap(BLE_L2CAP_CID_SM,random,sizeof(random));
        if(wrong_code) {
            assert(reply[8]==BLE_SM_OP_PAIR_FAIL && reply[9]==BLE_SM_ERR_CONFIRM_MISMATCH);
            assert(pairing_code==0 && !gopine_bond_exists());
            assert(gopine_ble_pairing_progress()==0x280);
            assert(gopine_ble_pairing_diagnostics()==BLE_HS_SM_US_ERR(BLE_SM_ERR_CONFIRM_MISMATCH));
            uint8_t disconnected[]={BLE_HCI_EVCODE_DISCONN_CMP,4,0,1,0,BLE_ERR_REM_USER_CONN_TERM};
            send_event(disconnected,sizeof(disconnected));gopine_ble_poll();
            assert(gopine_ble_pairing_progress()==0x280);
            assert(gopine_ble_pairing_diagnostics()==
                (BLE_HS_SM_US_ERR(BLE_SM_ERR_CONFIRM_MISMATCH)|
                 ((uint32_t)BLE_HS_HCI_ERR(BLE_ERR_REM_USER_CONN_TERM)<<16)));
            gopine_ble_stop();gopine_ble_poll();return;
        }
        assert(reply[8]==BLE_SM_OP_PAIR_RANDOM && pairing_code==passkey+1);
        assert(gopine_ble_pairing_progress()==(round==19?0x314:(0x200|round+1)));
        memcpy(watch_nonce,reply+9,16);
        uint8_t expected[16];
        assert(ble_sm_alg_f4(watch_public,public+1,watch_nonce,bit,expected)==0);
        assert(!memcmp(expected,watch_confirm,16));
    }
    struct ble_gap_conn_desc desc;assert(ble_gap_conn_find(1,&desc)==0);
    uint8_t dhkey[32],mackey[16],ltk[16],tk[16]={0},check[17]={BLE_SM_OP_PAIR_DHKEY_CHECK};
    put_le32(tk,passkey);
    assert(ble_sm_alg_gen_dhkey(watch_public,watch_public+32,private_le,dhkey)==0);
    assert(ble_sm_alg_f5(dhkey,nonce,watch_nonce,desc.peer_ota_addr.type,desc.peer_ota_addr.val,
                        desc.our_ota_addr.type,desc.our_ota_addr.val,mackey,ltk)==0);
    assert(ble_sm_alg_f6(mackey,nonce,watch_nonce,tk,pairing+1,
                        desc.peer_ota_addr.type,desc.peer_ota_addr.val,
                        desc.our_ota_addr.type,desc.our_ota_addr.val,check+1)==0);
    receive_l2cap(BLE_L2CAP_CID_SM,check,sizeof(check));
    assert(reply[8]==BLE_SM_OP_PAIR_DHKEY_CHECK);
    assert(gopine_ble_pairing_progress()==0x414);
    uint8_t expected[16];
    assert(ble_sm_alg_f6(mackey,watch_nonce,nonce,tk,response+1,
                        desc.our_ota_addr.type,desc.our_ota_addr.val,
                        desc.peer_ota_addr.type,desc.peer_ota_addr.val,expected)==0);
    assert(!memcmp(reply+9,expected,16));
    request_encryption(ltk);
    assert(gopine_ble_pairing_progress()==0x514);
    // iOS can retry its protected Battery read immediately after encryption,
    // while phase 3 still waits for identity distribution. Battery must not
    // reject an encrypted link just because no durable bond exists yet.
    assert(!gopine_bond_exists());
    assert(!ble_gap_conn_find(1,&desc) && desc.sec_state.encrypted &&
           desc.sec_state.authenticated && !desc.sec_state.bonded);
    uint8_t read[]={BLE_ATT_OP_READ_REQ,0,0};put_le16(read+1,battery_handle);
    att(read,sizeof(read),BLE_ATT_OP_READ_RSP);assert(reply[9]==80);
    // Secure Connections derives the LTK; phase 3 distributes identity keys.
    // ATT retries may precede those keys. Authentication has succeeded, so
    // don't ask iOS to authenticate again; retain data until bonding finishes.
    write_characteristic(BLE_UUID16_DECLARE(0x1805),BLE_UUID16_DECLARE(0x2a2b),
                         early_time,sizeof(early_time),BLE_ATT_OP_WRITE_RSP);
    uint8_t deferred[147];uint32_t deferred_age;
    assert(!gopine_ble_take(deferred,&deferred_age) && phone_time_incoming.size);
    write_characteristic(&weather_uuid.u,&weather_data_uuid.u,
                         companion_current_v1,sizeof(companion_current_v1),BLE_ATT_OP_WRITE_RSP);
    assert(!gopine_ble_take_weather(deferred) && weather_incoming.sizes[0]);
    write_characteristic(&music_uuids[0].u,&music_uuids[4].u,
                         companion_track,sizeof(companion_track),BLE_ATT_OP_WRITE_RSP);
    assert(gopine_ble_take_music(deferred) && deferred[81]==PHONE_LINK_SECURING && !deferred[0]);
    assert(!memcmp(music_incoming.track,companion_track,sizeof(companion_track)));
    assert(gopine_ble_music_command(0,peer_generation)==BLE_HS_ENOTCONN);
    write_characteristic(BLE_UUID16_DECLARE(0x1811),BLE_UUID16_DECLARE(0x2a46),
                         companion_alert,sizeof(companion_alert),BLE_ATT_OP_WRITE_RSP);
    assert(!gopine_ble_take_notification(deferred) && notification_incoming.count);
    gopine_ble_ack_updates();assert(!gopine_ble_updates()); // no pending-data spin
    assert(phone_link_snapshot(1,true)==PHONE_LINK_SECURING);
    assert(!client_request_count); // ANCS waits for completed bonding too.
    if(fail_identity) {
        uint8_t failed[]={BLE_SM_OP_PAIR_FAIL,BLE_SM_ERR_UNSPECIFIED};
        receive_l2cap(BLE_L2CAP_CID_SM,failed,sizeof(failed));
        assert(!pairing_code && !gopine_bond_exists() && !phone_link_ready(1));
        assert(!phone_time_incoming.size && !weather_incoming.sizes[0] &&
               !notification_incoming.count && !music_incoming.track[0]);
        assert(gopine_ble_pairing_progress()==0x514);
        gopine_ble_stop();test_rtc0.COUNTER+=32768;gopine_ble_poll();gopine_ble_poll();
        assert(!gopine_ble_busy());return;
    }
    uint8_t identity[17]={BLE_SM_OP_IDENTITY_INFO};memset(identity+1,0x55,16);
    receive_l2cap(BLE_L2CAP_CID_SM,identity,sizeof(identity));
    uint8_t address[]={BLE_SM_OP_IDENTITY_ADDR_INFO,BLE_ADDR_RANDOM,1,2,3,4,5,0xc0};
    receive_l2cap(BLE_L2CAP_CID_SM,address,sizeof(address));
    assert(gopine_ble_take(deferred,&deferred_age)==sizeof(early_time) &&
           !memcmp(deferred,early_time,sizeof(early_time)));
    assert(gopine_ble_take_weather(deferred)==sizeof(companion_current_v1) &&
           !memcmp(deferred,companion_current_v1,sizeof(companion_current_v1)));
    assert(gopine_ble_take_notification(deferred)==sizeof(companion_alert) &&
           !memcmp(deferred,companion_alert,sizeof(companion_alert)));
    assert(gopine_ble_take_music(deferred) && deferred[81]==PHONE_LINK_MUSIC_READY &&
           !memcmp(deferred,companion_track,sizeof(companion_track)));
    assert_authenticated();
    assert(gopine_ble_pairing_progress()==0x600);
    assert(gopine_ble_pairing_diagnostics()==0);
    struct ble_store_key_cccd cccd_key={.peer_addr={BLE_ADDR_RANDOM,{1,2,3,4,5,0xc0}},
                                      .chr_val_handle=music_handle};
    struct ble_store_value_cccd cccd;
    assert(ble_store_read_cccd(&cccd_key,&cccd)==0 && cccd.flags==1);
    cccd_key.peer_addr.val[5]=0x40;
    assert(ble_store_read_cccd(&cccd_key,&cccd)==BLE_HS_ENOENT);
    phone_feature_traffic();
    phone_ancs_traffic(true,true);
    gopine_ble_stop();
    // Feature writes retain a 200 ms final-ACK grace before radio teardown.
    test_rtc0.COUNTER+=32768;gopine_ble_poll();gopine_ble_poll();
    assert(!gopine_ble_busy() && gopine_bond_status()==2);
    // Reload the actual flash journal, then restore encryption with the saved
    // key. A reconnect must not ask for another code or merely read old RAM.
    gopine_bond_init();assert(gopine_bond_status()==2);
    unsigned saved_writes=flash_writes;
    // Fail a controller restore, then recover without losing the saved phone.
    reject_peer_irk=true;
    assert(gopine_ble_start_phone(time,80,0)==0);gopine_ble_poll();
    if(synced || ble_gap_adv_active() || gopine_ble_error()!=BLE_HS_HCI_ERR(BLE_ERR_MEM_CAPACITY))
        fprintf(stderr,"restore failure: synced %d advertising %d status %d expected %d\n",synced,
                ble_gap_adv_active(),gopine_ble_error(),BLE_HS_HCI_ERR(BLE_ERR_MEM_CAPACITY));
    assert(!synced && !ble_gap_adv_active() && gopine_ble_error()==BLE_HS_HCI_ERR(BLE_ERR_MEM_CAPACITY));
    assert((uint16_t)gopine_ble_pairing_diagnostics()==BLE_HS_HCI_ERR(BLE_ERR_MEM_CAPACITY));
    gopine_ble_stop();gopine_ble_poll();gopine_ble_poll();
    assert(!gopine_ble_busy() && gopine_bond_status()==2 && flash_writes==saved_writes);
    reject_peer_irk=false;
    for(unsigned rotation=0;rotation<8;rotation++) {
        // Generate a new RPA from the distributed IRK (0x55). It must resolve
        // using the controller key installed after each actual HCI Reset.
        struct ble_encryption_block rpa={0};memset(rpa.key,0x55,16);
        peer_address[3]=0x31+rotation;peer_address[4]=0x72;peer_address[5]=0x4a;
        for(unsigned k=0;k<3;k++)rpa.plain_text[15-k]=peer_address[3+k];
        assert(!ble_hw_encrypt_block(&rpa));
        for(unsigned k=0;k<3;k++)peer_address[k]=rpa.cipher_text[15-k];
        assert(gopine_ble_start_phone(time,80,0)==0);gopine_ble_poll();connect_peer();
        assert(ble_gap_conn_find(1,&desc)==0);
        assert(desc.peer_id_addr.type==BLE_ADDR_RANDOM && desc.peer_id_addr.val[5]==0xc0);
        assert(!memcmp(desc.peer_ota_addr.val,peer_address,6));
        att(mtu,sizeof(mtu),BLE_ATT_OP_MTU_RSP);
        request_encryption(ltk);assert_authenticated();
        assert(music_subscribed && phone_link_snapshot(1,true)==PHONE_LINK_MUSIC_READY);
        phone_feature_traffic();
        phone_ancs_traffic(rotation==0,false);
        gopine_ble_stop();test_rtc0.COUNTER+=32768;gopine_ble_poll();gopine_ble_poll();
        assert(!gopine_ble_busy() && gopine_bond_status()==2 && flash_writes==saved_writes);
    }
    assert(gopine_ble_forget_phone()==0);
    assert(gopine_ble_start_phone(time,80,0)==0);gopine_ble_poll();
    for(unsigned i=0;i<resolving_count;i++)assert(resolving_entries[i].peer_id_addr[5]!=0xc0);
    gopine_ble_stop();gopine_ble_poll();
}
int main(void) {
    memset(flash,0xff,sizeof(flash));
    ble_npl_eventq_init(&crypto_controller);
    ble_npl_event_init(&crypto_event,crypto_controller_callback,NULL);
    ble_npl_event_init(&crypto_host_event,crypto_host_callback,NULL);
    uint16_t original_battery=0,original_music=0;
    int original_attrs=0,original_services=0,original_configs=0;
    for(unsigned i=0;i<100;i++) {
        assert(gopine_ble_start_update(80)==0);gopine_ble_poll();
        assert(synced && ble_gap_adv_active() && update_window);
        if(i==0) {
            original_battery=battery_handle;original_music=music_handle;
            original_attrs=ble_hs_max_attrs;original_services=ble_hs_max_services;
            original_configs=ble_hs_max_client_configs;
        }
        assert(battery_handle==original_battery && music_handle==original_music);
        assert(ble_hs_max_attrs==original_attrs && ble_hs_max_services==original_services &&
               ble_hs_max_client_configs==original_configs);
        connect_peer();assert(!phone_window && !security_pending);
        discover();
        gopine_ble_stop();gopine_ble_poll();
        assert(!gopine_ble_busy() && connection==BLE_HS_CONN_HANDLE_NONE);
        if(i%10==0)phone_pairing_code(i==90);
    }
    phone_complete_pairing(true,false);
    phone_complete_pairing(false,true);
    phone_complete_pairing(false,false);
    assert(crypto_yields>1000 && crypto_controller_runs==crypto_yields);
    assert(controller_resets>=120);
    puts("100 update cycles, full SC bonding, 8 private-address reconnects, controller-restore recovery and real GATT ANCS lifecycle passed");
}
