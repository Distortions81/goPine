// One reserved internal page, append-only committed snapshots. The old snapshot
// remains valid across interrupted writes. Reclamation occurs only with BLE off.
#ifndef GOPINE_BOND_JOURNAL_H
#define GOPINE_BOND_JOURNAL_H
#include <stdint.h>
#include <stdbool.h>
#include <string.h>
#define BOND_RECORD_SIZE 512
#define BOND_PAGE_SIZE 4096
#define BOND_PAYLOAD_SIZE (BOND_RECORD_SIZE-12)
#define BOND_MAGIC UINT32_C(0x31444e42)
struct bond_record { uint8_t payload[BOND_PAYLOAD_SIZE]; uint32_t size, crc, commit; };
uint32_t bond_flash_word(unsigned offset);
bool bond_flash_write(unsigned offset, uint32_t word);
bool bond_flash_erase(void);
static uint32_t bond_crc(const void *data, unsigned size) {
    const uint8_t *p=data; uint32_t crc=~0u;
    for(unsigned i=0;i<size;i++) {
        crc^=p[i];
        for(unsigned b=0;b<8;b++) crc=(crc>>1)^((0u-(crc&1))&0xedb88320);
    }
    return ~crc;
}
static void bond_read_record(unsigned offset, struct bond_record *r) {
    for(unsigned i=0;i<sizeof(*r);i+=4) {
        uint32_t v=bond_flash_word(offset+i); memcpy((uint8_t*)r+i,&v,4);
    }
}
static bool bond_record_valid(const struct bond_record *r, unsigned size) {
    return r->commit==BOND_MAGIC && r->size==size && r->crc==bond_crc(r->payload,size);
}
static bool bond_journal_load(void *data, unsigned size, unsigned *next) {
    struct bond_record r; bool found=false; *next=0;
    for(unsigned at=0;at<BOND_PAGE_SIZE;at+=sizeof(r)) {
        bond_read_record(at,&r);
        bool blank=true;
        for(unsigned i=0;i<sizeof(r);i++) if(((uint8_t*)&r)[i]!=0xff) {blank=false;break;}
        if(!blank) *next=at+sizeof(r); // Never overwrite a torn record.
        if(bond_record_valid(&r,size)) {memcpy(data,r.payload,size);found=true;}
    }
    return found;
}
static bool bond_journal_save(const void *data, unsigned size, unsigned *next) {
    if(size>BOND_PAYLOAD_SIZE || *next>BOND_PAGE_SIZE-BOND_RECORD_SIZE) return false;
    struct bond_record r; memset(&r,0xff,sizeof(r)); memcpy(r.payload,data,size);
    r.size=size; r.crc=bond_crc(data,size); r.commit=BOND_MAGIC;
    unsigned at=*next; *next+=sizeof(r);
    for(unsigned i=0;i<sizeof(r);i+=4) {
        uint32_t v;memcpy(&v,(uint8_t*)&r+i,4);
        if(!bond_flash_write(at+i,v))return false;
    }
    bond_read_record(at,&r);
    return bond_record_valid(&r,size) && !memcmp(r.payload,data,size);
}
#endif
