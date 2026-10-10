#include "../port/weather_mailbox.h"
#include <assert.h>

int main(void) {
    struct weather_mailbox box = {0};
    uint8_t current[54] = {0, 1}, forecast[36] = {1, 0}, out[53];
    forecast[10] = 5;
    for (unsigned n = 0; n < 53; ++n) assert(!weather_store(&box, current, n));
    assert(!weather_store(&box, current, 54));
    assert(weather_store(&box, current, 53));
    current[10] = 42;
    assert(weather_store(&box, current, 53)); // Latest current replaces old.
    assert(weather_store(&box, forecast, 36));
    forecast[10] = 6;
    assert(!weather_store(&box, forecast, 36));
    assert(weather_take(&box, out) == 53 && out[10] == 42);
    assert(weather_take(&box, out) == 36 && out[10] == 5);
    assert(weather_take(&box, out) == 0);
    forecast[10] = 0;
    assert(weather_store(&box, forecast, 11));
    assert(weather_take(&box, out) == 11);
    return 0;
}
