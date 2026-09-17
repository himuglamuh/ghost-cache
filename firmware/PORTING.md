# Porting the Ghost Modem Firmware

This document is the standalone implementation contract and porting guide for moving the Ghost Modem firmware to another LoRa-capable board.

The reference implementation is `firmware/heltec-modem/src/main.cpp`. It targets a Heltec WiFi LoRa 32 V3 (ESP32-S3 plus SX1262) using Arduino, RadioLib 7.7.1, and PlatformIO's `heltec_wifi_lora_32_V3` board definition. A port may use another MCU, framework, radio driver, LoRa transceiver, display, or no display at all, but it must preserve the serial protocol and modem behavior described here.

## Prime Directive: This Is a Dumb Modem

**Ghost Cache logic belongs to the host, not the firmware.**

The firmware has exactly two data-plane jobs:

1. Accept an arbitrary byte string from the host and transmit it as one raw LoRa packet.
2. Deliver each received raw LoRa packet to the host, with RSSI and SNR metadata.

Do not parse, generate, route, acknowledge, retry, fragment, reassemble, deduplicate, store, hash, encrypt, sign, prioritize, or interpret Ghost Cache application packets in firmware. In particular, the current `GN` node envelope, legacy `GP` ingestion envelope, manifests, chunks, publication IDs, SHA-256 verification, signatures, durable partial state, and transfer retry policy are host-owned. They are deliberately above the modem boundary. Every radio payload is opaque regardless of its leading bytes.

The host also owns serial command sequence allocation and command timeout policy. Firmware only correlates a response with the sequence supplied by the host. The modem must not autonomously retry LoRa transmissions: doing so can duplicate an application packet. Firmware is not a mesh node, cache, broadcaster, publisher, or application server.

## Required Compatibility Surface

A compatible port must provide:

- A full-duplex serial byte stream at 115200 baud, 8 data bits, no parity, one stop bit (115200 8N1), with no hardware flow control.
- Ghost Modem serial protocol version 1 exactly as specified below.
- Raw LoRa transmit and continuous receive, with radio packets of 0 through 255 bytes where supported by the radio.
- The v1 default and runtime-configurable LoRa parameters.
- Correct recovery from boot text, serial noise, malformed records, and oversized records.
- Asynchronous completion reporting for TX and unsolicited delivery of RX packets.
- Continued serial protocol operation, including INFO, when the radio or optional display fails.

The reference host opens ports with DTR and RTS deasserted, but some USB adapters or operating systems may pulse those lines and reset the board. A port must tolerate a host beginning its handshake while the board resets and boots.

## Serial Framing

The serial stream is a sequence of COBS records. Each on-wire record is:

```text
COBS_ENCODE(decoded_record) 00
```

`00` is the frame delimiter and is not input to COBS decoding. A valid COBS-encoded body contains no zero bytes. The protocol is binary; newline has no framing meaning.

The decoded record is:

| Offset | Size | Encoding | Field |
|---:|---:|---|---|
| 0 | 2 | bytes | Magic: ASCII `G`, `C` (`47 43` hex) |
| 2 | 1 | uint8 | Protocol version, exactly `1` |
| 3 | 1 | uint8 | Message type |
| 4 | 2 | uint16 little-endian | Sequence number |
| 6 | 2 | uint16 little-endian | Payload length `N` |
| 8 | N | bytes | Type-specific payload |
| 8+N | 4 | uint32 little-endian | CRC-32 of decoded bytes `[0, 8+N)` |

The decoded size must be exactly `12 + N`; trailing or missing bytes invalidate the record. The serial payload limit is 512 bytes, so the largest decoded record is 524 bytes. The LoRa TX payload limit is separately 255 bytes. Do not confuse these limits: response metadata and diagnostics can make serial payloads larger than radio payloads.

All multibyte integers and floating-point bit patterns are little-endian. Signed integers use two's-complement representation. Floats are IEEE-754 binary32. Never serialize native structs: padding, alignment, float representation, and host endianness must not leak onto the wire.

### COBS

Use standard Consistent Overhead Byte Stuffing (COBS):

- Encode the entire decoded record, including its CRC.
- Append exactly one zero delimiter after the encoded bytes.
- Do not include the delimiter in COBS decoding.
- A COBS code byte of zero is invalid.
- A code byte may not claim bytes beyond the encoded record.
- A `0xFF` code represents 254 following nonzero bytes and does not imply an inserted zero.
- Empty delimiter-separated records are ignored.

The implementation must handle arbitrary binary payloads, including embedded zero bytes and every byte value from `00` through `FF`.

### CRC-32

Use the IEEE CRC-32 implemented by Go's `hash/crc32.ChecksumIEEE`, equivalently reflected CRC-32/ISO-HDLC:

- Polynomial in reflected form: `0xEDB88320` (normal form `0x04C11DB7`).
- Initial value: `0xFFFFFFFF`.
- Process each byte least-significant bit first.
- Final XOR/complement: `0xFFFFFFFF`.
- No bytes from COBS encoding and no zero delimiter participate.
- Store the resulting uint32 little-endian.

CRC coverage starts at the `G` magic byte and ends at the final payload byte. The CRC field itself is excluded.

## Protocol v1 Message Types

| Value | Name | Direction | Sequence | Payload |
|---:|---|---|---|---|
| `0x01` | INFO_REQUEST | host to modem | chosen by host | Empty |
| `0x02` | TX_REQUEST | host to modem | chosen by host | Opaque LoRa packet, 0-255 bytes |
| `0x03` | SET_CONFIG | host to modem | chosen by host | Exactly 15 bytes, layout below |
| `0x04` | SET_OLED | host to modem | chosen by host | Exactly one mode byte |
| `0x81` | RX_PACKET | modem to host | always zero | RSSI, SNR, then opaque LoRa packet |
| `0x82` | TX_RESULT | modem to host | copied from TX_REQUEST | int16 result code and UTF-8 detail |
| `0x83` | STATUS | modem to host | copied from request | UTF-8 diagnostic text |
| `0x84` | ERROR | modem to host | copied from rejected command, or zero for unsolicited radio error | int16 error code and UTF-8 detail |

Host command responses must copy the request's uint16 sequence exactly, including zero if a host chooses it. Sequence wraparound has no special firmware meaning. An unsolicited RX_PACKET or asynchronous receive error uses sequence zero.

Unknown host message types receive ERROR rather than being silently ignored. A port must not repurpose existing values or change payload layouts while claiming protocol version 1.

### INFO_REQUEST (`0x01`)

Payload length must be zero. A valid request produces one STATUS response with the same sequence. A nonempty payload produces ERROR code `-2` and detail `INFO payload must be empty`.

STATUS payload is plain UTF-8 diagnostic text with no NUL terminator. It is deliberately human-readable rather than a stable machine-parsed struct. The reference status resembles:

```text
firmware: OK; serial: OK; radio: OK RX_READY; oled: OK; protocol: v1 READY; frequency=915.000MHz sf=7 bw=125.0kHz cr=4/5 power=5dBm preamble=8 sync=0x12 oled_mode=diagnostic
```

Report actual current configuration and useful component state. Keep the literal `protocol: v1 READY` so diagnostics clearly identify readiness. INFO must work when the radio or display failed; report the failure in STATUS instead of making the serial control plane unavailable.

### TX_REQUEST (`0x02`)

The complete payload is one opaque LoRa packet. It may be empty and may contain any byte values. Payloads over 255 bytes receive ERROR code `-3`, detail `radio payload exceeds 255 bytes`.

Only one TX may be active. If the radio is unavailable, send ERROR with the current radio/driver error code and detail `radio is unavailable`. If already transmitting, send ERROR code `-4`, detail `radio is busy transmitting`.

For an accepted request:

1. Start transmission without blocking serial processing longer than necessary.
2. Remember that request's sequence.
3. On completion, finish/clear the driver's TX operation.
4. Send one TX_RESULT with the remembered sequence.
5. Return the radio to continuous receive, whether TX succeeded or failed.

TX_RESULT payload is:

| Offset | Size | Encoding | Field |
|---:|---:|---|---|
| 0 | 2 | int16 little-endian | Radio/driver result code; zero means success |
| 2 | remaining | UTF-8 bytes | Human-readable detail, no NUL terminator |

The reference sends code zero and `transmitted` on success. A failure to start TX still produces TX_RESULT (not ERROR), with the driver's code and `startTransmit failed`, then attempts to resume RX. A failure reported when finishing TX produces TX_RESULT with `finishTransmit failed`. The reference enforces a 10-second TX watchdog; timeout sends TX_RESULT code `-8`, detail `TX timeout`, aborts/finishes TX, and resumes RX. Preserve these observable semantics even if the new driver uses different interrupt or polling APIs.

Do not emit a success result merely because bytes were queued to a radio API. Report completion from the radio's TX-done indication or equivalent.

### SET_CONFIG (`0x03`)

The payload is exactly 15 bytes:

| Offset | Size | Encoding | Meaning |
|---:|---:|---|---|
| 0 | 4 | float32 little-endian | Frequency in MHz |
| 4 | 4 | float32 little-endian | Bandwidth in kHz |
| 8 | 1 | uint8 | Spreading factor |
| 9 | 1 | uint8 | Coding-rate denominator; `5` means 4/5, through `8` meaning 4/8 where supported |
| 10 | 1 | int8 | TX power in dBm |
| 11 | 2 | uint16 little-endian | Preamble length in symbols |
| 13 | 1 | uint8 | LoRa sync word |
| 14 | 1 | uint8 | Reserved; must be zero in v1 |

Wrong length or a nonzero reserved byte receives ERROR code `-5`. The reference details are respectively `SET_CONFIG payload must be 15 bytes` and `SET_CONFIG reserved byte must be zero`. A request during TX receives the same busy ERROR code `-4` as TX_REQUEST. An unavailable radio receives ERROR with its current driver code.

The firmware should let its radio driver validate frequency, bandwidth, spreading factor, coding rate, output power, preamble, and sync word against the actual hardware. Configuration is transactional:

1. Save the current config.
2. Stop/stand by the radio and apply all requested fields.
3. Restart continuous receive.
4. On complete success, retain the new config and return STATUS with the request sequence.
5. On any failure, restore the previous config and try to restart RX with it.
6. Return ERROR using the original apply failure code and detail `invalid or failed radio configuration`.
7. If restoration also fails, mark the radio unavailable and report that state on later INFO requests.

Radio configuration is volatile and returns to defaults after reset. Do not persist SET_CONFIG unless a future protocol explicitly requests that behavior.

The v1 defaults are:

| Setting | Default |
|---|---:|
| Frequency | 915.0 MHz |
| Bandwidth | 125.0 kHz |
| Spreading factor | 7 |
| Coding rate | 4/5 (wire denominator `5`) |
| TX power | 5 dBm |
| Preamble | 8 symbols |
| Sync word | `0x12` (private) |

Both communicating radios must use identical frequency, bandwidth, spreading factor, coding rate, preamble, and sync-word settings. The board port is responsible for mapping the generic v1 values to its driver's API without silently changing their meaning.

### SET_OLED (`0x04`)

The payload is exactly one byte:

| Value | Mode | Behavior |
|---:|---|---|
| `0` | diagnostic | Show identity, configuration, readiness, and temporary TX/RX events |
| `1` | minimal | Show a compact identity/readiness or event display |
| `2` | off | Clear and power down/blank the display where possible |

Any other length or value receives ERROR code `-6`, detail `invalid OLED mode`. A valid request applies the mode, persists it across reset where nonvolatile storage is available, and returns STATUS with the request sequence. The reference defaults to diagnostic mode on first boot or when storage is unavailable.

The display is optional and must never be a modem dependency. On a board without a display:

- Keep accepting all three SET_OLED values and return STATUS. This preserves host compatibility.
- Track the selected mode, and persist it if practical, even if it has no visual effect.
- Report the display as absent/unavailable in free-form STATUS text.
- Do not return a new payload shape, redefine SET_OLED, or fail radio initialization because no display exists.

A board with another display technology may render different text or graphics. Display updates must not block serial framing, miss radio IRQs, alter packet bytes, delay TX completion materially, or print unframed serial text after startup. The reference shows a temporary event for about 1.8 seconds and then returns to readiness; that timing and exact visual layout are not protocol requirements.

### RX_PACKET (`0x81`)

Emit one RX_PACKET for each successfully read LoRa packet:

| Offset | Size | Encoding | Field |
|---:|---:|---|---|
| 0 | 4 | float32 little-endian | RSSI in dBm |
| 4 | 4 | float32 little-endian | SNR in dB |
| 8 | remaining | bytes | Opaque LoRa packet, 0-255 bytes |

The serial frame sequence is zero. Read RSSI and SNR for the packet just received, not stale global values. Preserve packet length and bytes exactly. Do not strip, append, inspect, or validate an application envelope.

If the radio indicates a packet longer than 255 bytes, discard it and restart receive. If reading an indicated packet fails, emit ERROR with sequence zero, the driver's int16 error code, and detail `readData failed`, then restart receive. Always re-arm receive after handling a packet or receive error.

### ERROR (`0x84`)

ERROR uses the same payload format as TX_RESULT: int16 little-endian code followed by UTF-8 detail with no terminator. Reference firmware-defined negative codes are:

| Code | Meaning |
|---:|---|
| `-1` | Unsupported protocol version |
| `-2` | INFO payload is not empty |
| `-3` | TX radio payload exceeds 255 bytes |
| `-4` | Radio is busy transmitting |
| `-5` | Invalid SET_CONFIG payload shape/reserved byte |
| `-6` | Invalid OLED mode |
| `-7` | Unknown message type |
| `-8` | TX timeout (reported in TX_RESULT) |

Radio/driver errors are also transported as int16 values. If a new driver has wider or nonnumeric errors, define a stable int16 mapping in the port and put additional information in the UTF-8 detail. Zero is success for TX_RESULT; ERROR should represent failure.

## Command Validation and Malformed Input

Maintain a byte accumulator for nonzero serial bytes. A zero delimiter closes the current record and creates a clean boundary.

Required stream behavior:

- Ignore an empty record (one or more delimiters with no accumulated bytes).
- If an encoded record exceeds the implementation's bounded capacity, clear it, enter discard mode, and ignore every byte through the next zero delimiter.
- At that delimiter, leave discard mode and begin cleanly with subsequent bytes.
- If COBS decoding fails, discard only that record.
- Silently discard records with fewer than 12 decoded bytes or wrong magic.
- For correct magic and at least 12 bytes, an unsupported version produces ERROR `-1` using the sequence at decoded offsets 4-5. This check occurs before v1 length and CRC validation in the reference firmware.
- For version 1, silently discard a payload length over 512, a decoded-size mismatch, or a CRC mismatch.
- Once framing, magic, version, length, and CRC are valid, reject bad command-specific payloads with the defined ERROR response.

Malformed serial data must not reset, wedge, or reinitialize the radio. Never interpret bytes from one delimiter-separated record as part of the next. Use bounded buffers; do not allocate based on an untrusted payload-length field before validating it.

The reference firmware permits up to 527 nonzero encoded bytes in its input accumulator and decodes into a 524-byte raw buffer. A port can calculate the exact standard COBS bound or use a slightly larger fixed input buffer, but it must support every valid 512-byte protocol payload and must bound hostile/noisy input.

Do not send ERROR for bad COBS, bad magic, bad length, or bad CRC. Noise may contain an apparent sequence, and replying to noise can amplify corruption. The one intentional exception is a decodable record with `GC` magic and an unsupported version, as described above.

## Startup, Boot Noise, and Reconnection

Development boot diagnostics may be emitted as readable text at 115200 baud before binary protocol operation. ROM bootloaders may also emit text independently of this firmware. Immediately before entering normal protocol operation, write a single `0x00` byte and flush it. This delimiter causes all preceding boot text/noise to be treated as one malformed record and ensures the next COBS command starts at a clean boundary.

After that boundary, do not print raw logs, stack traces, or text to the protocol serial stream. Runtime diagnostics must be STATUS or ERROR frames. If the platform has a separate debug UART, logs may use it, but it must not be confused with the host protocol endpoint.

The current host synchronization behavior is important compatibility context:

- It opens 115200 8N1 with hardware flow control disabled and asks for DTR/RTS deasserted.
- Opening may nevertheless reset some boards.
- It sends INFO_REQUEST and retries the identical encoded request, with the same sequence, every 400 ms for up to 4 seconds.
- It discards boot text, malformed COBS records, bad CRC records, and empty delimiters while searching for a matching STATUS or ERROR.
- Valid unsolicited frames encountered during synchronization are queued rather than discarded.
- Once synchronized, ordinary commands are not automatically retried because retrying TX_REQUEST could transmit twice.

Therefore boot should reach serial protocol readiness comfortably within four seconds. Commands received too early may be lost; the INFO retry handles that. Repeated INFO requests must be safe and independently answered. Do not treat a duplicate sequence as a duplicate-suppression key: sequence numbers correlate responses but do not provide exactly-once semantics.

On USB disconnect/reconnect, retain no host-session state that is needed to parse a new stream. A physical disconnect may or may not reset the MCU. A delimiter always restores parser alignment. If the serial API exposes connection state, protocol correctness must not depend on it; continue servicing radio and bounded serial input. If reboot occurs, volatile radio configuration returns to defaults while persisted OLED mode remains.

## Radio State Machine

A straightforward compatible state machine has `FAILED`, `RECEIVING`, and `TRANSMITTING` states:

- `FAILED`: serial protocol remains active. INFO reports the radio error. TX and configuration return framed errors as described above. A display failure alone must not cause this state.
- `RECEIVING`: radio is continuously armed. A valid TX_REQUEST starts asynchronous TX and changes state to TRANSMITTING. A receive IRQ reads one packet, emits RX_PACKET or ERROR, and re-arms receive.
- `TRANSMITTING`: reject another TX or SET_CONFIG as busy. A TX-done IRQ or watchdog finishes TX, emits one TX_RESULT, and re-arms receive.

An ISR should only set a volatile/atomic flag or enqueue a minimal event. Do radio I/O, serial writes, COBS, CRC, and display work outside interrupt context. If TX-done and RX-done share an interrupt, interpret it according to the explicit software state. Clear stale IRQ flags/events before starting a new operation.

Keep processing serial input while waiting for radio events. INFO and SET_OLED should remain usable during TX; SET_CONFIG and a second TX are rejected as busy. Ensure timing and counter arithmetic remains correct across the MCU millisecond timer's wraparound.

## Board Adaptation Responsibilities

Before editing protocol code, identify and verify the new board's hardware. Do not infer pin mappings from a visually similar model.

### MCU and PlatformIO

- Add or select the exact PlatformIO board/environment and framework.
- Pin toolchain, platform, and library versions for reproducible builds as the reference does (`espressif32@7.1.3`, RadioLib `7.7.1`, SSD1306 driver `4.6.2`). Versions may differ for the target, but should not float unintentionally.
- Confirm the protocol `Serial` object maps to the USB connector users will open: native USB CDC, USB-to-UART bridge, or a hardware UART as applicable.
- Configure RX buffering large enough for a maximum encoded frame plus delimiter and ensure serial writes handle buffering without truncation.
- Preserve 115200 8N1 and disable hardware flow control.
- Understand reset/boot effects of DTR and RTS, USB enumeration delay, and whether `Serial` truthiness waits for a host. Do not wait indefinitely for a terminal connection.
- Keep RAM usage bounded. The reference uses fixed frame, decoded, TX/RX, and diagnostic buffers.

### LoRa Hardware

- Identify the exact transceiver (for example SX1262, SX1276, SX1280, or another compatible LoRa device), legal frequency variant, and maximum packet length.
- Verify NSS/chip-select, SCK, MISO, MOSI, reset, busy, and DIO/IRQ pins from the board schematic.
- Verify whether SPI is shared and whether explicit bus initialization is required.
- Verify TCXO presence and voltage, crystal versus TCXO operation, regulator mode (DC-DC versus LDO), RF switch control, antenna switch pins, and any board power-enable rail.
- Map RadioLib/module construction and initialization to the actual chip. The Heltec V3 reference uses `SX1262`, TCXO 1.6 V, and the board's SX1262 pins. Copying those values to another board can prevent startup or damage/overstress hardware.
- Confirm the driver accepts v1 units: MHz, kHz, coding-rate denominator, dBm, symbol count, and one-byte sync word.
- Enforce the target's valid frequency and power range through driver errors. Regulatory compliance, permitted frequencies, duty cycle, and antenna requirements remain deployment responsibilities; do not silently clamp requested settings and report success.
- Verify asynchronous receive and transmit completion signaling. Polling is acceptable if it remains responsive and preserves protocol behavior.
- Confirm RSSI and SNR units and convert to float32 dBm/dB if the driver uses fixed-point or another unit.
- Keep an antenna attached for RF testing and avoid testing radios extremely close together, where receiver overload can look like a firmware bug.

If the target radio cannot transmit an arbitrary 255-byte LoRa packet or cannot represent required v1 settings, do not silently advertise full v1 compatibility. Prefer a target that can satisfy the contract. Capability negotiation is not defined in v1.

### Power and Peripherals

- Determine active polarity and startup ordering for radio, display, Vext, RF switch, and peripheral power rails.
- Reset peripherals according to their datasheets and board schematic.
- Do not assume the Heltec V3's active-low Vext on GPIO36, OLED reset GPIO21, I2C pins 17/18, address `0x3C`, or vertical flip applies elsewhere.
- A failed optional display or nonvolatile store must not prevent serial/radio initialization.
- Avoid brownouts or RF instability caused by display or radio power sequencing.

### Nonvolatile Storage

Only OLED mode is persisted by the reference firmware. Radio settings are intentionally volatile. On ESP32 the reference uses Preferences/NVS namespace `ghost-modem`, key `oled`; another platform may use a different storage mechanism. Validate stored values and fall back to diagnostic mode if invalid. Storage errors should be reported diagnostically but must not stop modem operation.

## Optional Capabilities Without Breaking v1

Protocol v1 has no structured capability-negotiation message and no feature-bit field. Compatibility-safe variation is limited:

- Add board, radio, display, or firmware-build facts to the free-form STATUS text.
- Render any useful local display UI while preserving the three existing OLED modes.
- Omit physical display behavior when no display exists while still accepting SET_OLED.
- Use a different internal driver, scheduler, storage implementation, or IRQ mechanism with identical wire behavior.

Do not append undocumented binary fields to RX_PACKET, TX_RESULT, ERROR, or SET_CONFIG. Do not change units, limits, response types, or sequence rules. Do not assign new commands and assume the existing host will use them. A structured capability payload, larger radio packet, new config field, reliable-TX mode, or application-aware behavior requires an explicitly designed later protocol version and corresponding host changes. Continue accepting only version 1 until such a version exists.

STATUS is free-form, but avoid making essential correctness depend on hosts parsing new prose. The current host treats STATUS as printable text and uses its message type and matching sequence for synchronization.

## Reference Implementation Details to Preserve or Deliberately Adapt

The Heltec V3 reference currently:

- Uses Arduino `Serial` at 115200 and terminates boot diagnostics with zero.
- Uses RadioLib 7.7.1 and nonblocking `startTransmit`/`startReceive` operations.
- Uses one DIO1 callback for both RX and TX completion, interpreted by software radio state.
- Limits serial payloads to 512 bytes and radio packets to 255 bytes.
- Truncates internally generated result/error detail so its temporary payload buffer remains bounded; strings have no NUL terminator on wire.
- Initializes serial first, then board/display power and display, then persisted OLED mode, SPI, radio, receive mode, display state, and finally the protocol-ready delimiter.
- Keeps protocol service available if OLED initialization fails or radio initialization fails.
- Uses a 10-second TX watchdog and approximately 1.8-second transient display events.
- Delays the main loop by 1 ms after servicing serial, radio, and display timeout work.

Board-specific pin constants, display driver calls, NVS APIs, TCXO voltage, RadioLib radio class, and startup power sequence must be adapted. Frame encoding, validation order, message values/layouts, state transitions, response correlation, and recovery rules must not drift accidentally.

## Recommended Porting Workflow

1. Read this document and the target board's schematic, MCU documentation, radio datasheet, and PlatformIO board manifest.
2. Record the exact radio chip, SPI pins, reset/busy/IRQ pins, power enables, RF switch controls, oscillator/TCXO requirements, serial endpoint, display wiring, and storage API.
3. Create a separate PlatformIO environment or firmware directory rather than destabilizing an existing proven target unless the task explicitly requires a shared source architecture.
4. Keep protocol constants and codec behavior mechanically identical. Isolate only hardware-dependent serial, radio, display, power, and persistence operations.
5. Implement bounded COBS/CRC framing and parser recovery before integrating the radio.
6. Bring up serial and INFO first, including boot delimiter and radio/display failure reporting.
7. Add radio initialization and continuous RX, then asynchronous TX and its watchdog.
8. Add transactional SET_CONFIG and verify every field over the air.
9. Add optional display and persistence last; neither may affect modem correctness.
10. Build with pinned dependencies and no warnings that indicate type truncation, buffer overflow, unsafe ISR work, or incorrect signed conversions.

## Verification Checklist

Do not call a port complete based only on compilation or an INFO response.

### Codec and Parser Tests

- Encode and decode zero-length payloads and a 255-byte payload containing all byte values `00` through `FE` (or an equivalent arbitrary-binary vector).
- Confirm no encoded body contains `00` and every frame ends in exactly one `00` delimiter.
- Cross-check firmware-produced frames with the Go v1 codec behavior described here.
- Corrupt one payload bit without updating CRC and confirm silent rejection followed by acceptance of the next valid frame.
- Test invalid COBS, wrong magic, wrong version, short record, mismatched length, oversized input, nonzero SET_CONFIG reserved byte, and unknown message type.
- Feed data in single-byte chunks and in multiple concatenated frames to catch assumptions about serial read boundaries.
- Feed empty delimiters and boot text ending in zero; the following INFO must succeed.
- Verify all uint16, int16, uint32, and float32 fields byte-for-byte in little-endian order.

### Startup and Reconnection Tests

- Open the serial device in a way that resets the board and immediately begin INFO requests. STATUS must arrive within the host's four-second startup window.
- Reopen without reset and synchronize again.
- Disconnect/reconnect USB where supported and confirm parser recovery.
- Confirm boot logs occur only before the clean zero boundary and no unframed runtime logs contaminate the stream.
- Confirm repeated identical INFO requests are harmless.

### Radio Tests

- Use two boards with identical settings and suitable antennas, with some physical separation rather than placing antennas directly beside or touching each other.
- Send binary packets in both directions at lengths 0 (if the target radio supports empty LoRa packets), 1, 200, and 255; compare exact bytes.
- Confirm RX_PACKET sequence zero, payload length, RSSI, and SNR.
- Confirm one and only one TX_RESULT per accepted TX, with the request sequence.
- Attempt a second TX and SET_CONFIG during TX and verify busy errors.
- Force or simulate start-TX failure, finish-TX failure, RX read failure, and TX timeout; verify framed results and recovery to receive.
- Apply every SET_CONFIG field, verify STATUS, and communicate over the air. Submit an invalid setting and verify rollback to the previous working config.
- Reset and verify radio defaults return while OLED mode persists where storage is supported.

### Failure Isolation Tests

- Build/run with the display absent or forced to fail; INFO and radio TX/RX must still work, and SET_OLED must remain accepted.
- Force radio initialization failure; serial INFO must report it without reboot loops, memory corruption, or silence.
- Flood malformed and oversized serial records, terminate with zero, and verify immediate recovery on a valid INFO.
- Exercise sustained RX while issuing INFO and OLED commands; no packet bytes or serial frames may be interleaved or corrupted.

## Final Review Questions

Before delivering a port, answer all of these from code and test evidence:

- Is every radio packet still opaque to firmware?
- Is all Ghost Cache publication/reliability/storage logic still exclusively host-owned?
- Are v1 type values, lengths, byte order, float representation, CRC, COBS, delimiter, and sequence behavior exact?
- Can arbitrary binary data, including zeros, make a byte-identical round trip?
- Does malformed input lose only the malformed record and recover at the next zero?
- Does a final zero cleanly separate boot noise from protocol traffic?
- Can INFO synchronize after an open-triggered reset within four seconds?
- Is TX completion asynchronous, correlated once, watchdog-bounded, and followed by receive mode?
- Is configuration transactional and volatile?
- Can display, storage, or radio failure occur without taking down the serial control plane?
- Were all target-board pins, power rails, oscillator settings, RF controls, and driver units verified rather than copied from the Heltec V3?

If any answer is uncertain, the port is not yet protocol-compatible.
