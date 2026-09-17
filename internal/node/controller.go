package node

import (
	"math"
	"math/rand"
	"time"
)

type TrafficClass uint8

const (
	TrafficBroadcast TrafficClass = iota
	TrafficAddressedOther
	TrafficAddressedUs
	TrafficExpected
	TrafficFailure
)

type trafficEvent struct {
	at     time.Time
	weight float64
}

type TrafficStats struct {
	RXPackets, TXPackets      uint64
	Broadcast, AddressedOther uint64
	AddressedUs, Expected     uint64
	Failures, Deferred        uint64
	LastRSSI, LastSNR         float32
}

type Controller struct {
	rng          *rand.Rand
	events       []trafficEvent
	lastActivity time.Time
	deferUntil   time.Time
	stats        TrafficStats
}

func NewController(rng *rand.Rand) *Controller { return &Controller{rng: rng} }

func (c *Controller) Observe(now time.Time, class TrafficClass) {
	weight := 1.0
	switch class {
	case TrafficBroadcast:
		weight = 1.5
		c.stats.Broadcast++
	case TrafficAddressedOther:
		weight = 2.5
		c.stats.AddressedOther++
	case TrafficAddressedUs:
		weight = 2
		c.stats.AddressedUs++
	case TrafficExpected:
		weight = .75
		c.stats.Expected++
	case TrafficFailure:
		weight = 4
		c.stats.Failures++
	}
	if class != TrafficFailure {
		c.stats.RXPackets++
	}
	c.events = append(c.events, trafficEvent{now, weight})
	c.lastActivity = now
	c.prune(now)
}

func (c *Controller) NoteTX(now time.Time)         { c.stats.TXPackets++; c.lastActivity = now }
func (c *Controller) NoteSignal(rssi, snr float32) { c.stats.LastRSSI, c.stats.LastSNR = rssi, snr }

func (c *Controller) Pressure(now time.Time) float64 {
	c.prune(now)
	pressure := 0.0
	for _, event := range c.events {
		pressure += event.weight * math.Exp(-now.Sub(event.at).Seconds()/4)
	}
	return pressure
}

func (c *Controller) BatchSize(now time.Time) int {
	p := c.Pressure(now)
	switch {
	case p >= 10:
		return 1
	case p >= 6:
		return 2
	case p >= 3:
		return 3
	default:
		return MaxChunkBatch
	}
}

func (c *Controller) CanInitiate(now time.Time) bool {
	if now.Before(c.deferUntil) {
		c.stats.Deferred++
		return false
	}
	quiet := 150 * time.Millisecond
	p := c.Pressure(now)
	if p >= 10 {
		quiet = 1500 * time.Millisecond
	} else if p >= 6 {
		quiet = 800 * time.Millisecond
	} else if p >= 3 {
		quiet = 350 * time.Millisecond
	}
	if !c.lastActivity.IsZero() && now.Sub(c.lastActivity) < quiet {
		c.stats.Deferred++
		return false
	}
	return true
}

func (c *Controller) Defer(now time.Time, failed bool) time.Duration {
	p := c.Pressure(now)
	base := 100*time.Millisecond + time.Duration(p*75)*time.Millisecond
	if failed {
		base *= 2
	}
	if base > 4*time.Second {
		base = 4 * time.Second
	}
	jitter := time.Duration(c.rng.Int63n(int64(base) + 1))
	delay := base/2 + jitter
	c.deferUntil = now.Add(delay)
	return delay
}

func (c *Controller) StartupDelay(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(c.rng.Int63n(int64(max) + 1))
}

func (c *Controller) Stats() TrafficStats { return c.stats }

func (c *Controller) prune(now time.Time) {
	cutoff := now.Add(-20 * time.Second)
	i := 0
	for i < len(c.events) && c.events[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		c.events = append(c.events[:0], c.events[i:]...)
	}
}
