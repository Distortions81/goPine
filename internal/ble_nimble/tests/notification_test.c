#include "../port/notification_mailbox.h"
#include <assert.h>

int main(void) {
    struct notification_mailbox m={0};
    uint8_t packet[104]={255,1,0,'A'}, out[103]={0};
    for(unsigned n=0;n<4;n++) assert(!notification_store(&m,packet,n));
    assert(!notification_store(&m,packet,104));
    assert(notification_store(&m,packet,4));
    packet[3]='B'; assert(notification_store(&m,packet,103));
    packet[3]='C'; assert(notification_store(&m,packet,5));
    assert(notification_take(&m,out)==103 && out[3]=='B');
    assert(notification_take(&m,out)==5 && out[3]=='C');
    assert(notification_take(&m,out)==0);
    for(unsigned i=0;i<2;i++) for(unsigned j=0;j<103;j++) assert(m.records[i][j]==0);
    for(unsigned i=0;i<10000;i++) {
        packet[3]=(uint8_t)i;
        assert(notification_store(&m,packet,103));
        assert(notification_take(&m,out)==103 && out[3]==(uint8_t)i);
    }
    return 0;
}
