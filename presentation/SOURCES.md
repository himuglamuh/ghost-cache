# Presentation Sources

Every numeric external hardware or RF claim used in the deck is listed here. Ghost Cache behavior and software measurements are sourced from this repository and are distinct from manufacturer specifications.

## Heltec WiFi LoRa 32 V3

- **Source:** WiFi LoRa 32 V3 product page
- **Publisher:** Heltec Automation
- **URL:** https://heltec.org/project/wifi-lora-32-v3/
- **Claims used:** reference board combines an ESP32-S3 MCU, SX1262 LoRa transceiver, USB Type-C interface, CP2102 USB-to-serial bridge, and 0.96-inch 128x64 OLED.
- **Slides:** Physical-node and headless-deployment slides.
- **Classification:** Manufacturer board description. The deck does not use Heltec range or whole-board power claims.

## Ghost Cache Implementation Facts

- **Source:** current implementation, tests, documentation, and hardware-verification record in this repository.
- **Claims used:** 200-byte GN packet maximum; 178-byte chunk content; 1,424 chunks; 253,472-byte / 247.5-KiB publication maximum; 128-bit publication ID from SHA-256 prefix; full SHA-256 verification; Ed25519 signed manifests; four-chunk maximum batch; radix leaf threshold 10.
- **Slides:** Content flow, reconciliation, trust, limits, and protocol appendix slides.
- **Classification:** Implementation constants and software behavior verified by code and automated tests.

## Physical Verification

- **Source:** hardware acceptance results supplied by the project owner and recordings committed under `presentation/video/`.
- **Claims used:** physical binary LoRa transport; multi-packet transfer; A-to-B-to-C relay after A disappears; interrupted resume; source switching; receiver-driven acquisition; live `want`; signed relay and trusted original-signer verification; host-applied RF configuration.
- **Slides:** Product overview, both demonstration slides, and verified-capabilities summary.
- **Classification:** Physical verification demonstrated in the included recordings; videos are presentation evidence rather than automated tests.

## Ghost Cache Software Measurements

- **Source:** `internal/node/reconcile_test.go`, `TestOneDifferenceScaleCost`.
- **Claims used:** 1,000 objects with one difference reconciles in 5 packets / 484 RF bytes; 10,000 objects with one difference reconciles in 7 packets / 686 RF bytes; four concrete IDs appear in each measured leaf.
- **Slides:** Reconciliation core and appendix slides.
- **Classification:** Deterministic software measurements, not radio range tests or end-to-end transfer benchmarks.

## Idealized Transfer-Time Model

- **Source:** `presentation/transfer-model.py`, derived from current packet constants, LoRa time-on-air, reply spacing, and a conservative approximation of scheduler behavior.
- **Profile:** SF7, 125 kHz bandwidth, coding rate 4/5, eight-symbol preamble, explicit header, CRC enabled.
- **Protocol assumptions:** 178 content bytes per chunk; 33-byte `GET_CHUNKS` request; 16 bytes of GN/chunk overhead; adaptive one-to-four chunk batches; 75 ms inter-reply gap; current traffic-pressure decay and quiet-time thresholds; 100 ms runtime polling.
- **Results used on slide 4:** 4 KiB ≈ 11.6 seconds; 16 KiB ≈ 46.1 seconds; 64 KiB ≈ 184.2 seconds (3.1 minutes). The resulting conservative rule of thumb is approximately 3 seconds per KiB.
- **Additional model outputs:** 128 KiB ≈ 6.1 minutes; maximum 253,472-byte publication ≈ 11.9 minutes.
- **Exclusions:** discovery advertisement wait, radix reconciliation, manifest/signature inspection, serial/host processing overhead, packet loss, retries, collisions, competing nodes, startup backoff, and RF settings other than the stated profile.
- **Classification:** Conservative clean-link software model, not measured end-to-end throughput or an exact runtime simulation. Runtime timestamping can make a clean transfer slightly faster than the model; loss, contention, discovery, and retries make real transfers longer.
