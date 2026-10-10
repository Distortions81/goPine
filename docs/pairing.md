# InfiniLink pairing — 0.3.15

**0.3.14 is the last confirmed boot/KEEP baseline. A local 0.3.15 image
completed direct transfer and on-watch verification; boot/KEEP and phone
interoperability remain unconfirmed.**
The user reports Weather → Update → Done crashes only after InfiniLink connects.
The port could dispatch host disconnect events while synchronously waiting for
an HCI command acknowledgement. That reentered termination before its connection
count was updated. The fix defers host work during HCI waits and moves final
radio reset outside the disconnect callback. A regression test covers that
ordering; it does not establish the physical watch's crash cause conclusively.

The previous build could connect but had SMP, encryption and bonding disabled.
This candidate enables authenticated LE Secure Connections with a six-digit code
shown on the watch and entered on the phone. Battery reads require encryption,
triggering the phone pairing flow. One phone's keys and up to four notification
subscriptions persist in a separate internal flash journal. Bluetooth is still
Off by default after reboot; saved keys do not turn the radio on automatically.

## On-device acceptance

1. Install and KEEP the candidate. Leave InfiniLink's Developer → Force ANCS off
   initially; this build does not implement Apple's notification service client.
2. On the watch, open Music → LINK → CONNECT 10 MIN (or STAY CONNECTED when a
   persistent connection is wanted). In InfiniLink, select the goPine watch
   advertised as InfiniTime, distinct from the recovery clock's Bluetooth identity.
3. Enter the watch's six-digit code on the iPhone. Confirm InfiniLink receives
   battery data. On the watch, LINK → PAIR should show **Phone pairing saved**.
4. Send current weather and forecast. Test Weather → Update → Done while connected,
   then reopen it several times; also test Done while advertising without a peer.
5. Disconnect/reconnect and reboot the watch. Open a phone connection again;
   the existing iOS bond should encrypt without requesting a new code. Repeat
   after the phone has rotated its private address and while the phone is locked.
6. LINK → PAIR → FORGET PHONE → TAP TO CONFIRM stops BLE and erases its bond.
   Also Forget This Device in iOS Bluetooth settings before pairing again.
7. Verify Cancel during passkey entry, an incorrect code, pairing timeout,
   out-of-range disconnect, low battery, and alternating sync/weather/update
   windows. Confirm Off releases the radio and retains the previous sleep power.

The implementation is based on the pinned InfiniLink source audit at
`60abe2d2c67aff67726855374385a499d9353a94` (normal battery read, CTS write and
optional RequiresANCS connection option), and InfiniTime/NimBLE at
`6c119eb52206b580b556b41633dddc1e1b66a8da`. Supporting app-level weather/music
and saving a Bluetooth bond do not by themselves implement iPhone system-wide
notification forwarding. Test Force ANCS separately after the core flow passes.

## Storage and limitations

Bond data occupies the formerly spare internal page `0x7d000..0x7dfff`, separate
from MCUboot scratch and the clock/settings journal. Writes use CRC and a final
commit word, coalesced for one second and flushed at shutdown. Avoid resetting
while **Saving pairing...** is shown. Interrupted appends retain the prior record.
Page reclamation occurs only with BLE off and battery at least 20%; power loss
during reclamation can lose the bond and require pairing again. A storage failure
is displayed rather than claiming the bond was saved. Keys are not printed.
