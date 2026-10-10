#include "../port/update_mailbox.h"
#include <assert.h>
int main(void) {
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
