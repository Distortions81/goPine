#ifndef GOPINE_UPDATE_MAILBOX_H
#define GOPINE_UPDATE_MAILBOX_H
#include <stdint.h>
#include <string.h>
struct update_mailbox { uint8_t data[200], kind; uint16_t size; };
static inline int update_store(struct update_mailbox *m,uint8_t kind,const uint8_t *data,unsigned n) {
    if(m->kind) return 0;
    if((kind==1 && !((n==9 && data[0]==1) || (n==5 && (data[0]==2 || data[0]==3)))) ||
       (kind==2 && (n<9 || n>200)) || (kind!=1 && kind!=2)) return -1;
    memcpy(m->data,data,n);m->size=n;m->kind=kind;return 1;
}
static inline int update_take(struct update_mailbox *m,uint8_t *data,unsigned *n) {
    int kind=m->kind;if(!kind){*n=0;return 0;}
    *n=m->size;memcpy(data,m->data,*n);memset(m,0,sizeof(*m));return kind;
}
#endif
