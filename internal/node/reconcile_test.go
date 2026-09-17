package node

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

type reconcileCost struct{ Packets, Bytes, LeafIDs int }

func generatedIDs(count int) []publication.ID {
	ids := make([]publication.ID, count)
	for i := range ids {
		sum := sha256.Sum256([]byte(fmt.Sprintf("object-%08d", i)))
		copy(ids[i][:], sum[:16])
	}
	return ids
}

func reconcileSets(t *testing.T, local, remote []publication.ID) ([]publication.ID, reconcileCost) {
	t.Helper()
	cost := reconcileCost{}
	root := BuildSummary(remote, nil)
	wire, _ := EncodeSummary(1, Broadcast, 0, root)
	cost.Packets++
	cost.Bytes += len(wire)
	queue := DifferingChildren(BuildSummary(local, nil), root)
	var missing []publication.ID
	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]
		request, _ := EncodeSummaryRequest(2, 1, 1, root.Generation, prefix)
		cost.Packets++
		cost.Bytes += len(request)
		bucket := FilterPrefix(remote, prefix)
		if len(bucket) <= LeafThreshold {
			leaf := BuildLeaf(remote, prefix)
			response, _ := EncodeLeaf(1, 2, 1, leaf)
			cost.Packets++
			cost.Bytes += len(response)
			cost.LeafIDs += len(leaf.IDs)
			missing = append(missing, MissingIDs(local, leaf)...)
		} else {
			summary := BuildSummary(remote, prefix)
			response, _ := EncodeSummary(1, 2, 1, summary)
			cost.Packets++
			cost.Bytes += len(response)
			queue = append(queue, DifferingChildren(BuildSummary(local, prefix), summary)...)
		}
	}
	return missing, cost
}

func TestRadixReconciliationCases(t *testing.T) {
	cases := []struct {
		name          string
		local, remote []publication.ID
		want          int
	}{{"empty", nil, nil, 0}, {"identical", generatedIDs(100), generatedIDs(100), 0}, {"one-vs-empty", nil, generatedIDs(1), 1}, {"one-difference-10", generatedIDs(9), generatedIDs(10), 1}, {"one-difference-100", generatedIDs(99), generatedIDs(100), 1}, {"multiple", generatedIDs(90), generatedIDs(100), 10}, {"disjoint", generatedIDs(10), generatedIDs(20)[10:], 10}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing, _ := reconcileSets(t, tc.local, tc.remote)
			if len(missing) != tc.want {
				t.Fatalf("missing=%d want=%d", len(missing), tc.want)
			}
		})
	}
}

func TestOneDifferenceScaleCost(t *testing.T) {
	for _, count := range []int{1000, 10000} {
		remote := generatedIDs(count)
		local := append([]publication.ID(nil), remote[:count-1]...)
		missing, cost := reconcileSets(t, local, remote)
		if len(missing) != 1 {
			t.Fatalf("count=%d missing=%d", count, len(missing))
		}
		if cost.LeafIDs > LeafThreshold {
			t.Fatalf("count=%d leaf IDs=%d", count, cost.LeafIDs)
		}
		t.Logf("objects=%d packets=%d bytes=%d leaf_ids=%d", count, cost.Packets, cost.Bytes, cost.LeafIDs)
	}
}

func TestDigestIndependentOfInsertionOrder(t *testing.T) {
	a := generatedIDs(100)
	b := append([]publication.ID(nil), a...)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	if !reflect.DeepEqual(BuildSummary(a, nil), BuildSummary(b, nil)) {
		t.Fatal("summary depends on insertion order")
	}
}

func TestStaleGenerationRejectedByRuntime(t *testing.T) {
	now := testTime()
	r := makeRuntime(t, 2, t.TempDir(), &fakeRadio{}, now)
	job := reconcileJob{peer: 1, generation: 1, prefix: []byte{2}, transaction: 7}
	r.reconcileActive = &job
	summary := BuildSummary(nil, []byte{2})
	summary.Generation = 2
	wire, _ := EncodeSummary(1, 2, 7, summary)
	r.handleIncoming(now, RadioPacket{Data: wire})
	if r.reconcileActive != nil {
		t.Fatal("stale response retained")
	}
}

func TestPeerDisappearsAndReconciliationRetries(t *testing.T) {
	now := testTime()
	r := makeRuntime(t, 2, t.TempDir(), &fakeRadio{}, now)
	r.queueReconciliation(1, 7, []byte{3}, now)
	r.reconcileQueue[0].nextAttempt = now
	if !r.initiateReconcile(now) {
		t.Fatal("request not initiated")
	}
	r.reconcileActive.deadline = now.Add(time.Second)
	r.onTick(now.Add(2 * time.Second))
	if r.reconcileActive != nil || len(r.reconcileQueue) != 1 {
		t.Fatalf("active=%v queued=%d", r.reconcileActive, len(r.reconcileQueue))
	}
	if !r.reconcileQueue[0].nextAttempt.After(now.Add(2 * time.Second)) {
		t.Fatal("retry was not backed off")
	}
}
func testTime() time.Time { return time.Unix(5000, 0) }
