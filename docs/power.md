# Screen-off power audit

“Off” here means display sleep, not nRF System OFF: the RTC, alarms, countdowns,
and side-button recovery continue working. These changes have host-test and
firmware-build coverage; battery life and sleep current still need measurement
on a watch.

## Touch to wake

Settings has a **Touch to wake: Off/On** toggle. It defaults to Off, including
when loading existing settings. Back remains available through the top-left
arrow or a right swipe. Touch works normally while awake. With the setting On,
the waking contact is consumed instead of activating a control.

With the setting Off, screen sleep resets the CST816S and writes `0x03` to
register `0xA5`, following [InfiniTime's touch driver](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/drivers/Cst816s.cpp).
The touch interrupt is removed, its GPIOTE channel is disabled, and queued
contact state is discarded. TinyGo 0.42.0's `SetInterrupt(0, nil)` only masks
the interrupt, so channel configuration and latched events are explicitly
cleared too. Wake resets and configures touch again. Sleep writes have bounded
retries; failure leaves reset asserted and interrupts off until wake retries
initialization. A touch fault does not prevent button or timer wake.

The saved flag occupies previously reserved byte 144 in the existing
CRC-protected journal record. Old records contain zero there, so upgrades
preserve alarms/timers and select Off. Older firmware rejects records with
Touch to wake enabled and may restore an earlier snapshot, or defaults if no
compatible snapshot remains. Select Off and allow the save to finish before
downgrading to a firmware version without this setting if current state matters.

## Other activity

| Component | Audited behavior / change |
| --- | --- |
| LCD/backlight | Already enters LCD sleep and switches all three backlight channels off. SPI0 is now also disabled after the sleep command and re-enabled before wake. |
| Touch I2C | TinyGo leaves TWI1 enabled after transfers. Now enabled only for a transaction and disabled afterward, including errors. No touch polling or reads while asleep with touch wake Off. |
| Battery | Sampling changes from every second to every 30 seconds during sleep; button/touch/alert wake refreshes immediately. SAADC is disabled between completed conversions. TinyGo notes that merely enabling SAADC consumes negligible current; fewer conversions are the main intended reduction here. |
| Application loop | Sleep skips rendering and stopwatch/countdown animation. Timer and alarm deadlines still take precedence. Pending settings retain their two-second save deadline independently of battery sampling. |
| Side button/watchdog | Still polls every 20 ms, briefly energizing the button circuit and feeding the inherited watchdog only when released. This remains a source of CPU wakeups. Longer polling risks missing short presses; interrupt-driven sensing needs board/current validation before replacing it. |
| Bluetooth | Starts only for explicit time sync. Sleep cancels the session; existing teardown disables radio/RNG and releases HFXO. The cooperative service path continues to be called by the input loop, allowing asynchronous disconnect to finish. RTC0 bookkeeping remains after first sync. Verify current returns to baseline after repeated sync windows. |
| External flash/UART/regulator | Already sends external flash deep-power-down at boot, disables UART0, and enables DC/DC. |
| Vibration | Already off outside bounded alert pulses; sleep dismissal and error exits turn it off. |
| Motion/heart-rate sensors | No goPine sampling or driver initialization was found. Sensor state inherited across firmware resets and residual board current have not been measured. |

## Validation

Host tests cover default/toggle/repaint, both persisted choices, old-record
compatibility, malformed flag rejection, touch sleep command/retries, SDL
button and touch wake, consumed wake taps, sleep battery cadence, timely saves,
and existing sleeping alarm/countdown/snooze behavior. These do not simulate
the physical buses, GPIO interrupt hardware, or current consumption.

On a watch, compare sleep current with touch wake Off and On; test repeated
button wake, the first touch after wake, brief presses and held-button recovery,
alarm/countdown wake and dismissal, reboot persistence, and sleep after time
sync. Measure with the debugger disconnected. Confirm charging indications
refresh on wake and touch reinitializes reliably after many sleep cycles before
claiming a battery-life improvement.
