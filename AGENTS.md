# Watch updates

When the user asks to update or flash the watch, assume they have already put
it in **goPine Settings → Firmware Update**, showing **Ready to connect**.
Start the prepared uploader immediately. Do not ask an initial readiness or
mode question, request another confirmation, or run a preliminary scan that
delays the connection. The request authorizes the firmware transfer.

Use **goPine's in-app update by default**. Use InfiniTime recovery only when
the goPine update method is broken or the user explicitly says recovery is
running. Do not assume recovery merely because a previous update used it.

The configured watch uses the TP-Link USB adapter **hci1**. goPine's address
is **C9:9E:15:7A:69:B4**; recovery uses **C9:9E:15:7A:69:B5**. Do not enable
the antenna-less MediaTek adapter. Start the already prepared, validated package
with the appropriate mode:

```sh
build/ota/venv/bin/python -u scripts/ota_update.py VERSION \
  --package build/ota/gopine-dfu-VERSION.zip \
  --adapter hci1 --address C9:9E:15:7A:69:B4 --direct --ready
```

For actual recovery, use the recovery address and omit `--direct`.
If the package still needs preparation, finish that work before asking the
user to reopen a short connection window. Ask for a restart or mode clarification
only after an actual failure or conflicting evidence. Describe what failed and
whether any firmware was sent. Never retry recovery activation after successful
receiver validation merely because its reset acknowledgement is missing.

In-app uploads require the user to tap **INSTALL**, then **KEEP** after boot.
Recovery uploads activate automatically and require **KEEP** after boot.
After a verified upload, give a brief result and any necessary INSTALL/KEEP
instructions, then continue or finish. **Do not ask whether boot succeeded or
KEEP was tapped after every update.** The user will report any update failure.
Record transfer validation separately from user-confirmed boot and KEEP;
only mark boot/KEEP confirmed when the user volunteers that information. Do not
turn an unconfirmed boot into another question or block subsequent work on it.
