//go:build pinetime && bletime && !provision

package blenimble

/*
#include <stdint.h>
int gopine_ble_start(const uint8_t *value, uint8_t battery, uint32_t window_ms);
void gopine_ble_stop(void);
void gopine_ble_poll(void);
int gopine_ble_take(uint8_t *value, uint32_t *age);
int gopine_ble_error(void);
*/
import "C"

func Start(value [10]byte, battery uint8, windowMS uint32) int {
	return int(C.gopine_ble_start((*C.uint8_t)(&value[0]), C.uint8_t(battery), C.uint32_t(windowMS)))
}
func Stop()      { C.gopine_ble_stop() }
func Service()   { C.gopine_ble_poll() }
func Error() int { return int(C.gopine_ble_error()) }
func Take() (value [10]byte, size int, age uint32) {
	var elapsed C.uint32_t
	size = int(C.gopine_ble_take((*C.uint8_t)(&value[0]), &elapsed))
	return value, size, uint32(elapsed)
}
