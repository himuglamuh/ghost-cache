---
marp: true
theme: ghost-cache
size: 16:9
paginate: true
html: true
title: Ghost Cache
description: Delay-tolerant content replication over infrastructureless LoRa radio
---

<!-- _class: title -->
<!-- _paginate: false -->

# Ghost Cache

<div class="tagline">LoRa sneakernet without the sneakers</div>
<div class="subtle">Delay-tolerant content replication over infrastructureless radio</div>

<div class="relay-mark">A &nbsp; ~~~~~&gt; &nbsp; B &nbsp; ~~~~~&gt; &nbsp; C</div>

<!--
- Ghost Cache moves compact files directly between small Linux computers over LoRa.
- Core result: a verified receiver becomes a new source.
-->

---

## What if the Internet is not available, but the information still needs to move?

<div class="grid-3">
  <div class="card bad">
    <h3>Internet</h3>
    <p class="big">Fast—when it exists</p>
    <p class="muted">Unavailable, unreliable, damaged, or restricted.</p>
  </div>
  <div class="card neutral">
    <h3>USB handoff</h3>
    <p class="big">Reliable—but manual</p>
    <p class="muted">Every update requires another physical trip.</p>
  </div>
  <div class="card good">
    <h3>LoRa</h3>
    <p class="big">Tiny bandwidth—autonomous</p>
    <p class="muted">Compact information can propagate after deployment.</p>
  </div>
</div>

<p class="caveat">Ghost Cache does not replace broadband. It targets compact information when infrastructure is absent or intermittent.</p>

<!--
- Problem is not “replace the Internet.”
- Target compact documents, bulletins, updates, and small data files.
- USB works, but repeated updates require repeated human movement.
- LoRa buys autonomous propagation at the cost of bandwidth.
- Use cases include isolated infrastructure, field work, disasters, and restricted connectivity.
-->

---

## Add once. Verify. Relay onward.

<div class="physical">
  <div class="node"><div class="host">Linux host A</div><div class="usb">USB</div><div class="radio">LoRa modem</div><div class="action">add file</div></div>
  <div class="waves">~~~&gt;</div>
  <div class="node"><div class="host">Linux host B</div><div class="usb">USB</div><div class="radio">LoRa modem</div><div class="action">acquire + verify</div></div>
  <div class="waves">~~~&gt;</div>
  <div class="node"><div class="host">Linux host C</div><div class="usb">USB</div><div class="radio">LoRa modem</div><div class="action">acquire from B</div></div>
</div>

<p class="lede" style="margin-top: 38px;"><strong>Once a node verifies content, it becomes another source.</strong></p>

<p class="muted">No Wi-Fi · no cellular · no Internet · no central server · original source may disappear</p>

<!--
- A physical node is a Linux host, USB LoRa modem, and band-matched antenna.
- The host runs replication logic; identical modem firmware only moves opaque packets.
- Content—not user messages or routes—is the unit of value.
- B’s verified copy is equivalent to A’s for later replication.
-->

---

## Airtime is the scarce resource

<div class="grid-2" style="margin-top: -8px;">
  <div class="card">
    <h3>Example US RF profile</h3>
    <p class="big">915 MHz <span class="small">US example</span></p>
    <p>SF7 · 125 kHz · CR 4/5 · 5 dBm</p>
    <p class="small">A low-power example—not a universal deployment recommendation.</p>
  </div>
  <div class="card tradeoff-box">
    <h3>The tradeoff</h3>
    <div class="tradeoff-scale">
      <div class="tradeoff-end">faster<span>less airtime</span></div>
      <div class="tradeoff-axis">◀━━━━━━▶</div>
      <div class="tradeoff-end right">more sensitivity<span>more airtime</span></div>
    </div>
    <div class="tradeoff-labels"><span>lower SF</span><span>higher SF</span></div>
  </div>
</div>

<div class="grid-3" style="margin-top: 28px;">
  <div><div class="metric">4 KiB ≈ 12 sec</div><div class="metric-caption">small bulletin</div></div>
  <div><div class="metric">16 KiB ≈ 46 sec</div><div class="metric-caption">demo-sized publication</div></div>
  <div><div class="metric">64 KiB ≈ 3.1 min</div><div class="metric-caption">larger reference pack</div></div>
</div>

<p class="small" style="margin-top: 25px;"><strong>Rule of thumb: ≈ 3 sec/KiB.</strong> Idealized clean-link transfer phase only; discovery, metadata, loss, retries, contention, and slower RF settings add time.</p>

<!--
- LoRa is deliberately slow; every transmission occupies a shared channel.
- Lower SF is faster; higher SF can improve sensitivity but costs airtime.
- Under this RF profile, start with roughly 3 seconds per KiB.
- Conservative model includes 178-byte chunks, request packets, adaptive batches, scheduler quiet-time approximation, and 75 ms reply gaps.
- It excludes discovery, manifest/signature inspection, loss, retries, contention, serial overhead, and other nodes.
- 128 KiB models around 6.1 minutes; the 247.5 KiB maximum around 11.9 minutes before excluded delays.
- No fixed range claim: environment and installation dominate.
-->

---

## The network tracks content, not routes

<p class="small"><strong>Publication</strong> = one file plus immutable metadata: hash, filename, chunk layout, and optional signature.</p>

<div class="pipeline" style="margin-top: 42px;">
  <div class="step">DISCOVER</div>
  <div class="step">INSPECT</div>
  <div class="step">DECIDE</div>
  <div class="step">TRANSFER</div>
  <div class="step">VERIFY</div>
  <div class="step">REPLICATE</div>
</div>

<div class="grid-2">
  <div class="card"><h3>Receiver-driven</h3><p>A source advertises what exists. Each receiver decides what it will acquire.</p></div>
  <div class="card"><h3>Delay-tolerant</h3><p>Partial transfers persist, resume after restart, and can continue from another source.</p></div>
</div>

<!--
- Contrast with chat or routed user-to-user messaging.
- Discovery does not automatically consume bulk airtime.
- Receiver applies local size and trust policy after inspecting metadata.
- Locally added and remotely acquired objects become identical after verification.
- Source disappearance is expected, not exceptional.
-->

---

## Compare differences, not entire inventories

<div class="grid-2">
  <div class="card">
    <h3>Plain-English goal</h3>
    <div class="codebox">Node A: A B C D E<br>Node B: A B C D<br><br>Discover only:<br>“B is missing E.”</div>
  </div>
  <div class="card">
    <h3>Radix-prefix digest reconciliation</h3>
    <p>Compare compact summaries.</p>
    <p>Descend only through differing ID prefixes.</p>
    <p>Send concrete IDs only at small leaves.</p>
  </div>
</div>

<div class="grid-2" style="margin-top: 30px;">
  <div><div class="metric">5 packets · 484 bytes</div><div class="metric-caption">1 difference among 1,000 objects</div></div>
  <div><div class="metric">7 packets · 686 bytes</div><div class="metric-caption">1 difference among 10,000 objects</div></div>
</div>

<p class="measured measure-note">Software measurement: cost follows differing branches—not every publication ID.</p>

<!--
- Naive paging scales with total library size even when peers nearly match.
- First compare 16 compact child summaries, then only mismatched prefixes.
- Concrete IDs appear only when a branch has ten or fewer objects.
- Numbers are deterministic software measurements, not RF range tests.
- Advanced radix details are in the appendix and protocol docs.
-->

---

## Trust belongs to the receiver

<div class="trust-row"><div class="label">SHA-256</div><div>Are these the exact bytes named by the manifest?</div></div>
<div class="trust-row"><div class="label">Ed25519</div><div>Did this signing key sign this publication metadata?</div></div>
<div class="trust-row"><div class="label">Trust policy</div><div>Does this receiver accept that key—or require any signature at all?</div></div>

<div class="grid-2" style="margin-top: 28px;">
  <div class="card"><p><strong>A signs X</strong><br>B relays unchanged<br>C trusts A<br>C verifies A—even when X arrives from B</p></div>
  <div class="card"><p><strong>want = explicit local override</strong><br>May override unsigned / untrusted policy<br>Never overrides cryptographic failure</p></div>
</div>

<p class="caveat" style="margin-top: 24px;">Traffic is plaintext. Signing is not encryption, anonymity, or identity bootstrap.</p>

<!--
- Keep integrity, key authenticity, and local trust distinct.
- Relay path grants no trust; B never re-signs A’s publication.
- “signed” means cryptographically valid, not automatically trusted.
- Trusted key exchange happens outside Ghost Cache.
- Manual want overrides policy, never corrupted bytes or bad signatures.
-->

---

<!-- _class: demo -->
<!-- _paginate: false -->

## Demo 1 · The source disappears

<div class="video-frame">
  <img src="assets/relay-demo-poster.svg" alt="A signs, B acquires, A stops, and C acquires from B while verifying A" style="position:absolute;width:100%;height:100%;object-fit:contain;" />
  <video controls preload="metadata" poster="assets/relay-demo-poster.svg">
    <source src="video/relay-demo.mp4" type="video/mp4" />
  </video>
  <div class="video-fallback">PDF / missing-video fallback: A signs → B acquires → A stops → C acquires from B → C verifies A</div>
</div>

<!--
- Node A originates and signs the publication.
- Node B verifies it and becomes a source.
- A is terminated before C joins.
- C trusts A’s key, not B; hash and original signature are final proof.
-->

---

<!-- _class: demo -->
<!-- _paginate: false -->

## Demo 2 · The receiver is in control

<div class="video-frame">
  <img src="assets/policy-resume-poster.svg" alt="Policy defers content, want approves it live, and restart resumes partial transfer" style="position:absolute;width:100%;height:100%;object-fit:contain;" />
  <video controls preload="metadata" poster="assets/policy-resume-poster.svg">
    <source src="video/policy-resume-demo.mp4" type="video/mp4" />
  </video>
  <div class="video-fallback">PDF / missing-video fallback: defer → library → live want → interrupt → durable resume → verify</div>
</div>

<!--
- Policy blocks bulk transfer but preserves discovered metadata.
- Library makes deferred content visible to a human.
- want is safe while the daemon runs and begins acquisition without restart.
- Stop the receiver after durable partial chunks exist.
- Restart resumes missing chunks rather than beginning at zero.
-->

---

## Headless deployment

<div class="grid-2" style="margin-top: -8px;">
  <div>
    <img src="assets/hardware-node.svg" alt="Linux host connected by USB to a Heltec V3 modem and antenna" style="width:100%; margin-top: 4px;" />
  </div>
  <div class="card">
    <h3>Deployment capabilities</h3>
    <ul>
      <li>RF settings from TOML</li>
      <li>Apply + verify modem settings at startup</li>
      <li>Non-root systemd service</li>
      <li>Stable serial paths</li>
      <li>Volatile operational logs</li>
      <li>Persistent content + recovery state</li>
    </ul>
  </div>
</div>

<p class="caveat" style="margin-top: 28px;">Content and recovery state persist. Operational logs are volatile by default.</p>

<!--
- Field concept is a small headless Linux host such as a Raspberry Pi-class device.
- Host config is authoritative and startup fails on RF readback mismatch.
- Non-root systemd tooling and stable serial-device configuration are included.
- Whole-node power depends on the selected host and radio hardware and must be measured.
-->

---

## What Ghost Cache is—and is not

<div class="two-col-head">
  <div>
    <h3 class="verified">Physically verified</h3>
    <ul class="check">
      <li>Three-node LoRa replication</li>
      <li>Arbitrary binary publications</li>
      <li>Interrupted resume + source switching</li>
      <li>Receiver-driven policy + live want</li>
      <li>Radix reconciliation</li>
      <li>Signed relay + trusted origin verification</li>
      <li>Host-applied RF configuration</li>
    </ul>
  </div>
  <div>
    <h3 class="future">Current boundaries</h3>
    <ul class="limit">
      <li>247.5 KiB publication maximum</li>
      <li>Plaintext RF; no anonymity</li>
      <li>No freshness or revocation semantics</li>
      <li>No automatic retention or GC</li>
      <li>Heltec V3 reference modem</li>
      <li>Range is deployment-specific</li>
      <li>No general routed messaging</li>
    </ul>
  </div>
</div>

<!--
- These boundaries are deliberate, not hidden caveats.
- Hard systems behavior is physically demonstrated: relay, resume, policy, and trust.
- Target compact information, not video or IP-network replacement.
- No confidentiality or anonymity claim.
-->

---

<!-- _class: close -->
<!-- _paginate: false -->

# The sender can disappear.<br>The information doesn't have to.

<div class="subtle">Ghost Cache</div>
<div class="subtle">content-centric · receiver-driven · delay-tolerant</div>

<div class="relay-mark">A &nbsp; ~~~~~&gt; &nbsp; B &nbsp; ~~~~~&gt; &nbsp; C</div>

<!--
- Recap: compact files move without infrastructure, and verified copies become sources.
- The central behavior ran on physical radios, not only in simulation.
- Trust stays with each receiver; the original signer can be offline.
- Appendix slides contain RF and protocol detail.
-->

---

<!-- _class: appendix -->

## RF knobs in one slide

| Term | Practical meaning |
|---|---|
| LoRa | Low-bandwidth, long-range-oriented packet radio modulation |
| LoRaWAN | Gateway/network-server protocol; **not used by Ghost Cache** |
| Frequency | Where radios communicate; must match and be locally legal |
| Spreading factor | Lower is faster; higher can improve sensitivity but costs airtime |
| Bandwidth | Wider is faster; narrower may improve sensitivity |
| Coding rate | Error-correction redundancy; stronger coding costs airtime |
| RSSI / SNR | Received strength / signal relative to noise; diagnostics, not guarantees |

<!--
- Sync word separates compatible LoRa traffic but is not security.
- Radio is half duplex: it cannot receive while transmitting.
- Avoid universal range claims.
-->

---

<!-- _class: appendix -->

## GN v3 message flow

<pre><code>A advertises radix SUMMARY
B requests only differing prefixes
A returns SUMMARY or concrete LEAF IDs

B → GET_MANIFEST      A → MANIFEST
B → GET_SIGNATURE     A → SIGNATURE

B applies local policy

B → GET_CHUNKS        A → up to 4 CHUNK packets
... repeat missing chunks ...
B verifies SHA-256 + signature, then commits</code></pre>

<p class="small">200-byte application packets · one self-initiated addressed exchange at a time · randomized timing and bounded backoff</p>

<!--
- Reconciliation and bulk acquisition alternate for fairness.
- Manifest and signature come from the same source.
- Chunk batches may rotate across sources afterward.
-->

---

<!-- _class: appendix -->

## Publication format

<div class="grid-2">
  <div class="card">
    <h3>Identity + integrity</h3>
    <p><strong>Publication ID</strong><br>first 128 bits of content SHA-256</p>
    <p><strong>Authoritative hash</strong><br>full SHA-256 retained and verified</p>
  </div>
  <div class="card">
    <h3>Transfer + authenticity</h3>
    <p><strong>Chunk content</strong><br>178 bytes per packet</p>
    <p><strong>Optional signature</strong><br>Ed25519 over canonical immutable metadata</p>
  </div>
</div>

<p class="caveat" style="margin-top: 38px;">Maximum today: 1,424 chunks × 178 bytes = 253,472 bytes ≈ 247.5 KiB</p>

<!--
- ID is compact; full hash remains authoritative.
- Signature binds hash, ID, filename, length, chunk geometry, and manifest version.
- Relays preserve signature bytes unchanged.
- Current size limit follows directly from bitmap/chunk geometry.
-->

---

<!-- _class: appendix -->

## Signing and local policy

| Receiver policy | Unsigned | Valid unknown signer | Valid trusted signer | Invalid signature |
|---|---:|---:|---:|---:|
| `permissive` | acquire | acquire | acquire | reject |
| `signed` | defer | acquire | acquire | reject |
| `trusted` | defer | defer | acquire | reject |

<p><strong>want</strong> overrides unsigned/untrusted and size admission—but never invalid signatures or SHA-256 failure.</p>

<p class="small">Key ID = first 128 bits of SHA-256(raw Ed25519 public key). Key ownership is established outside Ghost Cache.</p>

<!--
- “signed” means valid signature, not known human identity.
- Trusted keys are installed through an external trusted channel.
- Trust removal changes future admission; it is not network revocation.
- No automatic key exchange or PKI.
-->

---

<!-- _class: appendix -->

## Reconciliation cost: paging vs radix

| Inventory | Full-ID paging estimate | Radix, one difference |
|---:|---:|---:|
| 1,000 objects | ≈ 91 inventory packets | **5 packets / 484 bytes** |
| 10,000 objects | ≈ 910 inventory packets | **7 packets / 686 bytes** |

<p class="small">Paging estimate: 11 full 128-bit IDs per packet. Radix figures: deterministic Ghost Cache software measurements. Both exclude manifest inspection and content transfer.</p>

<p class="lede" style="margin-top: 42px;">The win appears when libraries are large and mostly equal.</p>

<!--
- Do not compare packet counts as universal transfer latency.
- Radix walks only differing branches.
- 64-bit child digest collision could delay discovery, not accept corrupt content.
- Full SHA-256 still protects publication integrity.
-->
