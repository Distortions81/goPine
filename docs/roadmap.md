# goPine roadmap

Updated 2026-10-09. Release: **0.3.15**. Last confirmed boot/KEEP: **0.3.14**.
The [browser updater](https://distortions81.github.io/goPineTime/) offers published
firmware automatically. A local 0.3.15 image completed direct transfer and watch
verification; its boot/KEEP and phone interoperability checks remain pending.
Version **0.3.14** adds charger-event wake screens with
a large percentage. It includes 0.3.13 in-app updates with Cancel, progress and
local Install confirmation, plus battery reporting alongside
the 0.3.11 notification inbox, 0.3.10 rendering and 0.3.9 phone/music.
See [performance](performance.md) for host benchmarks and
firmware budgets.
The 24-hour battery comparison is pending. Boot confirmation does not establish
that the reported sleep-drain regression is resolved.

The user selected **phone features: notifications, music and weather**, supporting
both iPhone/InfiniLink and Android/Gadgetbridge, starting with the easiest shared
integration. This replaces the earlier standalone-tools-first direction.

## What the comparison showed

[InfiniTime's application guide](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/Applications.md)
includes clock tools, music, weather, navigation and sensor apps. goPine already
has alarms, snooze, stopwatch/lap and countdown. Its main functional gap is the
phone connection and services, rather than more timer screens.

[Bangle.js's scheduler](https://github.com/espruino/BangleApps/blob/master/apps/sched/README.md)
shows useful later refinements: custom repeat days, adjustable snooze and alert
patterns. Those are secondary to the selected phone work.

| Feature | Shared route / platform difference | goPine priority |
|---|---|---|
| Current weather and forecast | InfiniTime Simple Weather Service, already used by both companions | Implemented locally in 0.3.8/0.3.9; phone validation pending |
| Music controls and track text | InfiniTime music service works through companions; InfiniLink documents Apple Music support, not system-wide media support | Implemented locally in 0.3.9; phone validation pending |
| Android notifications | Companion forwards InfiniTime ANS packets during an opt-in phone connection | Implemented locally in 0.3.11; phone validation pending |
| iPhone notifications | Apple ANCS client with authorized access, service discovery and pairing/bonding lifecycle | Separate integration after the connection/security foundation |
| System-wide iPhone media | Apple Media Service, separate from the InfiniTime music service | Later extension, not implied by Apple Music controls |
| Navigation | Companion-generated directions, another service and presentation layer | Later |
| Steps / heart rate | New sensor drivers, algorithms and power validation | Deferred while battery behavior is being measured |
| Quick timers / calendar / display controls | Useful standalone additions without persistent radio | Backlog; the quick-timer experiment was set aside |

The [InfiniLink development README](https://github.com/InfiniTimeOrg/InfiniLink)
explicitly distinguishes Apple Music support from system-wide media, and warns
that its development branch may differ from released apps. Do not promise
all music players or notifications based on companion discovery alone.

## First increment: weather

The local candidate adds Weather under Apps, current conditions, five
forecast days, Celsius/Fahrenheit selection, and cached data after disconnect.
Update opens a **60-second** foreground radio window. It closes when both valid
records arrive, on Done/Back/sleep, on an alarm taking over, on error, or expiry.
The cache and unit choice are RAM-only; weather writes never enter the settings
journal. Existing time synchronization still requires approval in Sync Time.

The implementation follows the
[Simple Weather Service](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/SimpleWeatherService.md):
current packets v0/v1 and forecast v0, with bounded transport mailboxes and
transactional validation. The source audit used pinned InfiniTime
`6c119eb52206b580b556b41633dddc1e1b66a8da` and InfiniLink development commit
`60abe2d2c67aff67726855374385a499d9353a94`. This is protocol compatibility work;
actual iPhone and Android exchange still needs hardware testing.

InfiniLink sends UTC timestamps despite the protocol's local-time wording. The
UI therefore labels **receipt age**, never guessed observation age. Forecast
labels use the watch's confirmed local date at receipt; an uncertain clock
shows Day 1–5. The first forecast day is tomorrow. Data received at least 24
hours ago is marked out of date. Incoming time writes during weather updates
are acknowledged but do not silently set the clock or close the weather window.

**Acceptance:** receive real current/forecast data from both companions; test
long ATT writes, reconnect, bad/partial updates, screen sleep, alarm interruption,
Fahrenheit and negative temperatures. Confirm radio/current returns to baseline.
The user explicitly requested the 0.3.14 installation; it supersedes the 0.3.7
battery trial. Start the next comparison from this confirmed baseline.

## Second increment: connected mode and music

Implemented in the local **0.3.9** candidate: Off, a ten-minute session, and
opt-in Stay Connected. The latter two survive screen sleep and alarms. Off,
expiry, errors and explicit time/weather/update operations shut the radio down;
reboot defaults to Off. The candidate replaces the hardware's 20 ms BLE sleep
loop with queue/callout deadlines and an interrupt-safe WFE handoff, and enables
InfiniTime's 1,500 µs NimBLE radio-clock management. Reconnect advertising slows
to 1–1.5-second intervals after initial discovery or link loss.

Music now provides play/pause, previous/next, volume controls, and bounded
track/artist text. Subscription is required; playback state comes from the phone.
Media data clears on disconnect and connection generations prevent old gestures
from reaching a new peer. Incoming weather can update during the connection.
See [implementation and validation](time.md#phone-connections-and-music-candidate-039).

**Acceptance still open:** measure connected-idle, disconnected advertising,
active music, and screen-off current; verify HFXO settles between radio events.
Validate both Gadgetbridge and Apple Music through the installed InfiniLink
version, including reconnect and metadata refresh. Generic iPhone players need
[Apple Media Service](https://developer.apple.com/library/archive/documentation/CoreBluetooth/Reference/AppleMediaService_Reference/Specification/Specification.html).
In 0.3.12, Battery Service refreshes from existing UI samples, with notifications
when the percentage changes. Sleep adds no ADC reads; the last sample remains
visible until the watch next samples battery while awake.

## Third increment: notifications and pairing

The local **0.3.11** candidate implements InfiniTime's ANS receiver, a four-message
RAM inbox, unread clock indicator, paged text, local dismissal/Clear, a short
rate-limited vibration and quiet mode. Source/sender text is presented as supplied
by the companion. Messages do not wake the screen or replace active alarms;
the timer motor pattern takes priority. Nothing in the inbox is written to flash.
Bluetooth remains opt-in. See [behavior and limits](time.md#notification-inbox-candidate-0311).

Android forwarding is ready for hardware validation. InfiniLink-generated app
messages can use the same transport, but system-wide iPhone notifications still
require the separate ANCS/security integration below. Local dismissal does not
dismiss anything on the phone; this transport has no remote notification IDs.

Android forwarding and iOS are different transports. For iOS,
[ANCS](https://developer.apple.com/library/archive/documentation/CoreBluetooth/Reference/AppleNotificationCenterServiceSpecification/Specification/Specification.html)
requires authorized characteristic access and handling of service appearance,
notification additions/modifications/removals, and fragmented attribute responses.
The 0.3.15 candidate implements LE Secure Connections, passkey display, a
Forget Phone action and durable bond storage distinct from ordinary settings.
InfiniLink pairing/reconnect and the connected Done-crash fix require hardware
validation; see [the pairing test sequence](pairing.md).
Do not treat enabling Force ANCS in InfiniLink as sufficient support.

**Acceptance:** permission denial, locked phone, reconnect after reboot, removed
notifications, long/unsupported text, simultaneous alarms and bounded inbox
replacement. Verify power and memory under sustained traffic on both platforms.

## Delivered baseline and resource envelope

| Area | Installed 0.3.14 status |
|---|---|
| UI and time | Large clock, battery/charging, manual date/time, 12/24 hour, sleep/wake, opt-in touch wake |
| Clock tools | Five alarms, once/daily/weekdays, snooze, stopwatch/lap, countdown and vibration |
| Storage | Settings/timer journal, power-loss recovery and planned-reset time handoff |
| OTA | Recovery boot/KEEP confirmed through 0.3.14; local 0.3.15 direct transfer/watch verification passed, boot/KEEP pending |
| Time sync | Explicit five-minute CTS session and Battery Service; repeated phone sync still needs focused tests |
| Sleep | PORT interrupts, deadline waits, touch/peripheral shutdown; actual sleep current still unmeasured |
| Optimization | Fixed formats, partial timer redraws, flash CRC table, no recurring settings-key allocation |

Installed image: **246,460 bytes**; heap region: **45,196 bytes**. The earlier 0.3.7
optimization removed 8,372 linked flash bytes, reduced static reservation by
1,508 bytes and avoided an 8,192-byte CRC heap table. These are memory budget
components, not measured free runtime heap. See [performance](performance.md).

Keep the 8,192-byte task stack, 2,048-byte selected-path reserve and 2,880-byte
single display strip. Aim for an image below 256 KiB and a heap region of at least
44 KiB; the hard heap gate remains 40 KiB. Measure new BLE/security allocation
peaks rather than assuming the available flash also means available RAM.

## Hardware checks still open

- Compare controlled 24-hour drain on 0.3.14 with matching screen/radio use.
- Reset after KEEP; verify settings, running/paused timers and time handoff.
- Exercise alarm/countdown wake, snooze, simultaneous alerts and brief button presses.
- Repeat Sync Time sessions, including Back, expiry and sleep; verify radio shutdown.
- Check InfiniLink with Force ANCS off and record the app version.
- Measure on-watch render/input latency, minimum heap and stack headroom.
- Record per-candidate receiver validation, boot, KEEP, reboot behavior and checksum.

Details: [OTA](ota.md), [time and phone integration](time.md),
[clock tools](timers.md), [power](power.md). Host tests and simulator views do
not replace physical radio, battery or reboot validation.
