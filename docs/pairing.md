# Phone connection and pairing — 0.3.21 candidate

After the 0.3.20 transfer, the user reported entering the phone's code but never
completing pairing. INFO showed **No saved phone**. That establishes an
incomplete bond, not which security phase failed. The 0.3.21 build
shows **Waiting for phone code**, the number of completed code-check rounds,
key verification, encryption, or identity exchange on the PIN screen. INFO
retains the last observed stage and S/D errors after failure. It has not been
flashed. A separate reproduced permission mismatch is fixed: authenticated,
encrypted feature writes no longer return an ATT authentication error while
identity exchange is pending. Their data stays queued until bonding completes;
failed security clears it. Connected status, ANCS and outgoing music controls
continue to require the completed bond. This preserves the reference NimBLE
distinction between ATT authentication and bond completion, and does not accept
pre-encryption data. Its role in the physical stall remains unconfirmed.

The host test now feeds incoming HCI ACL fragments of at most 27 bytes, matching
the controller's disabled data-length extension. The full passkey exchange
still passes, including reassembly of the 65-byte public key. It also confirms
Battery reads succeed after encryption but before identity distribution, so
requiring a saved bond for that read is not the cause. A 31-second stalled-code
test times out, clears the PIN and retains the stage. These are simulated host
checks; physical radio/encryption timing remains untested.

**0.3.19 booted successfully and was confirmed with KEEP on 2026-10-10.
Phone pairing remains unreliable. On the preceding 0.3.17, the user reported a crash before the code appeared
on the first attempt and incomplete pairing after code entry on the second.
Its unified phone connection, automatic time sync, and
companion behavior still need the on-device checks below.**
The local 0.3.18 candidate adds ANCS, moves host processing out of screen drawing
and feature packet frames, and defers passkey injection until its callback
unwinds. Investigation also reproduced a GATT restart defect: the second host
start reused ATT memory without resetting its old list or restoring service
definitions. The corrected port passes 100 real-host discovery cycles and ten
interleaved phone public-key/passkey exchanges. It is installed; these
tests do not establish that the physical watch crash is resolved.

On installed 0.3.18, InfiniLink stalled at Connecting and the watch showed
Securing connection without a code. INFO showed a saved phone. Clearing the
saved pairing enabled a brief code and iPhone prompt, but pairing did not
complete and the watch slept quickly. The installed 0.3.19 candidate
keeps Phone setup visible for up to two minutes and defers settings saves during
that window and while the link is securing. After successful authentication,
normal screen sleep resumes. The side button still sleeps immediately; leaving
Phone also permits normal sleep without closing the shared connection.

The user reported that 0.3.19 still slept quickly after waking Phone and still
lost the code/iPhone prompt. Investigation reproduced a missing wake signal:
controller-only pumping could queue HCI/SMP host events without returning the
input wait to the main loop. The local 0.3.20 candidate exposes that pending
host work through HasUpdate, retaining shallow callback dispatch, and renews
the setup window when waking Phone or returning from INFO. The real-host test
now asserts wake readiness before dispatching raw connection, ATT and SMP
packets. It fails with the previous wake policy and passes with this fix.
The direct uploader sent 0.3.20 and the watch verified it on 2026-10-10.
INSTALL and boot/KEEP have not been reported; physical pairing remains
unvalidated. The latest user-confirmed installed baseline is 0.3.19.

The reference also gives its controller task higher priority than the host.
goPine's key generation/ECDH originally ran without servicing controller events;
connection-event completion needs that task work to schedule the next radio event.
The candidate now pumps only controller work at each scalar-multiplication bit,
never recursively dispatching the host. The same generated source is used in
firmware and host tests; the pinned vendor checkout remains unchanged.

The real-host test now completes all 20 passkey rounds, verifies DHKey checks
and the derived LTK, enables encryption, distributes identity keys, saves
pre-pairing subscriptions under the permanent phone identity, reloads the flash
journal, and restores the authenticated connection and subscriptions. An invalid
confirm reproduces a failed pairing and verifies the error survives disconnect.
Controller events queued during key calculation execute while host callbacks
remain deferred. This is simulated HCI/flash testing, not physical iOS validation.

InfiniTime's `NimbleController::RestoreBond` restores peer keys after host sync,
installing identity keys in the controller as well as host memory. goPine's
journal reload omitted that controller step. Reset also disabled address
resolution, which the pinned host could leave disabled on subsequent starts.
The stronger test uses real host startup and reproduces a saved phone losing
its identity after a private-address change. The candidate restores controller
identity keys and enables resolution before advertising. Eight address changes
now preserve the authenticated bond and subscriptions with no additional flash
writes. A rejected controller restore reports an error and retains the bond for
retry. These results still use a simulated controller, not an actual iPhone.

The ANCS client added in 0.3.18 could not actually discover the phone's services:
NimBLE's client discovery/write defaults follow its central radio role, which
goPine disables. Real ATT/HCI testing reproduced `BLE_HS_ENOTSUP`; the earlier
client tests stubbed these operations. The 0.3.20 candidate explicitly enables
only the required GATT client operations on the existing peripheral connection.
The real-host test now receives immediate subscription-time notifications,
fragmented text and removals, confirms Service Changed indications, and
rediscovers ANCS after disappearance/republication. A denied-sharing retry leaves
the authenticated battery/weather connection usable. Physical iOS authorization
and delivery remain unverified.

`testdata/infinilink_wire.json` contains synthetic payloads derived from the
audited companion's calendar, weather, music and app-alert encoders. The real
host test sends those payloads over ATT and checks exact mailbox bytes, including
v0/v1 current weather and a valid five-day forecast. It also verifies outbound
music notifications and rejection of commands from an old connection. The Go
controller test applies the same packets to the clock, weather cache, music and
inbox without creating feature sessions. This replaces an earlier transport
weather fixture whose zero timestamp would fail the Go decoder. The stages are
tested separately; neither fixture set is a physical phone capture.

**Phone → INFO** now retains the last security/disconnect error with short text
and numeric `S`/`D` codes. A new connection attempt does not erase it; successful
security or Forget Phone clears it. This allows a short-lived prompt to be
diagnosed without trying to read transient status text.

On 2026-10-09, the user confirmed that 0.3.16 still crashes at **Weather →
Update → Done**, specifically after InfiniLink connects. The 0.3.17 candidate
removes that separate session from normal navigation and waits for physical
radio shutdown before settings saves. These changes address the reported path;
the user's confirmation does not establish the underlying crash cause or prove
the candidate fixes it.

## One connection for phone features

Open **Settings → Phone → CONNECT PHONE**, the only pairing entry in 0.3.18.
Pair once, then keep **AUTO CONNECT: ON** to use time, weather,
music, and supported notifications through one connection. Browsing these apps,
returning to the clock, and sleeping the screen do not close that connection.

The auto-connect preference is saved with watch settings and restored after a
confirmed boot. New installations and older settings default to Bluetooth Off.
The watch advertises so the companion can reconnect; the phone remains the
Bluetooth central and decides when to connect. This setting does not force a
suspended or stopped phone app to run. Firmware Update temporarily takes over
the radio. **BLUETOOTH OFF** disables automatic reconnection while retaining the
saved bond. Canceling pairing or choosing Forget Phone also turns auto-connect
off.

During a trusted, encrypted phone connection, valid Current Time Service writes
update the clock automatically. They no longer require a separate Sync Time
session and ACCEPT tap. Time and timezone/DST changes still depend on the
companion sending a fresh local time; the watch does not independently poll a
time server. Weather packets update the cache without closing the connection.
Music controls and metadata use the existing InfiniTime music service.

Phone status follows authentication, independently of music support. A trusted
phone can show **Connected** and send time/weather without subscribing to music
controls. Media errors stay on the Music screen. **INFO** shows saved pairing
details; it does not start a separate pairing session.

## iPhone / InfiniLink

1. With the 0.3.20 pairing repair, use InfiniLink's normal connection mode with
   **Force ANCS on**. Allow notification sharing when iOS asks.
2. Open **Settings → Phone** on the watch and tap **CONNECT PHONE**. In InfiniLink, select the
   watch advertised as **InfiniTime**. goPine uses a different Bluetooth identity
   from the InfiniTime recovery clock.
3. Enter the six-digit code shown on the watch. The candidate requests security
   when the phone connects, without waiting for an app Battery read. Check that InfiniLink receives
   battery data and **Phone → INFO** shows **Phone pairing saved**.
4. Allow the phone permissions needed for weather/location and Apple Music.
   Leave auto-connect enabled on the watch. Subsequent connections should reuse
   the saved bond instead of requesting another code.

InfiniLink remembers the device identifier as soon as its connection succeeds.
Its screen can still say Connecting until a Battery value arrives, so that read
is part of the pairing acceptance check. A Device Information Service is not
required to add the watch. Do not add a fake InfiniTime firmware version to work
around companion checks: the audited app treats versions beginning with 0 as
recovery firmware.

The audited InfiniLink source writes time when it discovers the Current Time
characteristic. Its device screen starts a weather fetch when the weather
characteristic changes; successful fetches push current weather and forecast.
Location updates can also start a fetch. Fetches are normally throttled to one
per two minutes, so a quick reconnect can skip a fetch without resending cached
data. These are companion behaviors, not watch-side refresh requests.

**Background weather is limited by the current companion.** InfiniLink's
periodic weather check is called from its motion-notification handler. goPine
keeps its unused accelerometer powered down and does not emit motion data, so
holding a connection alone does not provide that periodic trigger. Battery,
music, and step callbacks do not substitute for it. Use InfiniLink in the
foreground and its location/weather settings to refresh when needed. The watch
continues receiving any real weather pushes while connected and shows receipt
age; it does not claim a guaranteed refresh interval.

InfiniLink's music integration controls Apple Music. It does not establish
system-wide media control. Its app-generated alerts can use the shared phone
connection. The 0.3.20 ANCS client is intended to receive iPhone notifications directly,
retrieving title/message text and applying updates/removals in Inbox. Silent
and pre-existing notifications do not buzz. Apple identifiers and messages are
cleared when their ANCS session ends; Android messages are preserved.
Dismiss clears a local message, without dismissing it on the iPhone or answering
calls. Notification content depends on iOS sharing and preview permissions.

## Android / Gadgetbridge

Use Gadgetbridge's PineTime/InfiniTime integration for time, music controls,
battery, and supported notification forwarding. Enable its time synchronization
option if automatic clock setting is wanted. The audited initialization path
sends Current Time and Local Time, subscribes to music events, and reads and
subscribes to battery data. Permissions and the phone's background restrictions
still affect delivery and reconnection.

**Unmodified Gadgetbridge currently blocks weather for goPine's version.** Its
PineTime integration initializes firmware-version fields to zero and refuses to
send weather for firmware major versions below 1. Versions 1.8–1.13 use its old
weather protocol; versions 1.14+ use the Simple Weather Service implemented by
goPine. Exposing the same weather UUID is therefore insufficient. Reporting
goPine's actual 0.3.x firmware version would still hit that gate. Android weather
needs companion capability detection or explicit goPine support; the watch does
not pretend to be an InfiniTime 1.x release.

Gadgetbridge also requires a configured external weather provider, such as
Breezy Weather, once that compatibility issue is resolved. Weather is pushed
from that provider through Gadgetbridge; the Simple Weather Service has no
watch-to-phone refresh command. Its music handler accepts play, pause, next,
previous, and volume events, but ignores the `0xe0` metadata refresh hint used by
goPine. Metadata still depends on the companion's music updates.

## On-device acceptance

1. Installation and KEEP on 0.3.18 are confirmed. Enable Phone once and complete pairing with
   InfiniLink Force ANCS on and iOS notification sharing allowed. Confirm
   automatic time setting, battery display, Apple Music controls, and a real
   current-weather/forecast transfer with the actual companion version recorded.
   Deliver a real iPhone notification, modify/remove it, and reconnect. Check
   silent/pre-existing notifications do not vibrate and stale session IDs clear.
2. Switch among Phone, Music, Weather, and the clock; let the screen sleep.
   Confirm the shared connection remains available and incoming weather does
   not disconnect it. Check the weather receipt age instead of assuming that a
   connection implies fresh weather. Repeatedly enter and leave Weather after
   InfiniLink connects, then exercise BLUETOOTH OFF and a settings-save shutdown
   while connected to check the previously reported exit/crash path.
3. Leave range and return, lock the phone, then reboot the confirmed watch.
   Confirm the saved setting and bond allow reconnection without another code.
   Repeat after the phone rotates its private address. Test time received during
   pairing as well as after an already-bonded reconnect.
4. Verify **BLUETOOTH OFF** survives reboot and releases the radio. Test the
   Firmware Update handoff and return to normal phone use after canceling it.
5. Test pairing cancellation, incorrect code, timeout, removed phone bond,
   radio failure, and low battery. **Phone → INFO → FORGET PHONE → TAP TO
   CONFIRM** stops Bluetooth and erases the saved bond; also forget the watch in
   the phone's Bluetooth settings before pairing again.
6. Measure screen-off consumption with auto-connect off, connected, and waiting
   out of range. A successful connection does not establish battery endurance.

## Storage and radio behavior

One phone's keys and up to four characteristic subscriptions persist in the
internal bond journal at `0x7d000..0x7dfff`, separate from MCUboot scratch and
clock/settings storage. Writes use a CRC and a final commit word, are coalesced
for one second, and flush at shutdown. Avoid resetting while **Saving pairing...**
is shown. Interrupted appends preserve the previous record. Reclamation occurs
with BLE off and at least 20% battery; power loss during that erase can require
pairing again. Storage errors are displayed, and keys are not logged.

Subscriptions created before first pairing completes are saved only after the
phone's learned identity and both keys are stored. This corrects a pinned-stack
omission while avoiding subscriptions tied to the temporary private address
used for initial pairing. Reconnect restoration still needs the physical-phone
checks above, even though the upstream host-path regression passes locally.

The connected-radio shutdown repair defers host work during HCI waits and resets
the controller after disconnect callbacks unwind. Version 0.3.16 additionally
allows ordinary unencrypted Battery reads during the explicit firmware-update
session, avoiding Linux's automatic pairing probe. Phone sessions retain their
encryption requirement; this updater exception does not replace a saved phone
bond.

## Source audit

The implementation uses protocol facts from these sources; companion code is
not copied into the firmware. Source behavior is distinct from compatibility
with every released App Store, TestFlight, or Gadgetbridge build.

- InfiniLink `60abe2d2c67aff67726855374385a499d9353a94`:
  [connection and restoration](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/BLEManager.swift),
  [time, battery, music and motion callbacks](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/BLECharacteristicHandler.swift),
  [calendar encoder](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/SetTime.swift),
  [weather, music and alert encoders](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/BLEWriteManager.swift),
  [foreground weather trigger](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/Core/DeviceView.swift),
  [weather refresh policy](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/Utils/WeatherController.swift),
  [location handling](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/Utils/LocationManager.swift),
  [music handling](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/Utils/MusicController.swift).
- Gadgetbridge `8449c3e99498c81535177db68cbf7b98a2e5ae68`:
  [PineTime initialization, music events, and weather version gate](https://codeberg.org/Freeyourgadget/Gadgetbridge/src/commit/8449c3e99498c81535177db68cbf7b98a2e5ae68/app/src/main/java/nodomain/freeyourgadget/gadgetbridge/service/devices/pinetime/PineTimeJFSupport.java),
  [weather provider integration](https://gadgetbridge.org/internals/development/weather-support/).
- InfiniTime `6c119eb52206b580b556b41633dddc1e1b66a8da`:
  [Simple Weather Service](https://github.com/InfiniTimeOrg/InfiniTime/blob/6c119eb52206b580b556b41633dddc1e1b66a8da/doc/SimpleWeatherService.md).
- Apple: [Core Bluetooth background processing and restoration](https://developer.apple.com/library/archive/documentation/NetworkingInternetWeb/Conceptual/CoreBluetooth_concepts/CoreBluetoothBackgroundProcessingForIOSApps/PerformingTasksWhileYourAppIsInTheBackground.html).
