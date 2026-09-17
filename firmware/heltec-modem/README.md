# Ghost Modem for Heltec WiFi LoRa 32 V3

The same firmware runs on every Heltec WiFi LoRa 32 V3. It exposes raw SX1262 packets over the board's USB-to-UART bridge and has no Ghost Cache application semantics.

From the repository root, build and upload with:

```bash
pio run --project-dir firmware/heltec-modem
pio run --project-dir firmware/heltec-modem \
  --target upload \
  --upload-port /dev/ttyUSB0
```

Use `ghost-radio info --device /dev/ttyUSB0` for normal diagnostics, replacing the path for your system. A terminal monitor is useful only during reset to inspect readable boot diagnostics; normal traffic is binary framed data.

At reset, readable bring-up diagnostics are printed at 115200 baud before the binary protocol starts. Heltec V3 Vext on GPIO36 is active-low and is enabled before the OLED is reset and initialized.

Radio defaults are centralized in `RadioConfig` near the top of `src/main.cpp`: 915 MHz, SF7, 125 kHz, CR 4/5, 5 dBm, eight-symbol preamble, and private sync word `0x12`. Runtime RF configuration is supplied by the Linux host and is not persisted by firmware. OLED mode is persisted in ESP32 NVS.

See [Hardware and firmware](../../docs/hardware.md), the [serial protocol](../../docs/protocol.md), and the [porting contract](../PORTING.md).
