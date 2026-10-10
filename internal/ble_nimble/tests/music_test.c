#include <assert.h>
#include "../port/music_mailbox.h"
int main(void) {
    struct music_mailbox m={0}, before;
    uint8_t text[100]; memset(text,'x',sizeof(text));
    assert(music_store(&m,4,text,100));
    assert(m.track_len==40 && m.track[36]=='x' && !memcmp(m.track+37,"...",3));
    assert(music_store(&m,4,(uint8_t *)"Hi",2));
    assert(m.track_len==2 && m.track[2]==0);
    assert(music_store(&m,3,text,0) && m.artist_len==0);
    for(unsigned id=2;id<=12;id++) {
        if(id>=3 && id<=5) continue;
        unsigned size=(id>=6 && id<=10)?4:1;
        uint8_t data[4]={0,0,1,2};
        before=m;
        assert(!music_store(&m,id,data,size-1));
        assert(!memcmp(&m,&before,sizeof(m)));
        assert(music_store(&m,id,data,size));
        unsigned n; assert(!memcmp(music_field(&m,id,&n),data,size) && n==size);
    }
    uint8_t invalid=2; before=m;
    assert(!music_store(&m,2,&invalid,1));
    assert(!memcmp(&m,&before,sizeof(m)));
    assert(!music_store(&m,13,text,1));
    music_clear(&m); assert(m.dirty && !m.status_known && !m.track_len && !m.artist_len);
    m.dirty=false; assert(music_store(&m,6,text,4) && !m.dirty);
    assert(music_store(&m,2,(uint8_t[]){1},1) && m.status_known && m.status && m.dirty);
    m.dirty=false;
    assert(music_store(&m,2,(uint8_t[]){1},1) && !m.dirty);
    assert(music_store(&m,4,text,100) && m.dirty);
    m.dirty=false;
    assert(music_store(&m,4,text,100) && !m.dirty);
    assert(music_command_valid(0) && music_command_valid(6) && music_command_valid(0xe0));
    assert(!music_command_valid(2) && !music_command_valid(7));
}
