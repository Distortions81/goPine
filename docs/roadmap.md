# goPine roadmap

Updated 2026-10-10. Public release: **0.3.16**. Installed and user-confirmed
boot/KEEP: **0.3.19**. Receiver-verified direct upload: **0.3.20**, awaiting
reported installation/boot. The user subsequently reported incomplete code
entry and **No saved phone**. **0.3.21** is a prepared candidate fixing an ATT
authentication error during pending identity exchange and showing security
phases/completed code-check rounds; it is not flashed.
Installed 0.3.19 still fails InfiniLink pairing: setup sleeps quickly and the
watch/iPhone passkey prompt disappears. A successful firmware transfer or host
test is not phone compatibility. See [pairing evidence](pairing.md),
[diagnostic changes](releases/0.3.21.md), and [transfer history](ota.md).

The user selected phone notifications, music and weather on both iPhone and
Android. The immediate priority is one reliable saved pairing and reconnect,
then proving those features work on that connection. Screen sleep should preserve
the connection. Settings → Phone is the single entry; app-specific pairing
windows are no longer exposed by normal navigation.

## InfiniLink contract and gaps

This comparison uses the source revisions listed in [the source audit](pairing.md#source-audit).
It describes the inspected development code, not every App Store release.

| Feature / contract | Current source | Evidence still needed / gap |
|---|---|---|
| Add device | Advertises InfiniTime, a stable identity distinct from recovery | InfiniLink filters that name; a completed physical add remains unverified |
| Pair once and reconnect | Secure Connections, Display Only passkey, MITM, one durable bond; controller identity keys restored after reset | Full simulated exchange, real host startup, journal reload and eight private-address reconnects pass; actual iOS pairing still fails on installed 0.3.19 |
| Connection status | Standard Battery read/notify | InfiniLink waits for a battery response before showing Connected; verify that response on iPhone after pairing |
| Automatic time | Shared authenticated CTS, no ACCEPT step in Phone mode | InfiniLink writes during discovery; the host test rejects the pre-auth write and accepts a trusted retry; verify physical clock/timezone changes |
| Current weather / forecast | Shared Simple Weather Service; cached conditions and five forecast days | Verify foreground delivery and reconnect; InfiniLink uses motion callbacks for periodic background checks, which goPine does not supply |
| Music | InfiniTime controls, metadata and refresh hint | Host tests exercise trusted metadata writes; verify Apple Music and Gadgetbridge commands physically |
| iPhone notifications | ANCS discovery, sharing authorization, bounded attribute parsing, add/modify/remove; required GATT client operations enabled | Real host/GATT tests cover immediate notifications, fragmented text, removal, Service Changed and denied-sharing retry across saved-bond reconnects; physical iOS behavior remains unverified |
| Android / companion alerts | InfiniTime ANS receiver and local inbox | Host test exercises the authenticated characteristic; verify real companion forwarding |
| Device information | No Device Information Service yet | InfiniLink treats numeric major 0 as recovery and applies firmware version gates; reporting goPine 0.3.x needs companion-aware handling, not a fake InfiniTime version |
| Motion / steps / HealthKit | Not implemented; unused accelerometer remains off | Genuine sensor data and power validation required; do not manufacture motion callbacks to trigger weather |
| Heart rate | Not implemented | Driver, acquisition, reporting and power validation required |
| Navigation | Not implemented | Companion UUIDs/format, guidance UI and validation required |
| Companion settings/resources | InfiniTime BLEFS not implemented | goPine UI/settings differ; do not expose an incompatible filesystem as working |
| Firmware updates | Browser / Python custom in-app update, recovery Legacy DFU | InfiniLink's Nordic Legacy DFU cannot update normal goPine directly yet |
| System-wide iPhone media / remote notification actions | Apple AMS and ANCS actions not implemented | Separate services and controls; existing Apple Music and local dismissal do not imply these capabilities |

## Pairing repair evidence

The source uses InfiniTime's pinned NimBLE host and controller. Its cooperative
port must preserve behavior supplied by InfiniTime's FreeRTOS tasks. The 0.3.20
candidate fixes raw host events that did not return the input wait to the main
loop, renews setup visibility after waking Phone, and services controller work
through key generation/ECDH. InfiniTime runs that controller at a higher task
priority; it schedules subsequent connection events outside the radio IRQ.
Phone → INFO now retains security/disconnect failure details through retries.
InfiniTime's bond restore also revealed missing controller identity-key restore
and address resolution after reset. Both now run after host sync and before
advertising; saved keys are not rewritten to flash.
The real GATT test found that disabling the central radio role had implicitly
disabled ANCS's discovery/writes. The required GATT client operations are now
enabled on the existing peripheral connection, with no scanning role added.

The host integration test runs 100 updater discovery cycles, ten public-key
handoffs, a deliberately invalid confirmation, a full 20-round passkey exchange,
identity/key distribution, CCCD persistence under the permanent phone identity,
flash-journal reload and authenticated reconnect. Time, weather, music metadata
and companion alert writes are tested before/after eight private-address
reconnects. Real host startup and injected controller restore failures are
covered, with no additional flash writes and the saved pairing retained.
Controller work queued during ECC runs while host callbacks remain deferred. The actual nRF
radio, interrupts, phone UI and battery are not simulated.
Real ATT exchanges also exercise immediate ANCS notification delivery,
fragmented attributes, removal, service disappearance/republication and a
denied-sharing retry that preserves access to battery/weather.
Shared source-derived companion fixtures cover both the real-host mailbox path
and Go's automatic clock/weather/music/inbox path; outbound music controls and
stale-generation rejection are checked over ATT. The fixtures replace a prior
transport weather packet with a zero timestamp that Go would reject. The stages
are tested separately and do not replace a physical phone exchange.

## Acceptance before calling phone features ready

- Pair in InfiniLink with normal ANCS requirements, keep the code visible and
  finish iOS authorization; confirm battery/status and saved pairing.
- Reconnect after screen sleep, phone Bluetooth cycling and watch reboot using
  the saved bond; test phone private-address rotation and explicit Forget.
- Receive automatic time, real current/forecast weather, Apple Music commands
  and ANCS notifications; repeat on Gadgetbridge for the shared services.
- Test denied sharing, wrong code, removed messages, partial/long text and alarms
  during phone traffic; retain usable failure reasons.
- Measure connected idle, disconnected advertising, sleep drain and runtime
  stack/heap headroom. Boot/KEEP does not prove power improvements.

Keep the 8,192-byte task stack, 2,048-byte selected-path reserve, 2,880-byte display
strip and hard 40 KiB heap-region floor. The current report also accounts for
selected C crypto/controller frames. These are regression budgets, not a measured
whole-call-graph bound. See [performance](performance.md) and [power](power.md).

Update compression is deferred. First measure transfer time with the scheduling
repair; optional progress notifications remain separate unfinished work.
Further standalone tools, alarm repeat refinements and new sensor/navigation
features follow a stable phone connection and battery measurement.
