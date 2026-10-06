# goPine roadmap

Updated 2026-10-05. Current installed firmware: **0.3.2**. The user confirmed
that goPine works, tapped KEEP, and reported that it was working well. This is
a successful boot and general-use smoke test; focused hardware checks remain.

## Completed

| Area | Delivered | Validation |
|---|---|---|
| Watch UI | Clock, battery/charging display, settings, manual time/date, 12/24-hour format, display sleep and wake | Installed and used on the watch; focused touch/hold checks remain |
| Clock tools | Five alarms, snooze, stopwatch/lap, countdown, alert wake and vibration | Host event-loop tests; countdown wake/vibration confirmed on 0.3.1 |
| Persistence | Versioned flash journal for settings and timers, reboot reconciliation, planned-reset time handoff | Host recovery/interruption tests; dedicated watch reboot/resync checks remain |
| Recovery OTA | Stock MCUboot/InfiniTime recovery, DFU ZIP packaging, deliberate update hold, KEEP/REVERT | Recovery, transfer, KEEP and REVERT exercised; repeated confirmed updates through 0.3.2 |
| Time sync | Bounded five-minute BLE window, user approval, PC sender, radio restart fix, Battery Service for InfiniLink | One PC sync confirmed on 0.2.5; restart fix and phone compatibility included in 0.3.2 but need focused tests |
| Memory and binary size | Smaller strip buffer, selective formatting/SHA-256 code, font subsets and lossless row RLE | Resource gates, integrity/font tests, firmware builds and 0.3.2 boot/KEEP |
| CPU and drawing | Reused callbacks, numeric frame keys, fixed text buffers, packed fills, off-strip rejection, indexed glyph rows, precomputed corners | Pixel comparisons, allocation checks and host benchmarks |

The optimization pass reduced linked flash from **313,036 to 209,692 bytes**
and the heap-allocated display strip from **5,760 to 2,880 bytes**. The heap
region grew from **42,304 to 45,152 bytes**; this is not measured free runtime
heap. Tested steady-state rendering paths allocate nothing after warmup on the
host. See [performance measurements](performance.md) for build conditions,
benchmark results, stack budgets and limitations.

## Short-term development plan

Work in four increments, keeping each independently buildable and testable.
Fix any boot, persistence or timer failure before proceeding with optimization.
Assign release versions when a candidate is ready, rather than promising dates.

### Resource envelope

The current MCUboot linker provides 64 KiB of RAM. The OTA package limit is
471,040 bytes, including image metadata; the installed 0.3.2 image is 209,828
bytes. Linked flash measurements above exclude packaging differences.

| Resource | Short-term constraint |
|---|---|
| Packaged image | Aim to stay below 256 KiB; this is a proposed working budget, not the bootloader limit. Spend the remaining allowance on measured CPU savings and useful diagnostics. |
| Heap region | Aim to retain at least 44 KiB; the existing build gate requires 40 KiB. Review static RAM growth against actual runtime measurements. |
| Task stack | Keep 8,192 bytes and the existing 2,048-byte selected-path reserve. Do not reclaim stack space based only on a successful boot. |
| Display buffer | Keep the single 2,880-byte strip initially. Any larger buffer must justify its RAM cost with on-watch measurements. |
| Diagnostics | Budget at most 512 bytes of additional persistent RAM initially, counted within the RAM budget; fixed counters/aggregates, no unbounded sample history or per-frame text logging. |
| Rendering allocations | Preserve zero allocations after warmup in the tested hot paths; verify TinyGo behavior as well as host tests. |
| Runtime headroom | Measure minimum free heap and stack use before setting an enforced runtime margin. The heap-region size alone does not establish safe headroom. |

These working budgets are planning targets; only the existing resource checks
are enforced today. Keep one application task, bounded BLE sessions and the
current lossless fonts. No full framebuffer or runtime font renderer is planned.

### 1. Establish a measurable, reliable baseline

- Add an optional diagnostic build with fixed counters for render CPU time,
  display transfer time, strips/bytes sent, input-service delays and available
  runtime memory statistics. Inspect TinyGo support before choosing a heap or
  stack measurement method. Export results on demand without writing samples
  to flash or keeping BLE running.
- Capture clock, stopwatch, settings and hold-feedback workloads, plus settings
  saves and repeated sync windows. Separate measurement overhead from the
  normal build and retain the build/resource report with each result.
- Run the basic 0.3.2 checks: reset after KEEP, settings/timer persistence,
  countdown/alarm wake, snooze and repeated PC sync. Record failures separately
  from tests that require phone or measurement equipment not yet available.

**Done when:** a reproducible on-watch baseline exists, basic reliability checks
pass, and remaining unknowns are explicitly recorded. Current measurement can
follow when equipment is available; host timing cannot substitute for it.

### 2. Reduce work for small UI changes

- Add a bounded dirty-strip mask for the 30 display strips. Initially mark the
  old and new bounds of changed clock/status fields, stopwatch digits and hold
  feedback. Screen transitions and uncertain invalidation redraw the full UI.
- Repaint marked strips with all intersecting content so disappearing text,
  overlays and changed backgrounds cannot leave stale pixels. Keep the current
  strip hashes as a final guard against unnecessary transfers.
- Compare against the existing complete-render path, including randomized
  transitions and pixel comparisons. Measure CPU time, bytes sent and input
  latency on the watch before extending selective redraws to more screens.

**Done when:** common small updates rasterize fewer strips, produce identical
pixels, retain allocation/resource budgets and show a measurable on-device
improvement without worse input or timer service. Retain the simpler current
path for any screen where selective redraws do not help.

A 240×240 RGB444 frame contains 86,400 pixel bytes. At the configured 8 MHz SPI
rate, pixel transmission alone has a theoretical minimum of 86.4 ms. Full-screen
high-frame-rate animation is therefore outside this plan; changing less of the
screen is more useful than optimizing glyph decoding alone. Existing strip
hashes already skip unchanged transfers, so dirty tracking primarily removes
unnecessary rasterization/hash work unless it also narrows transfer regions.

### 3. Reduce unnecessary wakeups and radio work

- Measure loop wakeups with the display asleep and awake. Audit deadlines for
  visible updates, alarms, countdowns, touch and radio service; remove redundant
  polling only where the platform can still wake for the required event.
- Verify radio/crystal teardown after cancellation, sleep and expiry, followed
  by repeated successful sync. Preserve the five-minute opt-in sync model.
- Measure current in sleep, clock display, stopwatch and sync states when
  equipment is available. Compare identical brightness and workloads, and
  check that diagnostics themselves do not prevent low-power operation.

**Done when:** all timer/input/sync behavior remains correct, unnecessary work
is demonstrably reduced, and any energy-saving claim has physical measurements.
Defer power conclusions if only software wakeup counts are available.

### 4. Stabilize and install the next candidate

- Run a 24-hour mixed-use watch trial with repeated sleep/wake, timers, settings
  saves and sync windows. Record resets, missed alerts and minimum observed
  memory headroom rather than relying only on an end-of-run snapshot.
- Complete relevant host, pixel, allocation and resource checks; build every
  firmware target. Summarize image size, RAM budgets and measured performance
  changes against 0.3.2.
- Upload through recovery, confirm KEEP, reset and verify retention. Check
  rollback and persistence after the relevant changes, using the detailed
  hardware checklist below.

**Done when:** the candidate passes the recorded acceptance checks and improves
the measured workloads within the resource envelope. Keep experimental buffer
sizes, further library replacements and extra precomputed tables out of this
candidate unless profiling demonstrates their value.

## Next: validate the installed build

Complete these checks before treating 0.3.2 as a fully validated baseline.
Record the firmware version and observed result in the relevant validation doc.

- [ ] Reset after KEEP and verify goPine remains installed. Check settings,
  paused/running timers and approximate time restoration across planned resets,
  then accept time sync and verify timer reconciliation.
- [ ] Exercise alarm dismissal, five-minute snooze, simultaneous alerts and
  countdown wake/vibration on 0.3.2, including while the display is asleep.
- [ ] Check clock/tools/settings navigation, sleep/wake, update-hold timing,
  early-release/drag-away cancellation and redraw responsiveness.
- [ ] Repeat PC sync windows without rebooting. Exercise Back, sleep, expiry
  and reconnection, and verify that the radio shuts down after each window.
- [ ] Test InfiniLink with **Force ANCS off**, including connection, battery
  reading, time proposal and on-watch acceptance.
- [ ] Exercise rollback on the current build and a deliberately canceled BLE
  transfer followed by reconnect. Test staging/swap power interruption only
  with wired SWD recovery available.

Details and prior evidence: [OTA](ota.md), [clock tools and persistence](timers.md),
and [time sync](time.md). A successful earlier-version test is useful evidence,
but does not replace a focused check after relevant changes.

## Next: measure CPU, display and power on the watch

Use the flash headroom to improve responsiveness and energy use where actual
measurements justify it. Further binary shrinking is secondary to those goals.

- [ ] Establish on-device timing for clock, stopwatch, settings and hold-feedback
  frames. Separate rasterization/hash work from SPI transfer time and record
  input-to-display latency, including slow frames.
- [ ] Measure minimum free heap, allocation/GC behavior and stack headroom during
  drawing, persistence, sync and update validation. Run an extended mixed-use
  session to look for memory growth, resets and missed timer deadlines.
- [ ] Measure display-off, display-on and BLE-window current, including after
  repeated sync windows. Check battery estimates against observed discharge
  before claiming battery-life improvements.

The watch already uses 8 MHz SPI DMA, the DC/DC regulator and the low-frequency
RTC. Host render benchmarks exclude display transport and do not establish
watch latency or energy savings.

## Then: optimize measured bottlenecks

These are experiments to select from after profiling, not promised changes.

- Compare strip heights and SPI transaction costs against RAM use and input
  latency. The eight-row strip saves RAM but increases full-frame transfers.
- Investigate tracking changed screen regions to avoid rasterizing and hashing
  unchanged content, while preserving correct redraws when overlays or text
  disappear.
- Spend flash on additional glyph indexes or precomputed drawing data when they
  reduce measured CPU work without increasing live RAM disproportionately.
- Review remaining library costs using linked symbols and profiles. Replace
  only the parts with demonstrated benefit and maintainable, tested equivalents.
  Preserve firmware integrity checks and font coverage.

Accept an optimization only with a reproducible before/after result, unchanged
pixels or an intentional UI improvement, responsive touch/timer handling, and
passing resource budgets. Keep race/vet checks, font coverage comparisons,
SHA-256 tests and all firmware-target builds as relevant regression gates.

## Scope and release evidence

Keep the current focus on a reliable, efficient clock and its tools. Background
BLE connections, notifications/ANCS, bonding, automatic timezone/DST rules and
additional apps are not scheduled. Font compression remains lossless; a runtime
font rasterizer or reduced coverage precision needs a measured benefit before
being considered.

For each hardware candidate, retain the package checksum, build/resource report
and concrete test results. Distinguish receiver validation, successful boot,
KEEP confirmation and behavior after reset; none alone proves all four.
