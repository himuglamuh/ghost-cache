# Hardware and Firmware

## Reference Modem

Ghost Cache is hardware-verified with the Heltec WiFi LoRa 32 V3:

- ESP32-S3 MCU
- SX1262 LoRa transceiver
- SSD1306 128x64 OLED
- CP2102 USB-to-UART interface on common board revisions

Every node runs the same firmware. Publisher, relay, trust, storage, and replication behavior are host concerns and do not create firmware roles.

## What You Need

A physical node consists of a Linux computer, one compatible USB LoRa modem, one data-capable USB cable, and an antenna designed for the configured frequency band.

Use a 915 MHz antenna for US 915 MHz operation, or an antenna designed for the legal band selected in your region. An antenna is not generically "LoRa"; its supported frequency range matters.

## Safety and Compliance

- Attach an antenna matched to the operating band before powering or transmitting with the modem.
- Do not power the transmitter without an antenna or suitable RF load.
- Give same-room test radios some physical separation rather than placing antennas directly beside or touching each other.
- Configure only frequencies, power, bandwidth, and duty cycle permitted in your jurisdiction.
- Ghost Cache validates hardware-shaped values, not regional regulations.

## Flashing

Identify the board:

```bash
pio device list
ls -l /dev/serial/by-id/
```

Build and upload:

```bash
pio run --project-dir firmware/heltec-modem \
  --target upload \
  --upload-port /dev/ttyUSB0
```

If automatic bootloader entry fails, hold **BOOT**, tap **RESET**, start upload, and release **BOOT** when writing begins.

## Boot Diagnostics

At 115200 baud, a healthy modem reports:

```text
ghost-modem boot
serial: ok (UART0)
vext: enabled
oled: ok
spi: ok
radio: ok
protocol: ready
```

The binary protocol remains available if the optional OLED fails. INFO also reports radio and display errors so host diagnostics do not depend on the screen.

## OLED Modes

```bash
./ghost-radio oled --device /dev/ttyUSB0 diagnostic
./ghost-radio oled --device /dev/ttyUSB0 minimal
./ghost-radio oled --device /dev/ttyUSB0 off
```

OLED mode persists in ESP32 NVS. A display is optional for ports to other hardware.

## Firmware Boundary

The modem provides raw packet TX/RX, RSSI, SNR, configuration, diagnostics, and robust serial framing. It does not understand publications or replication. See the [serial protocol](protocol.md) and [firmware porting contract](../firmware/PORTING.md).

## GPIO Reference

These details are for firmware development and board ports; normal operators do not need them.

| Function | GPIO |
|---|---:|
| SX1262 NSS | 8 |
| SX1262 DIO1 | 14 |
| SX1262 RESET | 12 |
| SX1262 BUSY | 13 |
| SPI SCK | 9 |
| SPI MISO | 11 |
| SPI MOSI | 10 |
| OLED SDA | 17 |
| OLED SCL | 18 |
| OLED RESET | 21 |
| Vext control, active-low | 36 |
