package node

import (
	"math/rand"
	"testing"
	"time"
)

func TestStartupDelayDeterministicAndBounded(t *testing.T) {
	a := NewController(rand.New(rand.NewSource(42)))
	b := NewController(rand.New(rand.NewSource(42)))
	for i := 0; i < 20; i++ {
		da, db := a.StartupDelay(5*time.Second), b.StartupDelay(5*time.Second)
		if da != db {
			t.Fatal("not deterministic")
		}
		if da < 0 || da > 5*time.Second {
			t.Fatalf("delay=%s", da)
		}
	}
}

func TestJitterBounded(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		got := Jitter(10*time.Second, rng)
		if got < 8*time.Second || got > 12*time.Second {
			t.Fatalf("jitter=%s", got)
		}
	}
}

func TestPressureDefersAndShrinksBatch(t *testing.T) {
	now := time.Unix(100, 0)
	c := NewController(rand.New(rand.NewSource(1)))
	if c.BatchSize(now) != MaxChunkBatch || !c.CanInitiate(now) {
		t.Fatal("idle controller blocked")
	}
	for i := 0; i < 5; i++ {
		c.Observe(now.Add(time.Duration(i)*time.Millisecond), TrafficAddressedOther)
	}
	busy := now.Add(10 * time.Millisecond)
	if c.BatchSize(busy) != 1 {
		t.Fatalf("batch=%d", c.BatchSize(busy))
	}
	if c.CanInitiate(busy) {
		t.Fatal("initiated during active traffic")
	}
	later := now.Add(30 * time.Second)
	if c.BatchSize(later) != MaxChunkBatch || !c.CanInitiate(later) {
		t.Fatal("controller did not recover")
	}
}

func TestFailureBackoffBounded(t *testing.T) {
	for failures := 0; failures < 20; failures++ {
		got := Backoff(failures)
		if got < time.Second || got > 32*time.Second {
			t.Fatalf("failures=%d backoff=%s", failures, got)
		}
	}
}

func TestDeferralGrowsWithPressure(t *testing.T) {
	now := time.Unix(10, 0)
	idle := NewController(rand.New(rand.NewSource(5)))
	busy := NewController(rand.New(rand.NewSource(5)))
	for i := 0; i < 5; i++ {
		busy.Observe(now, TrafficAddressedOther)
	}
	if busy.Defer(now, false) <= idle.Defer(now, false) {
		t.Fatal("busy deferral did not increase")
	}
}
