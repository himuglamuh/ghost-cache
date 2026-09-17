#!/usr/bin/env python3
"""Conservative clean-link transfer model for the SF7/BW125 example profile."""

import math

SPREADING_FACTOR = 7
BANDWIDTH_HZ = 125_000
CHUNK_BYTES = 178
REQUEST_PACKET_BYTES = 33
NODE_CHUNK_OVERHEAD_BYTES = 16
REPLY_GAP_SECONDS = 0.075


def lora_time_on_air(payload_bytes: int) -> float:
    symbol_seconds = (2**SPREADING_FACTOR) / BANDWIDTH_HZ
    payload_symbols = 8 + max(
        math.ceil(
            (8 * payload_bytes - 4 * SPREADING_FACTOR + 28 + 16)
            / (4 * SPREADING_FACTOR)
        )
        * 5,
        0,
    )
    return (8 + 4.25 + payload_symbols) * symbol_seconds


def traffic_pressure(events: list[float], now: float) -> float:
    return sum(0.75 * math.exp(-(now - event) / 4) for event in events if now - event <= 20)


def batch_size(pressure: float) -> int:
    if pressure >= 10:
        return 1
    if pressure >= 6:
        return 2
    if pressure >= 3:
        return 3
    return 4


def quiet_seconds(pressure: float) -> float:
    if pressure >= 10:
        return 1.5
    if pressure >= 6:
        return 0.8
    if pressure >= 3:
        return 0.35
    return 0.15


def transfer_seconds(content_bytes: int) -> float:
    total_chunks = math.ceil(content_bytes / CHUNK_BYTES)
    remaining = total_chunks
    events: list[float] = []
    now = 0.0

    while remaining:
        count = min(batch_size(traffic_pressure(events, now)), remaining)
        now += lora_time_on_air(REQUEST_PACKET_BYTES)

        for batch_index in range(count):
            chunk_index = total_chunks - remaining
            data_bytes = min(CHUNK_BYTES, content_bytes - chunk_index * CHUNK_BYTES)
            now += lora_time_on_air(NODE_CHUNK_OVERHEAD_BYTES + data_bytes)
            events.append(now)
            remaining -= 1
            if batch_index < count - 1:
                now += REPLY_GAP_SECONDS

        # Conservatively approximate runtime polling and controller quiet time.
        # Runtime timestamps receive events before some blocking work, so this
        # deliberately rounds upward rather than reproducing exact wall time.
        now = math.ceil((now + 0.1) / 0.1) * 0.1
        while now - events[-1] < quiet_seconds(traffic_pressure(events, now)):
            now += 0.1

    return now


if __name__ == "__main__":
    for kib in (4, 16, 64, 128, 247.53125):
        size = round(kib * 1024)
        seconds = transfer_seconds(size)
        print(f"{kib:g} KiB: {seconds:.1f} s ({seconds / 60:.2f} min)")
