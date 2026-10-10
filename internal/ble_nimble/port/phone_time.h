#ifndef GOPINE_PHONE_TIME_H
#define GOPINE_PHONE_TIME_H

#include <string.h>
#include "phone_security.h"

// This mailbox is only for automatic clock updates. The one-shot time screen
// has its own mailbox and continues to require confirmation on the watch.
#define PHONE_TIME_MAX_AGE_MS 60000u
struct phone_time_mailbox {
    uint8_t value[10], size;
    uint16_t connection;
    uint32_t generation, received_at;
};

static inline void phone_time_clear(struct phone_time_mailbox *m) {
    memset(m, 0, sizeof(*m));
}

static inline int phone_time_receive(struct phone_time_mailbox *m,
                                     uint16_t conn, uint32_t generation,
                                     const uint8_t *value, unsigned size,
                                     uint32_t now) {
    int rc = phone_link_permission(conn);
    if (rc) return rc;
    if (size < 9 || size > sizeof(m->value))
        return BLE_ATT_ERR_INVALID_ATTR_VALUE_LEN;
    // Never retain pre-encryption data and later promote it to trusted data:
    // subsequent authentication does not authenticate an earlier ATT payload.
    memcpy(m->value, value, size);
    m->size = size;
    m->connection = conn;
    m->generation = generation;
    m->received_at = now;
    return 0;
}

static inline int phone_time_take(struct phone_time_mailbox *m,
                                  uint16_t conn, uint32_t generation,
                                  uint32_t now, uint8_t *value, uint32_t *age) {
    if (!m->size) return 0;
    uint32_t elapsed = now - m->received_at;
    if (conn != m->connection || generation != m->generation ||
        elapsed > PHONE_TIME_MAX_AGE_MS || phone_link_permission(conn)) {
        phone_time_clear(m);
        return 0;
    }
    if (!phone_link_ready(conn)) return 0;
    int size = m->size;
    memcpy(value, m->value, size);
    *age = elapsed;
    phone_time_clear(m);
    return size;
}

#endif
