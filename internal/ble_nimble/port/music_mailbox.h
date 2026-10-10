#ifndef GOPINE_MUSIC_MAILBOX_H
#define GOPINE_MUSIC_MAILBOX_H
#include <stdbool.h>
#include <stdint.h>
#include <string.h>

// InfiniTime characteristic suffixes 2..12. Strings are bounded to 40 bytes;
// numeric fields retain their wire bytes (four-byte integers are big endian).
struct music_mailbox {
    uint8_t artist[40], track[40], album[40], numbers[5][4];
    uint8_t artist_len, track_len, album_len, status, repeat, shuffle;
    bool status_known, dirty;
};
static inline uint8_t *music_field(struct music_mailbox *m, unsigned id, unsigned *len) {
    *len=1;
    switch(id) {
    case 2: return &m->status;
    case 3: *len=m->artist_len; return m->artist;
    case 4: *len=m->track_len; return m->track;
    case 5: *len=m->album_len; return m->album;
    case 11: return &m->repeat;
    case 12: return &m->shuffle;
    default: if(id>=6 && id<=10) { *len=4; return m->numbers[id-6]; }
    }
    *len=0; return NULL;
}
static inline bool music_store(struct music_mailbox *m, unsigned id, const uint8_t *data, unsigned n) {
    unsigned len;
    uint8_t *field=music_field(m,id,&len);
    if(!field) return false;
    bool changed=false;
    if(id>=3 && id<=5) {
        unsigned used=n>40?40:n;
        uint8_t text[40]={0};
        memcpy(text,data,used);
        if(n>40) memcpy(text+37,"...",3);
        changed=memcmp(field,text,40)!=0;
        memset(field,0,40);
        memcpy(field,text,40);
        if(id==3) m->artist_len=used;
        if(id==4) m->track_len=used;
        if(id==5) m->album_len=used;
    } else {
        if(n!=len || ((id==2 || id==12) && data[0]>1)) return false;
        changed=memcmp(field,data,n)!=0 || (id==2 && !m->status_known);
        memcpy(field,data,n);
        if(id==2) m->status_known=true;
    }
    if(id<=4 && changed) m->dirty=true; // Repeated metadata/position must not repaint.
    return true;
}
static inline void music_clear(struct music_mailbox *m) {
    memset(m,0,sizeof(*m)); m->dirty=true;
}
static inline bool music_command_valid(uint8_t command) {
    return command==0 || command==1 || (command>=3 && command<=6) || command==0xe0;
}
#endif
