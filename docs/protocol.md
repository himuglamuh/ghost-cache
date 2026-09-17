# Ghost Modem Serial Protocol v1

The Heltec V3 USB-to-UART serial port uses 115200 8N1. Each frame is COBS-encoded and terminated by `0x00`. Development boot diagnostics may precede a final zero delimiter; framed protocol processing starts at that clean boundary. The decoded little-endian record is:

| Offset | Size | Field |
|---:|---:|---|
| 0 | 2 | magic `GC` |
| 2 | 1 | protocol version (`1`) |
| 3 | 1 | message type |
| 4 | 2 | sequence number |
| 6 | 2 | payload length |
| 8 | N | payload |
| 8+N | 4 | IEEE CRC-32 over all preceding decoded bytes |

A receiver buffers through the next zero delimiter. Invalid COBS, magic, version, length, or CRC discards only that frame. Oversized input is discarded through the next delimiter. Host command responses copy the command sequence; unsolicited RX events use sequence zero.

## Message Types

| Value | Direction | Meaning | Payload |
|---:|---|---|---|
| `0x01` | host to modem | INFO / STATUS request | empty |
| `0x02` | host to modem | TX packet | opaque bytes, 0-255 |
| `0x03` | host to modem | SET CONFIG | configuration below |
| `0x04` | host to modem | SET OLED MODE | one byte: diagnostic=0, minimal=1, off=2 |
| `0x81` | modem to host | RX packet | float32 RSSI, float32 SNR, opaque bytes |
| `0x82` | modem to host | TX result | int16 RadioLib/status code, UTF-8 detail |
| `0x83` | modem to host | modem status | UTF-8 diagnostic text |
| `0x84` | modem to host | error | int16 code, UTF-8 detail |

The 15-byte SET CONFIG payload is float32 MHz, float32 kHz, uint8 spreading factor, uint8 coding-rate denominator, int8 dBm, uint16 preamble symbols, uint8 sync word, and one reserved zero byte. Floating-point values use IEEE-754 binary32. Radio config is intentionally volatile; OLED mode is persisted.
