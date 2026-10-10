#include <assert.h>
#include <string.h>
#include "../port/bond_journal.h"
static uint32_t flash[BOND_PAGE_SIZE/4];
static int writes_left=-1;
uint32_t bond_flash_word(unsigned off) {assert(off<sizeof(flash) && off%4==0);return flash[off/4];}
bool bond_flash_write(unsigned off,uint32_t v) {
    assert(off<sizeof(flash) && off%4==0);
    if(writes_left==0)return false;
    if(writes_left>0)writes_left--;
    assert((flash[off/4]&v)==v);flash[off/4]=v;return true;
}
bool bond_flash_erase(void) {memset(flash,0xff,sizeof(flash));return true;}
int main(void) {
    uint8_t old[240], next[240], loaded[240];
    memset(old,0x42,sizeof(old));memset(next,0x73,sizeof(next));
    unsigned offset;
    for(int cut=0;cut<BOND_RECORD_SIZE/4;cut++) {
        bond_flash_erase();writes_left=-1;offset=0;
        assert(bond_journal_save(old,sizeof(old),&offset));
        writes_left=cut;assert(!bond_journal_save(next,sizeof(next),&offset));
        memset(loaded,0,sizeof(loaded));
        assert(bond_journal_load(loaded,sizeof(loaded),&offset));
        assert(!memcmp(loaded,old,sizeof(old)));
        writes_left=-1;
        assert(bond_journal_save(next,sizeof(next),&offset));
        assert(bond_journal_load(loaded,sizeof(loaded),&offset));
        assert(!memcmp(loaded,next,sizeof(next)));
    }
    bond_flash_erase();offset=0;writes_left=-1;
    for(int i=0;i<8;i++) {next[0]=i;assert(bond_journal_save(next,sizeof(next),&offset));}
    assert(!bond_journal_save(old,sizeof(old),&offset));
    assert(bond_journal_load(loaded,sizeof(loaded),&offset) && loaded[0]==7);
    flash[7*BOND_RECORD_SIZE/4]^=1; // Bad CRC falls back to last complete snapshot.
    assert(bond_journal_load(loaded,sizeof(loaded),&offset) && loaded[0]==6);
    assert(!bond_journal_load(loaded,sizeof(loaded)-1,&offset)); // Schema mismatch.
    bond_flash_erase();assert(!bond_journal_load(loaded,sizeof(loaded),&offset) && offset==0);
}
