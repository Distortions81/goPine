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
| Battery | Samples every second awake, with no periodic samples while asleep; button/touch/alert wake refreshes immediately. Pending settings writes take a fresh sample only at the write boundary. SAADC is disabled between completed conversions. |
| Application loop | Sleep skips rendering, stopwatch/countdown animation, minute redraw deadlines, update-page expiry redraws, and closed time-sync-page polls. Timer and alarm deadlines still take precedence. Pending settings retain their two-second save deadline independently of battery sampling. |
| Side button/watchdog | While asleep and BLE is fully stopped, use low-power GPIO SENSE/PORT and interruptible WFE instead of 20 ms polling. RTC2 wakes for the earliest application deadline or watchdog service. Wake at most one second apart with a running watchdog, shortened to one quarter of its configured timeout. A held button still inhibits watchdog feeding and uses 20 ms release polling. Awake input polling is unchanged. |
| Bluetooth | Starts only for explicit time sync. Sleep cancels the session; existing teardown disables radio/RNG and releases HFXO. Continue pumping through asynchronous disconnect, then skip the pump once the host is stopped. RTC0 remains allocated after first sync; no changes to its ownership. Verify current returns to baseline after repeated sync windows. |
| External flash/UART/regulator | Already sends external flash deep-power-down at boot, disables UART0, and enables DC/DC. |
| Vibration | Already off outside bounded alert pulses; sleep dismissal and error exits turn it off. |
| Motion/heart-rate sensors | At boot, disable HRS3300 LED drive and clear its enable bit. Recognize BMA421/425 IDs (0x11/0x13), soft reset to discard inherited features/step counting, disable acceleration, enable advanced power save, and disable FIFO self-wakeup. No recurring sensor reads. Transfers are bounded by the TinyGo TWI timeout; attempt both sensors independently and allow the watch to boot if one is absent/unresponsive. |

During idle sleep the application has no periodic refresh deadline. Necessary
alarm/countdown/snooze and pending-save deadlines still apply. Hardware waits
service the inherited watchdog and RTC range limits internally. A low-power
settings-write rejection defers the save until another event instead of creating
an ADC retry timer. Unused heart-rate/motion sensors receive shutdown commands
once at boot only; they are never enabled for measurement or polled.

## Button sleep implementation and tradeoff

TinyGo 0.42's nRF `time.Sleep` keeps waiting for its RTC1 deadline after a GPIO
interrupt. Merely lengthening that sleep could miss short presses. The new
path uses RTC2 for a one-shot deadline and ARM WFE with SEVONPEND. It temporarily
masks the NVIC GPIOTE handler (TinyGo's handler does not service PORT), leaving
the peripheral's PORT interrupt enabled to generate a pending-interrupt event.
Button PORT events latch even if the user releases before the CPU resumes.
Existing touch IN events remain latched for the normal handler. Cleanup removes
button SENSE before clearing PORT, then restores SCR/NVIC state. RTC0/NimBLE and
RTC1/runtime are not repurposed. This assumes the current single Go UI task;
additional background Go tasks would require scheduler integration.

GPIO sense requires BUTTON_OUT to remain high while waiting. PINE64 reports
about **34 µA** for that circuit, versus briefly pulsing it for every poll.
The implementation trades that fixed draw for fewer CPU wakeups: typically
one watchdog wake per second instead of 50 input wakes per second, and up to
four minutes between wakes when no watchdog is running. This is **not yet a
measured net current reduction**. Compare against 0.3.3 on the watch before
claiming a battery-life improvement. Touch-wake On and BLE shutdown can cause
additional work.

Sensor register sequences follow the pinned InfiniTime
[HRS3300 driver](https://github.com/InfiniTimeOrg/InfiniTime/blob/6c119eb52206b580b556b41633dddc1e1b66a8da/src/drivers/Hrs3300.cpp)
and [BMA driver/API](https://github.com/InfiniTimeOrg/InfiniTime/tree/6c119eb52206b580b556b41633dddc1e1b66a8da/src/drivers/Bma421_C).
Wake design references: [PineTime button circuit](https://pine64.org/documentation/PineTime/Hardware/#button),
[Nordic GPIOTE PORT](https://docs.nordicsemi.com/r/bundle/ps_nrf52832/page/gpiote.html),
and [ARM event wake without an ISR](https://developer.arm.com/community/arm-community-blogs/b/architectures-and-processors-blog/posts/beginner-guide-on-interrupt-latency-and-interrupt-latency-of-the-arm-cortex-m-processors).

## Battery voltage curve

Use the six-point fit from Finlay Davidson's measurements on **three PineTimes**,
adopted by InfiniTime in
[commit 8b0d8889](https://github.com/InfiniTimeOrg/InfiniTime/commit/8b0d888952bb3cfbf587ab20d5096f2e578a6107).
The [methodology and CSV datasets](https://gist.github.com/FintasticMan/72d3aa1fc5c7cb9739f45b81197836c7)
include both intensive and light-load runs. This is more appropriate to the
watch's listed 170–180 mAh, 3.8 V LiPo than a generic lithium curve.

| Battery voltage | Estimated remaining |
| --- | --- |
| 3.500 V | 0% |
| 3.616 V | 3% |
| 3.723 V | 22% |
| 3.776 V | 48% |
| 3.979 V | 79% |
| 4.180 V | 100% |

Interpolate linearly between points, clamping outside the range. These are
empirical estimation points, not charging limits or a cutoff controller. The
study reconstructed voltages from InfiniTime's former linear percentage reports
and fitted normalized remaining runtime under constant loads; it did not measure
charge with a coulomb counter. It is not calibrated to this individual watch,
cell age, temperature, or goPine's load. The percentage may change after this
update without any change in actual energy. Keep the existing voltage conversion
and smoothing; judge power savings using controlled runtime/current comparisons.

An [older 2019 PineTime dataset](https://forum.pine64.org/showthread.php?tid=8147)
was also reviewed. It used a lit screen, maximum backlight and an attached
debugger, with a substantially lower measured full voltage. It is not used for
the new estimator.

## Validation

Host tests cover default/toggle/repaint, both persisted choices, old-record
compatibility, malformed flag rejection, touch sleep command/retries, SDL
button and touch wake, consumed wake taps, zero periodic battery samples through 12 hours of sleep,
timely power-checked saves without low-power retry loops, sensor failures/unknown IDs, watchdog service intervals, and
existing sleeping alarm/countdown/snooze behavior. These do not simulate
the physical buses, GPIO interrupt hardware, or current consumption.

On a watch, compare sleep current with touch wake Off and On; test repeated
button wake, the first touch after wake, brief presses and held-button recovery,
alarm/countdown wake and dismissal, reboot persistence, and sleep after time
sync. Measure with the debugger disconnected. Confirm charging indications
refresh on wake and touch reinitializes reliably after many sleep cycles before
claiming a battery-life improvement.

The 0.3.4 candidate containing these changes has not been installed or measured
on the watch yet. In particular, GPIO sense, short-press latching, and repeated
RTC2 sleep/wake need device validation with and without prior Bluetooth sync.
