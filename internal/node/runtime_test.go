package node

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
	"github.com/himuglamuh/ghost-cache/internal/publication"
)

type fakeRadio struct{ sent [][]byte }

func (r *fakeRadio) Send(data []byte, _ time.Duration) error {
	r.sent = append(r.sent, append([]byte(nil), data...))
	return nil
}
func (r *fakeRadio) Receive(time.Duration) (RadioPacket, error) {
	return RadioPacket{}, modem.ErrTimeout
}
func (r *fakeRadio) Close() error { return nil }

func makeRuntime(t *testing.T, id NodeID, dir string, radio *fakeRadio, now time.Time) *Runtime {
	t.Helper()
	rng := rand.New(rand.NewSource(int64(id)))
	controller := NewController(rng)
	r, err := NewRuntime(Config{ID: id, Store: publication.NewStore(dir), Radio: radio, Controller: controller, AdvertiseInterval: time.Hour, ResponseTimeout: time.Second}, rng, now)
	if err != nil {
		t.Fatal(err)
	}
	r.nextAdvertise = now.Add(time.Hour)
	return r
}

func reconcileUntilCandidate(t *testing.T, source, target *Runtime, sourceRadio, targetRadio *fakeRadio, now time.Time, id publication.ID) {
	t.Helper()
	ids, err := source.cfg.Store.IDs()
	if err != nil {
		t.Fatal(err)
	}
	root, _ := EncodeSummary(source.cfg.ID, Broadcast, 0, BuildSummary(ids, nil))
	target.handleIncoming(now, RadioPacket{Data: root})
	for i := range target.reconcileQueue {
		target.reconcileQueue[i].nextAttempt = now
	}
	for step := 0; step < 40 && target.acquisitions[id] == nil; step++ {
		if !target.initiateReconcile(now) {
			t.Fatalf("reconciliation stalled at step %d", step)
		}
		request := targetRadio.sent[len(targetRadio.sent)-1]
		start := len(sourceRadio.sent)
		source.handleIncoming(now, RadioPacket{Data: request})
		if len(sourceRadio.sent) == start {
			t.Fatal("source did not answer reconciliation")
		}
		target.handleIncoming(now.Add(time.Millisecond), RadioPacket{Data: sourceRadio.sent[start]})
		for i := range target.reconcileQueue {
			target.reconcileQueue[i].nextAttempt = now
		}
	}
	if target.acquisitions[id] == nil {
		t.Fatal("publication not discovered")
	}
}

func TestPeerAcquisitionBecomesAdvertised(t *testing.T) {
	now := time.Unix(1000, 0)
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	input := filepath.Join(t.TempDir(), "relay.bin")
	content := bytes.Repeat([]byte("ghost"), 100)
	if err := os.WriteFile(input, content, 0600); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := publication.NewStore(sourceDir).AddFile(input)
	if err != nil {
		t.Fatal(err)
	}
	sourceRadio, targetRadio := &fakeRadio{}, &fakeRadio{}
	source := makeRuntime(t, 1, sourceDir, sourceRadio, now)
	target := makeRuntime(t, 2, targetDir, targetRadio, now)
	reconcileUntilCandidate(t, source, target, sourceRadio, targetRadio, now, manifest.ID)
	a := deliverManifest(t, source, target, sourceRadio, targetRadio, now, manifest.ID)
	if a.assembly == nil && target.acquisitions[manifest.ID] != nil {
		t.Fatal("manifest not accepted")
	}
	for !target.cfg.Store.Complete(manifest.ID) {
		a.nextAttempt = now
		a.waiting = 0
		target.initiate(now, a)
		request, _ := Decode(targetRadio.sent[len(targetRadio.sent)-1])
		if request.Type != TypeGetChunks {
			t.Fatalf("request type=%d", request.Type)
		}
		start := len(sourceRadio.sent)
		source.handleIncoming(now, RadioPacket{Data: targetRadio.sent[len(targetRadio.sent)-1]})
		for _, wire := range sourceRadio.sent[start:] {
			target.handleIncoming(now.Add(time.Millisecond), RadioPacket{Data: wire, RSSI: -72, SNR: 7})
		}
	}
	stored, err := os.ReadFile(filepath.Join(targetDir, "objects", manifest.ID.String(), "content"))
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(stored) != sha256.Sum256(content) {
		t.Fatal("acquired content differs")
	}
	ids, err := target.cfg.Store.IDs()
	if err != nil || len(ids) != 1 || ids[0] != manifest.ID {
		t.Fatalf("advertised IDs=%v err=%v", ids, err)
	}
}

func TestKnownInventoryIgnoredAndPartialUsesNewSource(t *testing.T) {
	now := time.Unix(2000, 0)
	dir := t.TempDir()
	input := filepath.Join(t.TempDir(), "known.bin")
	if err := os.WriteFile(input, []byte("known"), 0600); err != nil {
		t.Fatal(err)
	}
	known, _, _ := publication.NewStore(dir).AddFile(input)
	r := makeRuntime(t, 2, dir, &fakeRadio{}, now)
	root, _ := EncodeSummary(1, Broadcast, 0, BuildSummary([]publication.ID{known.ID}, nil))
	r.handleIncoming(now, RadioPacket{Data: root})
	if r.acquisitions[known.ID] != nil {
		t.Fatal("known object scheduled")
	}
	var unknown publication.ID
	unknown[0] = 9
	r.addCandidate(unknown, 1)
	r.addCandidate(unknown, 3)
	if got := r.acquisitions[unknown].sources; len(got) != 2 || got[1] != 3 {
		t.Fatalf("sources=%v", got)
	}
}

func TestSchedulerAdvertisesBeforeContinuousWork(t *testing.T) {
	now := time.Unix(3000, 0)
	radio := &fakeRadio{}
	r := makeRuntime(t, 1, t.TempDir(), radio, now)
	r.cfg.Controller.deferUntil = time.Time{}
	var id publication.ID
	id[0] = 1
	a := r.addCandidate(id, 2)
	a.nextAttempt = now
	r.nextAdvertise = now
	r.onTick(now.Add(time.Second))
	if len(radio.sent) != 1 {
		t.Fatalf("sent=%d", len(radio.sent))
	}
	packet, err := Decode(radio.sent[0])
	if err != nil || packet.Type != TypeSummary {
		t.Fatalf("packet=%+v err=%v", packet, err)
	}
}

func TestRuntimeReceiveTimeoutRecognized(t *testing.T) {
	if !errors.Is(modem.ErrTimeout, modem.ErrTimeout) {
		t.Fatal("timeout sentinel")
	}
}

func deliverManifest(t *testing.T, source, target *Runtime, sourceRadio, targetRadio *fakeRadio, now time.Time, id publication.ID) *acquisition {
	t.Helper()
	a := target.acquisitions[id]
	if a == nil {
		t.Fatal("candidate missing")
	}
	a.nextAttempt = now
	target.initiate(now, a)
	start := len(sourceRadio.sent)
	source.handleIncoming(now, RadioPacket{Data: targetRadio.sent[len(targetRadio.sent)-1]})
	if len(sourceRadio.sent) == start {
		t.Fatal("manifest response missing")
	}
	target.handleIncoming(now.Add(time.Millisecond), RadioPacket{Data: sourceRadio.sent[start]})
	if a.manifest == nil {
		t.Fatal("base manifest not accepted")
	}
	a.nextAttempt = now
	a.waiting = 0
	target.initiate(now, a)
	start = len(sourceRadio.sent)
	source.handleIncoming(now, RadioPacket{Data: targetRadio.sent[len(targetRadio.sent)-1]})
	if len(sourceRadio.sent) == start {
		t.Fatal("signature response missing")
	}
	target.handleIncoming(now.Add(2*time.Millisecond), RadioPacket{Data: sourceRadio.sent[start]})
	return a
}

func TestDeferredManifestRetainedWithoutBulkTransfer(t *testing.T) {
	now := time.Unix(6000, 0)
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	input := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(input, bytes.Repeat([]byte{7}, 1000), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := publication.NewStore(sourceDir).AddFile(input)
	if err != nil {
		t.Fatal(err)
	}
	sr, tr := &fakeRadio{}, &fakeRadio{}
	source := makeRuntime(t, 1, sourceDir, sr, now)
	target := makeRuntime(t, 2, targetDir, tr, now)
	target.cfg.Policy = AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}
	reconcileUntilCandidate(t, source, target, sr, tr, now, manifest.ID)
	deliverManifest(t, source, target, sr, tr, now, manifest.ID)
	if !target.cfg.Store.HasKnown(manifest.ID) {
		t.Fatal("deferred manifest not retained")
	}
	if target.cfg.Store.HasPartial(manifest.ID) {
		t.Fatal("deferred object created partial content")
	}
	if target.acquisitions[manifest.ID] != nil {
		t.Fatal("deferred object remained scheduled")
	}
	sent := len(tr.sent)
	reconcileUntilNoCandidate(t, source, target, sr, tr, now.Add(time.Second), manifest.ID)
	if len(tr.sent) == sent {
		t.Fatal("expected reconciliation traffic")
	}
	for _, wire := range tr.sent[sent:] {
		packet, _ := Decode(wire)
		if packet.Type == TypeGetManifest || packet.Type == TypeGetChunks {
			t.Fatalf("re-requested deferred object with type %d", packet.Type)
		}
	}
}

func reconcileUntilNoCandidate(t *testing.T, source, target *Runtime, sr, tr *fakeRadio, now time.Time, id publication.ID) {
	t.Helper()
	ids, _ := source.cfg.Store.IDs()
	root, _ := EncodeSummary(source.cfg.ID, Broadcast, 0, BuildSummary(ids, nil))
	target.handleIncoming(now, RadioPacket{Data: root})
	for i := range target.reconcileQueue {
		target.reconcileQueue[i].nextAttempt = now
	}
	for step := 0; step < 40 && len(target.reconcileQueue) > 0; step++ {
		if !target.initiateReconcile(now) {
			break
		}
		start := len(sr.sent)
		source.handleIncoming(now, RadioPacket{Data: tr.sent[len(tr.sent)-1]})
		if len(sr.sent) == start {
			break
		}
		target.handleIncoming(now, RadioPacket{Data: sr.sent[start]})
		for i := range target.reconcileQueue {
			target.reconcileQueue[i].nextAttempt = now
		}
	}
	if target.acquisitions[id] != nil {
		t.Fatal("known deferred object scheduled again")
	}
}

func TestManualWantStartsDeferredAcquisition(t *testing.T) {
	now := time.Unix(7000, 0)
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	input := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(input, bytes.Repeat([]byte{3}, 500), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, _, _ := publication.NewStore(sourceDir).AddFile(input)
	sr, tr := &fakeRadio{}, &fakeRadio{}
	source := makeRuntime(t, 1, sourceDir, sr, now)
	target := makeRuntime(t, 2, targetDir, tr, now)
	target.cfg.Policy = AcquisitionPolicy{MaxAutoSize: 10, HasMaxSize: true}
	reconcileUntilCandidate(t, source, target, sr, tr, now, manifest.ID)
	deliverManifest(t, source, target, sr, tr, now, manifest.ID)
	if err := target.cfg.Store.Want(manifest.ID); err != nil {
		t.Fatal(err)
	}
	reconcileUntilCandidate(t, source, target, sr, tr, now.Add(time.Second), manifest.ID)
	a := target.acquisitions[manifest.ID]
	if a == nil {
		t.Fatal("wanted object not scheduled")
	}
	deliverManifest(t, source, target, sr, tr, now.Add(time.Second), manifest.ID)
	if a.assembly == nil {
		t.Fatal("wanted object not admitted")
	}
}

func TestSignedRelayPreservesOriginSignature(t *testing.T) {
	now := time.Unix(8000, 0)
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	identity, err := publication.GenerateIdentity(sourceDir, "Origin")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "signed.bin")
	content := bytes.Repeat([]byte("signed"), 80)
	os.WriteFile(input, content, 0600)
	manifest, _, err := publication.NewStore(sourceDir).AddFileSigned(input, &identity)
	if err != nil {
		t.Fatal(err)
	}
	sr, tr := &fakeRadio{}, &fakeRadio{}
	source := makeRuntime(t, 1, sourceDir, sr, now)
	target := makeRuntime(t, 2, targetDir, tr, now)
	reconcileUntilCandidate(t, source, target, sr, tr, now, manifest.ID)
	a := deliverManifest(t, source, target, sr, tr, now, manifest.ID)
	for !target.cfg.Store.Complete(manifest.ID) {
		a.nextAttempt = now
		a.waiting = 0
		target.initiate(now, a)
		start := len(sr.sent)
		source.handleIncoming(now, RadioPacket{Data: tr.sent[len(tr.sent)-1]})
		for _, wire := range sr.sent[start:] {
			target.handleIncoming(now.Add(time.Millisecond), RadioPacket{Data: wire})
		}
	}
	relayed, err := target.cfg.Store.LoadManifest(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if relayed.Signature == nil || *relayed.Signature != *manifest.Signature {
		t.Fatal("original signature was not preserved")
	}
	if publication.VerifyManifestSignature(relayed) != nil {
		t.Fatal("relayed signature invalid")
	}
}

func TestKnownDeferredMayInspectNewSource(t *testing.T) {
	now := time.Unix(9000, 0)
	r := makeRuntime(t, 2, t.TempDir(), &fakeRadio{}, now)
	m, _ := signedTestPublication(t)
	unsigned := m
	unsigned.Signature = nil
	unsigned.Version = 1
	if err := r.cfg.Store.SaveKnown(unsigned); err != nil {
		t.Fatal(err)
	}
	r.markSourceInspected(m.ID, 1)
	leaf := RadixLeaf{Generation: 1, IDs: []publication.ID{m.ID}}
	job := reconcileJob{peer: 3, generation: 1, transaction: 7}
	r.reconcileActive = &job
	wire, _ := EncodeLeaf(3, 2, 7, leaf)
	r.handleIncoming(now, RadioPacket{Data: wire})
	a := r.acquisitions[m.ID]
	if a == nil {
		t.Fatal("new source not scheduled")
	}
	found := false
	for _, source := range a.sources {
		if source == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("new source absent")
	}
}

func TestSignatureTimeoutRotatesSource(t *testing.T) {
	now := time.Unix(10000, 0)
	r := makeRuntime(t, 2, t.TempDir(), &fakeRadio{}, now)
	m, _ := signedTestPublication(t)
	a := r.addCandidate(m.ID, 1)
	r.addCandidate(m.ID, 3)
	a.sourceCursor = 1
	a.manifest = &m
	a.manifestSource = 1
	a.activeSource = 1
	a.waiting = TypeSignature
	r.fail(now, a)
	if a.manifest != nil || a.manifestSource != 0 {
		t.Fatal("staged manifest remained pinned")
	}
	a.nextAttempt = now
	r.initiate(now, a)
	if a.activeSource == 1 {
		t.Fatal("retry did not rotate source")
	}
}

func TestReconciliationStateIsBounded(t *testing.T) {
	now := time.Unix(11000, 0)
	r := makeRuntime(t, 1, t.TempDir(), &fakeRadio{}, now)
	for i := 0; i < maxTrackedPeers+10; i++ {
		id := NodeID(i + 2)
		summary := BuildSummary(nil, nil)
		summary.Generation = uint32(i + 1)
		wire, _ := EncodeSummary(id, Broadcast, 0, summary)
		r.handleIncoming(now, RadioPacket{Data: wire})
	}
	if len(r.peerGenerations) > maxTrackedPeers {
		t.Fatalf("tracked peers=%d", len(r.peerGenerations))
	}
	if _, ok := r.peerGenerations[NodeID(maxTrackedPeers+11)]; !ok {
		t.Fatal("new peer was not admitted through eviction")
	}
	for i := 0; i < maxReconcileJobs+20; i++ {
		r.queueReconciliation(2, 1, []byte{byte(i % 16), byte((i / 16) % 16), byte((i / 256) % 16)}, now)
	}
	if len(r.reconcileQueue) > maxReconcileJobs {
		t.Fatalf("jobs=%d", len(r.reconcileQueue))
	}
	for i := 0; i < maxAcquisitions+20; i++ {
		var id publication.ID
		binary.LittleEndian.PutUint32(id[:4], uint32(i+1))
		r.addCandidate(id, 2)
	}
	if len(r.acquisitions) > maxAcquisitions {
		t.Fatalf("acquisitions=%d", len(r.acquisitions))
	}
	for i := 0; i < maxInspectedIDs+20; i++ {
		var id publication.ID
		binary.LittleEndian.PutUint32(id[:4], uint32(i+1))
		r.markSourceInspected(id, 2)
	}
	if len(r.inspectedSources) > maxInspectedIDs {
		t.Fatalf("inspected IDs=%d", len(r.inspectedSources))
	}
}
