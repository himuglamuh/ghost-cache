# RF Primer for Ghost Cache Users

You do not need to be a radio engineer to use Ghost Cache. You do need a basic understanding of the settings that make two radios compatible and the practical limits of a shared radio channel.

## LoRa

LoRa is a low-bandwidth packet radio technology designed for long-range-oriented communication. It is useful where Wi-Fi, cellular service, and the Internet are unavailable or unreliable. LoRa sends relatively small packets and trades speed for sensitivity and range potential.

Ghost Cache uses that limited link to move compact documents, bulletins, configuration bundles, and small data files. It is not intended for large media or high-speed file transfer. The current maximum publication size is 253,472 bytes, about 247.5 KiB.

Actual range and reliability depend on antennas, terrain, buildings, interference, radio settings, installation quality, and local regulations. There is no universal range figure.

## LoRa Is Not LoRaWAN

LoRa is the radio modulation. LoRaWAN is a separate network protocol built around gateways, network servers, device enrollment, and session keys.

Ghost Cache uses raw LoRa packets directly between radios. It does not use LoRaWAN, The Things Network, gateways, join keys, network servers, or LoRaWAN device classes. A Ghost Cache node needs a Linux computer and a compatible LoRa modem, not LoRaWAN infrastructure.

## Frequency

Frequency identifies where on the radio spectrum the nodes communicate. Radios must use compatible frequency and modulation settings to hear one another.

Legal unlicensed bands and operating rules differ by country and region. Repository examples use a US 915 MHz evaluation configuration. Do not use 915 MHz merely because it appears in an example; choose settings legal for your location and equipment. Ghost Cache does not enforce regional radio regulations.

Use an antenna designed for the configured frequency band. An antenna is not generically a "LoRa antenna"; its supported frequency range matters.

## Spreading Factor

Ghost Cache accepts `SF5` through `SF12` on the reference modem. Spreading factor controls an important speed-versus-sensitivity tradeoff; SF7 is the first-test setting.

- Lower SF is faster and occupies the channel for less time.
- Higher SF is slower and can improve receiver sensitivity and range potential.
- Higher SF increases airtime, typically increases energy consumed per transmitted packet, and gives other traffic more time to overlap the transmission.

Use SF7 for an initial local evaluation. Do not increase SF automatically when a link has problems; antenna placement and interference may matter more.

## Bandwidth

Bandwidth is the width of spectrum used by a LoRa signal.

- Wider bandwidth generally increases throughput and shortens airtime.
- Narrower bandwidth may improve sensitivity but keeps each packet on the channel longer.

The default example setting is 125 kHz, shown as `bandwidth_khz = 125.0`.

## Coding Rate

LoRa adds error-correction redundancy so a receiver has a better chance of recovering damaged data. Stronger coding adds more redundancy and consumes more airtime.

Ghost Cache configuration stores the denominator: `coding_rate = 5` means LoRa coding rate 4/5. The default example uses 4/5.

## TX Power

Transmit power, measured in dBm, is roughly how strongly the radio transmits. More power is not automatically better:

- it consumes more energy;
- it may exceed legal limits;
- extremely strong nearby signals can be unrepresentative or counterproductive;
- it does not prevent collisions between simultaneous transmissions.

The default example setting is deliberately low at 5 dBm.

## Preamble

The preamble is synchronization material sent before each packet payload. It helps a receiver detect and align with the transmission. The reference value is eight symbols. Leave it unchanged unless you have a specific interoperability reason to alter it.

## Sync Word

The sync word helps radios distinguish packets intended for the same LoRa network from unrelated LoRa traffic using different sync words.

A sync word is not encryption, authentication, a password, or a security boundary. Anyone using compatible radio settings and the same sync word may receive the packets.

## RSSI

Received Signal Strength Indicator, or RSSI, estimates received signal power in dBm. Values are usually negative; a value closer to zero is stronger. For example, -60 dBm is stronger than -100 dBm.

RSSI is diagnostic evidence, not a guarantee. A strong signal can still lose packets because of interference, timing, collisions, or half-duplex operation.

## SNR

Signal-to-Noise Ratio, or SNR, compares the signal with background noise. Higher is generally better. LoRa can decode some signals at negative SNR, which is one reason it is useful for weak links.

RSSI and SNR should be considered together and over multiple packets. One reading does not characterize an entire deployment.

## Half Duplex

The SX1262 radio is half duplex: it cannot transmit and receive at the same time. If two nodes transmit simultaneously, each may miss the other. A third node may also miss one or both packets when transmissions overlap.

Ghost Cache expects packet loss. It uses randomized timing, bounded exchanges, missing-chunk recovery, and repeated reconciliation so occasional loss does not corrupt accepted content. Strong RSSI does not remove half-duplex scheduling constraints.

## Airtime

Airtime is how long a transmission occupies the radio channel. LoRa airtime is scarce because every packet prevents some other useful transmission during that period.

Larger payloads, higher spreading factors, narrower bandwidth, stronger coding, retries, and chatty protocols all increase airtime. Ghost Cache reduces unnecessary traffic by comparing compact inventory summaries, inspecting metadata before transferring content, requesting only missing chunks, and limiting each transfer batch.

## First-Test Settings

For a US 915 MHz local evaluation, the reference settings are:

```text
frequency:       915.0 MHz
spreading factor: 7
bandwidth:       125 kHz
coding rate:     4/5
TX power:        5 dBm
preamble:        8 symbols
sync word:       0x12
```

All nodes must use compatible settings. These values are test defaults, not a universal deployment recommendation.

Continue with [Hardware and firmware](hardware.md), [Quick Start](quickstart.md), and [Configuration](configuration.md).
