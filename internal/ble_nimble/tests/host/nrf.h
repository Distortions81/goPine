#include "../nrf.h"
typedef struct { uint32_t DEVICEADDR[2]; } test_ficr;
extern test_ficr test_ficr0;
#define NRF_FICR (&test_ficr0)
