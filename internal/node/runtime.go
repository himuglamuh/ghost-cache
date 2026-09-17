package node

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
	"github.com/himuglamuh/ghost-cache/internal/publication"
)

type Config struct {
	ID                NodeID
	Store             *publication.Store
	Radio             Radio
	Controller        *Controller
	AdvertiseInterval time.Duration
	StartupBackoff    time.Duration
	ResponseTimeout   time.Duration
	PollInterval      time.Duration
	LogInterval       time.Duration
	Log               io.Writer
	ReplyGap          time.Duration
	Policy            AcquisitionPolicy
}

type acquisition struct {
	id             publication.ID
	sources        []NodeID
	sourceCursor   int
	activeSource   NodeID
	assembly       *publication.Assembly
	failures       int
	nextAttempt    time.Time
	cursor         int
	transaction    uint16
	waiting        Type
	deadline       time.Time
	wanted         map[uint16]bool
	manifest       *publication.Manifest
	manifestSource NodeID
}

type Runtime struct {
	cfg              Config
	acquisitions     map[publication.ID]*acquisition
	order            []publication.ID
	workCursor       int
	nextAdvertise    time.Time
	nextLog          time.Time
	tx               uint16
	reconcileQueue   []reconcileJob
	reconcileActive  *reconcileJob
	preferReconcile  bool
	metrics          Metrics
	peerGenerations  map[NodeID]uint32
	inspectedSources map[publication.ID]map[NodeID]bool
	inspectedOrder   []publication.ID
	completeIDs      []publication.ID
	peerOrder        []NodeID
}

const (
	maxTrackedPeers  = 64
	maxReconcileJobs = 256
	maxAcquisitions  = 256
	maxInspectedIDs  = 256
)

type reconcileJob struct {
	peer        NodeID
	generation  uint32
	prefix      []byte
	failures    int
	nextAttempt time.Time
	transaction uint16
	deadline    time.Time
}
type Metrics struct {
	InventoryObjects, SummariesSent, SummariesReceived, LeavesSent, LeafIDsSent, ManifestRequests, ManifestResponses, ObjectsDiscovered, ObjectsAccepted, ObjectsDeferred uint64
	ReconciliationBytes, ManifestBytes, BulkBytes                                                                                                                         uint64
}

func NewRuntime(cfg Config, rng *rand.Rand, now time.Time) (*Runtime, error) {
	if cfg.ID == 0 || cfg.ID == Broadcast || cfg.Store == nil || cfg.Radio == nil {
		return nil, errors.New("invalid node configuration")
	}
	if cfg.Controller == nil {
		cfg.Controller = NewController(rng)
	}
	if cfg.AdvertiseInterval <= 0 {
		cfg.AdvertiseInterval = 30 * time.Second
	}
	if cfg.ResponseTimeout <= 0 {
		cfg.ResponseTimeout = 3 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	if cfg.LogInterval <= 0 {
		cfg.LogInterval = 30 * time.Second
	}
	if cfg.ReplyGap <= 0 {
		cfg.ReplyGap = 75 * time.Millisecond
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	r := &Runtime{cfg: cfg, acquisitions: make(map[publication.ID]*acquisition), tx: uint16(rng.Intn(1 << 16)), peerGenerations: make(map[NodeID]uint32), inspectedSources: make(map[publication.ID]map[NodeID]bool)}
	if err := r.refreshInventory(); err != nil {
		return nil, err
	}
	partial, err := cfg.Store.PartialIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range partial {
		a, err := cfg.Store.LoadPartial(id)
		if err != nil {
			_ = cfg.Store.DiscardPartial(id)
			r.addCandidate(id, 0)
			continue
		}
		if !cfg.Policy.Accept(a.Manifest, true, cfg.Store.Wanted(id)) {
			fmt.Fprintf(cfg.Log, "partial deferred by signature policy: %s\n", id)
			continue
		}
		r.addCandidate(id, 0)
		r.acquisitions[id].assembly = a
	}
	wanted, err := cfg.Store.WantedIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range wanted {
		r.addCandidate(id, 0)
	}
	startup := cfg.StartupBackoff
	if startup <= 0 {
		startup = min(5*time.Second, cfg.AdvertiseInterval)
	}
	delay := cfg.Controller.StartupDelay(startup)
	cfg.Controller.deferUntil = now.Add(delay)
	fmt.Fprintf(cfg.Log, "startup listen-only backoff: %s\n", delay)
	r.nextAdvertise = now.Add(delay)
	r.nextLog = now.Add(cfg.LogInterval)
	return r, nil
}

func (r *Runtime) Run(stop <-chan struct{}) error {
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		now := time.Now()
		packet, err := r.cfg.Radio.Receive(r.cfg.PollInterval)
		if err == nil {
			r.handleIncoming(now, packet)
			r.onTick(time.Now())
			continue
		}
		if !errors.Is(err, modem.ErrTimeout) {
			return err
		}
		r.onTick(now)
	}
}

func (r *Runtime) handleIncoming(now time.Time, radioPacket RadioPacket) {
	packet, err := Decode(radioPacket.Data)
	if err != nil {
		return
	}
	r.cfg.Controller.NoteSignal(radioPacket.RSSI, radioPacket.SNR)
	expected := r.isExpected(packet)
	switch {
	case expected:
		r.cfg.Controller.Observe(now, TrafficExpected)
	case packet.Destination == Broadcast:
		r.cfg.Controller.Observe(now, TrafficBroadcast)
	case packet.Destination == r.cfg.ID:
		r.cfg.Controller.Observe(now, TrafficAddressedUs)
	default:
		r.cfg.Controller.Observe(now, TrafficAddressedOther)
	}
	if packet.Source == r.cfg.ID {
		return
	}
	if packet.Destination != Broadcast && packet.Destination != r.cfg.ID {
		return
	}
	if packet.Destination == Broadcast {
		if packet.Type == TypeSummary {
			r.handleRootSummary(now, packet)
		}
		return
	}
	switch packet.Type {
	case TypeGetManifest:
		r.serveManifest(now, packet)
	case TypeGetChunks:
		r.serveChunks(now, packet)
	case TypeManifest:
		r.acceptManifest(now, packet)
	case TypeChunk:
		r.acceptChunk(now, packet)
	case TypeGetSignature:
		r.serveSignature(now, packet)
	case TypeSignature:
		r.acceptSignature(now, packet)
	case TypeSummaryRequest:
		r.serveSummary(now, packet)
	case TypeSummary:
		r.acceptSummary(now, packet)
	case TypeLeaf:
		r.acceptLeaf(now, packet)
	}
}

func (r *Runtime) handleRootSummary(now time.Time, packet Packet) {
	remote, err := DecodeSummary(packet)
	if err != nil || len(remote.Prefix) != 0 {
		return
	}
	r.metrics.SummariesReceived++
	local := BuildSummary(r.completeIDs, nil)
	old, knownPeer := r.peerGenerations[packet.Source]
	if !knownPeer {
		if len(r.peerOrder) >= maxTrackedPeers {
			evicted := r.peerOrder[0]
			r.peerOrder = r.peerOrder[1:]
			delete(r.peerGenerations, evicted)
			r.dropReconciliation(evicted)
		}
	} else {
		for i, peer := range r.peerOrder {
			if peer == packet.Source {
				r.peerOrder = append(r.peerOrder[:i], r.peerOrder[i+1:]...)
				break
			}
		}
	}
	r.peerOrder = append(r.peerOrder, packet.Source)
	if !knownPeer || old != remote.Generation {
		r.dropReconciliation(packet.Source)
		r.peerGenerations[packet.Source] = remote.Generation
	}
	for _, prefix := range DifferingChildren(local, remote) {
		r.queueReconciliation(packet.Source, remote.Generation, prefix, now)
	}
}

func (r *Runtime) serveSummary(now time.Time, packet Packet) {
	generation, prefix, err := DecodeSummaryRequest(packet)
	if err != nil {
		return
	}
	if InventoryGeneration(r.completeIDs) != generation {
		return
	}
	bucket := FilterPrefix(r.completeIDs, prefix)
	var wire []byte
	if len(bucket) <= LeafThreshold {
		wire, err = EncodeLeaf(r.cfg.ID, packet.Source, packet.Transaction, BuildLeaf(r.completeIDs, prefix))
		if err == nil {
			r.metrics.LeavesSent++
			r.metrics.LeafIDsSent += uint64(len(bucket))
		}
	} else {
		wire, err = EncodeSummary(r.cfg.ID, packet.Source, packet.Transaction, BuildSummary(r.completeIDs, prefix))
		if err == nil {
			r.metrics.SummariesSent++
		}
	}
	if err == nil && r.send(now, wire) == nil {
		r.metrics.ReconciliationBytes += uint64(len(wire))
	}
}

func (r *Runtime) acceptSummary(now time.Time, packet Packet) {
	job := r.activeReconciliation(packet)
	if job == nil {
		return
	}
	remote, err := DecodeSummary(packet)
	if err != nil || remote.Generation != job.generation || string(remote.Prefix) != string(job.prefix) {
		r.reconcileActive = nil
		return
	}
	r.metrics.SummariesReceived++
	local := BuildSummary(r.completeIDs, job.prefix)
	for _, prefix := range DifferingChildren(local, remote) {
		r.queueReconciliation(job.peer, job.generation, prefix, now)
	}
	r.reconcileActive = nil
}

func (r *Runtime) acceptLeaf(now time.Time, packet Packet) {
	job := r.activeReconciliation(packet)
	if job == nil {
		return
	}
	leaf, err := DecodeLeaf(packet)
	if err != nil || leaf.Generation != job.generation || string(leaf.Prefix) != string(job.prefix) {
		r.reconcileActive = nil
		return
	}
	r.reconcileActive = nil
	for _, id := range MissingIDs(r.completeIDs, leaf) {
		if r.cfg.Store.Complete(id) {
			continue
		}
		if r.cfg.Store.HasKnown(id) {
			if r.cfg.Store.HasPartial(id) || r.cfg.Store.Wanted(id) {
				a := r.addCandidate(id, packet.Source)
				if a == nil {
					continue
				}
				if a.nextAttempt.IsZero() {
					a.nextAttempt = now
				}
			} else if !r.sourceInspected(id, packet.Source) {
				a := r.addCandidate(id, packet.Source)
				if a == nil {
					continue
				}
				if a.nextAttempt.IsZero() {
					a.nextAttempt = now
				}
			}
			continue
		}
		r.metrics.ObjectsDiscovered++
		a := r.addCandidate(id, packet.Source)
		if a == nil {
			continue
		}
		if a.nextAttempt.IsZero() {
			a.nextAttempt = now.Add(time.Duration(r.cfg.Controller.rng.Int63n(int64(500*time.Millisecond) + 1)))
		}
		fmt.Fprintf(r.cfg.Log, "publication discovered: %s source=%08x\n", id, uint32(packet.Source))
	}
}

func (r *Runtime) queueReconciliation(peer NodeID, generation uint32, prefix []byte, now time.Time) {
	if len(r.reconcileQueue) >= maxReconcileJobs {
		return
	}
	if r.reconcileActive != nil && r.reconcileActive.peer == peer && string(r.reconcileActive.prefix) == string(prefix) {
		return
	}
	for _, job := range r.reconcileQueue {
		if job.peer == peer && job.generation == generation && string(job.prefix) == string(prefix) {
			return
		}
	}
	jitter := time.Duration(r.cfg.Controller.rng.Int63n(int64(750*time.Millisecond) + 1))
	r.reconcileQueue = append(r.reconcileQueue, reconcileJob{peer: peer, generation: generation, prefix: append([]byte(nil), prefix...), nextAttempt: now.Add(jitter)})
}
func (r *Runtime) initiateReconcile(now time.Time) bool {
	if r.reconcileActive != nil || r.hasOutstandingAcquisition() {
		return false
	}
	for i := 0; i < len(r.reconcileQueue); i++ {
		job := r.reconcileQueue[i]
		if now.Before(job.nextAttempt) {
			continue
		}
		r.reconcileQueue = append(r.reconcileQueue[:i], r.reconcileQueue[i+1:]...)
		r.tx++
		job.transaction = r.tx
		wire, err := EncodeSummaryRequest(r.cfg.ID, job.peer, job.transaction, job.generation, job.prefix)
		if err != nil {
			return false
		}
		if r.send(now, wire) != nil {
			job.failures++
			base := Backoff(job.failures)
			job.nextAttempt = now.Add(base + time.Duration(r.cfg.Controller.rng.Int63n(int64(base/2)+1)))
			r.reconcileQueue = append(r.reconcileQueue, job)
			return true
		}
		job.deadline = time.Now().Add(r.cfg.ResponseTimeout)
		r.reconcileActive = &job
		r.metrics.ReconciliationBytes += uint64(len(wire))
		return true
	}
	return false
}

func (r *Runtime) hasOutstandingAcquisition() bool {
	for _, a := range r.acquisitions {
		if a.waiting != 0 {
			return true
		}
	}
	return false
}
func (r *Runtime) activeReconciliation(packet Packet) *reconcileJob {
	job := r.reconcileActive
	if job != nil && job.peer == packet.Source && job.transaction == packet.Transaction && packet.Destination == r.cfg.ID {
		return job
	}
	return nil
}
func (r *Runtime) dropReconciliation(peer NodeID) {
	if r.reconcileActive != nil && r.reconcileActive.peer == peer {
		r.reconcileActive = nil
	}
	out := r.reconcileQueue[:0]
	for _, job := range r.reconcileQueue {
		if job.peer != peer {
			out = append(out, job)
		}
	}
	r.reconcileQueue = out
}

func (r *Runtime) addCandidate(id publication.ID, source NodeID) *acquisition {
	a := r.acquisitions[id]
	if a == nil {
		if source != 0 && len(r.acquisitions) >= maxAcquisitions {
			return nil
		}
		a = &acquisition{id: id}
		r.acquisitions[id] = a
		r.order = append(r.order, id)
	}
	if source != 0 && !slices.Contains(a.sources, source) {
		const maxSourcesPerAcquisition = 8
		if len(a.sources) < maxSourcesPerAcquisition {
			a.sources = append(a.sources, source)
		}
	}
	return a
}

func (r *Runtime) serveManifest(now time.Time, packet Packet) {
	id, err := DecodeIDBody(packet)
	if err != nil || !r.cfg.Store.Complete(id) {
		return
	}
	manifest, err := r.cfg.Store.LoadManifest(id)
	if err != nil {
		return
	}
	wire, err := EncodeManifest(r.cfg.ID, packet.Source, packet.Transaction, manifest)
	if err == nil {
		r.metrics.ManifestResponses++
		r.metrics.ManifestBytes += uint64(len(wire))
		r.send(now, wire)
	}
}

func (r *Runtime) serveSignature(now time.Time, packet Packet) {
	id, err := DecodeIDBody(packet)
	if err != nil || !r.cfg.Store.Complete(id) {
		return
	}
	manifest, err := r.cfg.Store.LoadManifest(id)
	if err != nil {
		return
	}
	wire, err := EncodeSignature(r.cfg.ID, packet.Source, packet.Transaction, manifest.Signature)
	if err == nil {
		r.metrics.ManifestResponses++
		r.metrics.ManifestBytes += uint64(len(wire))
		r.send(now, wire)
	}
}

func (r *Runtime) serveChunks(now time.Time, packet Packet) {
	req, err := DecodeChunkRequest(packet)
	if err != nil || !r.cfg.Store.Complete(req.ID) {
		return
	}
	// The requester already selected an adaptive bounded quantum. Serving the
	// complete grant avoids making it wait for chunks we silently withheld.
	remaining := MaxChunkBatch
	for bit := 0; bit < 8 && remaining > 0; bit++ {
		if req.Bitmap&(1<<bit) == 0 {
			continue
		}
		index := req.Base + uint16(bit)
		data, err := r.cfg.Store.ReadChunk(req.ID, index)
		if err != nil {
			continue
		}
		wire, err := EncodeChunk(r.cfg.ID, packet.Source, packet.Transaction, index, data)
		if err == nil && r.send(now, wire) == nil {
			r.metrics.BulkBytes += uint64(len(wire))
			remaining--
			if remaining > 0 {
				time.Sleep(r.cfg.ReplyGap)
			}
		}
	}
}

func (r *Runtime) acceptManifest(now time.Time, packet Packet) {
	a := r.active(packet)
	if a == nil || a.waiting != TypeManifest {
		return
	}
	manifest, err := DecodeManifest(packet, a.id)
	if err != nil || manifest.ID != a.id {
		r.fail(now, a)
		return
	}
	a.manifest = &manifest
	a.manifestSource = packet.Source
	a.waiting = 0
	a.activeSource = 0
	a.nextAttempt = now.Add(100 * time.Millisecond)
}

func (r *Runtime) acceptSignature(now time.Time, packet Packet) {
	a := r.active(packet)
	if a == nil || a.waiting != TypeSignature || a.manifest == nil {
		return
	}
	signature, err := DecodeSignature(packet)
	if err != nil {
		r.fail(now, a)
		return
	}
	manifest := *a.manifest
	r.markSourceInspected(manifest.ID, packet.Source)
	manifest.Signature = signature
	if signature != nil {
		manifest.Version = publication.SignedManifestVersion
	} else {
		manifest.Version = 1
	}
	if signature != nil && publication.VerifyManifestSignature(manifest) != nil {
		fmt.Fprintf(r.cfg.Log, "publication rejected: invalid signature %s\n", manifest.ID)
		delete(r.acquisitions, a.id)
		r.clearInspected(a.id)
		r.removeOrder(a.id)
		return
	}
	if err := r.cfg.Store.SaveKnown(manifest); err != nil {
		r.fail(now, a)
		return
	}
	partial := r.cfg.Store.HasPartial(manifest.ID)
	wanted := r.cfg.Store.Wanted(manifest.ID)
	if !r.cfg.Policy.Accept(manifest, partial, wanted) {
		r.metrics.ObjectsDeferred++
		fmt.Fprintf(r.cfg.Log, "publication deferred by policy: %s size=%d signature=%s\n", manifest.ID, manifest.Length, signatureStatus(manifest, r.cfg.Policy.Trust))
		delete(r.acquisitions, a.id)
		r.removeOrder(a.id)
		return
	}
	r.metrics.ObjectsAccepted++
	assembly, err := r.cfg.Store.Begin(manifest)
	if err != nil {
		r.fail(now, a)
		return
	}
	a.assembly = assembly
	a.manifest = nil
	a.manifestSource = 0
	if assembly.Complete() {
		path, err := r.cfg.Store.Commit(assembly)
		if errors.Is(err, publication.ErrHashMismatch) {
			_ = r.cfg.Store.DiscardPartial(a.id)
			a.assembly = nil
			r.fail(now, a)
			return
		}
		if err != nil {
			r.fail(now, a)
			return
		}
		fmt.Fprintf(r.cfg.Log, "publication verified and committed: %s path=%s\n", a.id, path)
		if err := r.refreshInventory(); err != nil {
			fmt.Fprintf(r.cfg.Log, "inventory refresh failed: %v\n", err)
		}
		delete(r.acquisitions, a.id)
		r.clearInspected(a.id)
		r.removeOrder(a.id)
		return
	}
	a.waiting = 0
	a.failures = 0
	a.nextAttempt = now.Add(100 * time.Millisecond)
	fmt.Fprintf(r.cfg.Log, "manifest received: %s chunks=%d\n", a.id, manifest.ChunkCount)
}

func (r *Runtime) sourceInspected(id publication.ID, source NodeID) bool {
	return r.inspectedSources[id] != nil && r.inspectedSources[id][source]
}
func (r *Runtime) markSourceInspected(id publication.ID, source NodeID) {
	if r.inspectedSources[id] == nil {
		if len(r.inspectedOrder) >= maxInspectedIDs {
			evicted := r.inspectedOrder[0]
			r.inspectedOrder = r.inspectedOrder[1:]
			delete(r.inspectedSources, evicted)
		}
		r.inspectedSources[id] = make(map[NodeID]bool)
		r.inspectedOrder = append(r.inspectedOrder, id)
	}
	const maxInspectedSources = 8
	if len(r.inspectedSources[id]) < maxInspectedSources {
		r.inspectedSources[id][source] = true
	}
}

func (r *Runtime) clearInspected(id publication.ID) {
	delete(r.inspectedSources, id)
	for i, candidate := range r.inspectedOrder {
		if candidate == id {
			r.inspectedOrder = append(r.inspectedOrder[:i], r.inspectedOrder[i+1:]...)
			return
		}
	}
}

func signatureStatus(manifest publication.Manifest, trust *publication.TrustStore) string {
	if manifest.Signature == nil {
		return "unsigned"
	}
	if publication.VerifyManifestSignature(manifest) != nil {
		return "invalid"
	}
	id := publication.KeyID(manifest.Signature.PublicKey[:])
	if trust != nil && trust.Trusted(manifest.Signature.PublicKey[:]) {
		return "trusted:" + id
	}
	return "signed:" + id
}

func (r *Runtime) acceptChunk(now time.Time, packet Packet) {
	a := r.active(packet)
	if a == nil || a.waiting != TypeChunk {
		return
	}
	index, data, err := DecodeChunk(packet)
	if err != nil || !a.wanted[index] {
		return
	}
	if err := r.cfg.Store.Add(a.assembly, index, data); err != nil {
		return
	}
	delete(a.wanted, index)
	fmt.Fprintf(r.cfg.Log, "received %s chunk %d/%d rssi activity\n", a.id, index+1, a.assembly.Manifest.ChunkCount)
	if a.assembly.Complete() {
		path, err := r.cfg.Store.Commit(a.assembly)
		if errors.Is(err, publication.ErrHashMismatch) {
			restarted, restartErr := r.cfg.Store.Restart(a.assembly.Manifest)
			if restartErr == nil {
				a.assembly = restarted
			}
			r.fail(now, a)
			return
		}
		if err != nil {
			r.fail(now, a)
			return
		}
		fmt.Fprintf(r.cfg.Log, "publication verified and committed: %s path=%s\n", a.id, path)
		if err := r.refreshInventory(); err != nil {
			fmt.Fprintf(r.cfg.Log, "inventory refresh failed: %v\n", err)
		}
		delete(r.acquisitions, a.id)
		r.clearInspected(a.id)
		r.removeOrder(a.id)
		return
	}
	if len(a.wanted) == 0 {
		a.waiting = 0
		a.failures = 0
		a.nextAttempt = now.Add(100 * time.Millisecond)
	}
}

func (r *Runtime) onTick(now time.Time) {
	for _, a := range r.acquisitions {
		if a.waiting != 0 && now.After(a.deadline) {
			r.cfg.Controller.Observe(now, TrafficFailure)
			r.fail(now, a)
		}
	}
	if r.reconcileActive != nil && now.After(r.reconcileActive.deadline) {
		job := r.reconcileActive
		r.reconcileActive = nil
		job.failures++
		base := Backoff(job.failures)
		job.nextAttempt = now.Add(base + time.Duration(r.cfg.Controller.rng.Int63n(int64(base/2)+1)))
		if len(r.reconcileQueue) < maxReconcileJobs {
			r.reconcileQueue = append(r.reconcileQueue, *job)
		}
		r.cfg.Controller.Observe(now, TrafficFailure)
	}
	if now.After(r.nextLog) {
		s := r.cfg.Controller.Stats()
		r.metrics.InventoryObjects = uint64(len(r.completeIDs))
		fmt.Fprintf(r.cfg.Log, "traffic pressure=%.2f rx=%d tx=%d other=%d failures=%d deferred=%d batch=%d last_rssi=%.1f last_snr=%.1f inventory=%d summaries_tx=%d summaries_rx=%d leaves_tx=%d leaf_ids=%d manifests_req=%d manifests_rsp=%d discovered=%d accepted=%d policy_deferred=%d reconcile_bytes=%d manifest_bytes=%d bulk_bytes=%d\n", r.cfg.Controller.Pressure(now), s.RXPackets, s.TXPackets, s.AddressedOther, s.Failures, s.Deferred, r.cfg.Controller.BatchSize(now), s.LastRSSI, s.LastSNR, len(r.completeIDs), r.metrics.SummariesSent, r.metrics.SummariesReceived, r.metrics.LeavesSent, r.metrics.LeafIDsSent, r.metrics.ManifestRequests, r.metrics.ManifestResponses, r.metrics.ObjectsDiscovered, r.metrics.ObjectsAccepted, r.metrics.ObjectsDeferred, r.metrics.ReconciliationBytes, r.metrics.ManifestBytes, r.metrics.BulkBytes)
		r.nextLog = now.Add(r.cfg.LogInterval)
	}
	if !r.cfg.Controller.CanInitiate(now) {
		return
	}
	if r.reconcileActive != nil || r.hasOutstandingAcquisition() {
		return
	}
	if !now.Before(r.nextAdvertise) {
		r.advertise(now)
		return
	}
	if r.preferReconcile && r.initiateReconcile(now) {
		r.preferReconcile = false
		return
	}
	if a := r.nextWork(now); a != nil {
		r.initiate(now, a)
		r.preferReconcile = true
		return
	}
	if r.initiateReconcile(now) {
		r.preferReconcile = false
	}
}

func (r *Runtime) nextWork(now time.Time) *acquisition {
	if len(r.order) == 0 {
		return nil
	}
	for offset := 0; offset < len(r.order); offset++ {
		i := (r.workCursor + offset) % len(r.order)
		a := r.acquisitions[r.order[i]]
		if a != nil && a.waiting == 0 && len(a.sources) > 0 && !now.Before(a.nextAttempt) {
			r.workCursor = (i + 1) % len(r.order)
			return a
		}
	}
	return nil
}

func (r *Runtime) initiate(now time.Time, a *acquisition) {
	source := a.sources[a.sourceCursor%len(a.sources)]
	if a.manifest != nil && a.manifestSource != 0 {
		source = a.manifestSource
	} else {
		a.sourceCursor = (a.sourceCursor + 1) % len(a.sources)
	}
	a.activeSource = source
	r.tx++
	a.transaction = r.tx
	if a.assembly == nil {
		if a.manifest != nil {
			wire, _ := EncodeSignatureRequest(r.cfg.ID, source, a.transaction, a.id)
			if r.send(now, wire) != nil {
				r.fail(now, a)
				return
			}
			r.metrics.ManifestRequests++
			r.metrics.ManifestBytes += uint64(len(wire))
			a.waiting = TypeSignature
			a.deadline = time.Now().Add(r.cfg.ResponseTimeout)
			return
		}
		wire, _ := EncodeIDRequest(TypeGetManifest, r.cfg.ID, source, a.transaction, a.id)
		if r.send(now, wire) != nil {
			r.fail(now, a)
			return
		}
		r.metrics.ManifestRequests++
		r.metrics.ManifestBytes += uint64(len(wire))
		a.waiting = TypeManifest
		a.deadline = time.Now().Add(r.cfg.ResponseTimeout)
		return
	}
	base, bitmap, indexes := MissingBatch(a.assembly.Received(), a.cursor)
	if len(indexes) == 0 {
		return
	}
	limit := r.cfg.Controller.BatchSize(now)
	bitmap = 0
	a.wanted = make(map[uint16]bool)
	for _, index := range indexes[:min(limit, len(indexes))] {
		bitmap |= 1 << uint(index-uint16(base))
		a.wanted[index] = true
		a.cursor = int(index) + 1
	}
	wire, _ := EncodeChunkRequest(r.cfg.ID, source, a.transaction, ChunkRequest{ID: a.id, Base: base, Bitmap: bitmap})
	if r.send(now, wire) != nil {
		r.fail(now, a)
		return
	}
	r.metrics.BulkBytes += uint64(len(wire))
	a.waiting = TypeChunk
	a.deadline = time.Now().Add(r.cfg.ResponseTimeout)
}

func (r *Runtime) advertise(now time.Time) {
	if err := r.refreshInventory(); err != nil {
		fmt.Fprintf(r.cfg.Log, "inventory refresh failed: %v\n", err)
		r.nextAdvertise = now.Add(Jitter(r.cfg.AdvertiseInterval, r.cfg.Controller.rng))
		return
	}
	summary := BuildSummary(r.completeIDs, nil)
	wire, _ := EncodeSummary(r.cfg.ID, Broadcast, 0, summary)
	if r.send(now, wire) == nil {
		r.metrics.SummariesSent++
		r.metrics.ReconciliationBytes += uint64(len(wire))
		fmt.Fprintf(r.cfg.Log, "inventory root summary objects=%d generation=%08x bytes=%d\n", len(r.completeIDs), summary.Generation, len(wire))
	}
	r.nextAdvertise = now.Add(Jitter(r.cfg.AdvertiseInterval, r.cfg.Controller.rng))
}

func (r *Runtime) refreshInventory() error {
	ids, err := r.cfg.Store.IDs()
	if err != nil {
		return err
	}
	r.completeIDs = ids
	return nil
}
func (r *Runtime) send(now time.Time, data []byte) error {
	err := r.cfg.Radio.Send(data, 10*time.Second)
	if err != nil {
		r.cfg.Controller.Observe(now, TrafficFailure)
		r.cfg.Controller.Defer(now, true)
		return err
	}
	r.cfg.Controller.NoteTX(now)
	return nil
}
func (r *Runtime) fail(now time.Time, a *acquisition) {
	if a.waiting == TypeSignature {
		a.manifest = nil
		a.manifestSource = 0
	}
	a.waiting = 0
	a.wanted = nil
	a.activeSource = 0
	a.failures++
	base := Backoff(a.failures)
	delay := base + time.Duration(r.cfg.Controller.rng.Int63n(int64(base/2)+1))
	a.nextAttempt = now.Add(delay)
	fmt.Fprintf(r.cfg.Log, "acquisition deferred: %s failures=%d retry_in=%s\n", a.id, a.failures, delay)
	r.cfg.Controller.Defer(now, true)
}
func (r *Runtime) active(packet Packet) *acquisition {
	for _, a := range r.acquisitions {
		if a.transaction == packet.Transaction && a.activeSource == packet.Source && packet.Destination == r.cfg.ID {
			return a
		}
	}
	return nil
}
func (r *Runtime) isExpected(packet Packet) bool {
	return r.active(packet) != nil || r.activeReconciliation(packet) != nil
}
func (r *Runtime) removeOrder(id publication.ID) {
	for i, v := range r.order {
		if v == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			if r.workCursor >= len(r.order) {
				r.workCursor = 0
			}
			return
		}
	}
}
