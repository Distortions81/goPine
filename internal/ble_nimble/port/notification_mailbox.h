#ifndef GOPINE_NOTIFICATION_MAILBOX_H
#define GOPINE_NOTIFICATION_MAILBOX_H
#include <stdint.h>
#include <string.h>

#define NOTIFICATION_PACKET_SIZE 103
// Cooperative host only. A burst retains the two newest packets; each Go
// update drains both. No interrupt runs Go or retains an mbuf pointer.
struct notification_mailbox {
    uint8_t records[2][NOTIFICATION_PACKET_SIZE], sizes[2], head, count;
};
static inline int notification_store(struct notification_mailbox *m, const uint8_t *data, unsigned n) {
    if(n<4 || n>NOTIFICATION_PACKET_SIZE) return 0;
    if(m->count==2) { m->head=(m->head+1)%2; m->count--; }
    unsigned at=(m->head+m->count)%2;
    memcpy(m->records[at],data,n); m->sizes[at]=n; m->count++;
    return 1;
}
static inline int notification_take(struct notification_mailbox *m, uint8_t *out) {
    if(!m->count) return 0;
    unsigned n=m->sizes[m->head];
    memcpy(out,m->records[m->head],n);
    memset(m->records[m->head],0,NOTIFICATION_PACKET_SIZE);
    m->sizes[m->head]=0; m->head=(m->head+1)%2; m->count--;
    return n;
}
#endif
