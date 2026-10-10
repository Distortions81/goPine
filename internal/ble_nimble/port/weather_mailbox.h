#ifndef GOPINE_WEATHER_MAILBOX_H
#define GOPINE_WEATHER_MAILBOX_H
#include <stdint.h>
#include <string.h>

// One latest record per wire message type. Only the cooperative host accesses
// this mailbox; no interrupt callback runs Go or retains an mbuf pointer.
struct weather_mailbox { uint8_t records[2][53], sizes[2]; };

static inline int weather_store(struct weather_mailbox *box, const uint8_t *data, unsigned n) {
    if (n < 2 || n > 53) return 0;
    if (data[0] == 0) {
        if (!((data[1] == 0 && n == 49) || (data[1] == 1 && n == 53))) return 0;
    } else if (data[0] == 1) {
        if (data[1] != 0 || n < 11 || data[10] > 5 ||
            (n != 11u + 5u * data[10] && n != 36)) return 0;
    } else return 0;
    memcpy(box->records[data[0]], data, n);
    box->sizes[data[0]] = n;
    return 1;
}

static inline int weather_take(struct weather_mailbox *box, uint8_t *out) {
    for (unsigned i = 0; i < 2; ++i) {
        unsigned n = box->sizes[i];
        if (!n) continue;
        memcpy(out, box->records[i], n);
        box->sizes[i] = 0;
        return n;
    }
    return 0;
}
#endif
