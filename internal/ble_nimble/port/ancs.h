#ifndef GOPINE_ANCS_H
#define GOPINE_ANCS_H
#include "host/ble_hs.h"
#include "ancs_mailbox.h"

void gopine_ancs_reset(void);
void gopine_ancs_poll(uint16_t conn, bool trusted);
void gopine_ancs_notify(uint16_t conn, uint16_t attr, struct os_mbuf *om);
int gopine_ancs_take(uint8_t *out, bool trusted);
bool gopine_ancs_updates(void);
uint32_t gopine_ancs_delay(void);
#endif
