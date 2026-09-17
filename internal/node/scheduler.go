package node

import (
	"math/rand"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

func Jitter(interval time.Duration, rng *rand.Rand) time.Duration {
	if interval <= 0 {
		return 0
	}
	spread := interval / 5
	return interval - spread + time.Duration(rng.Int63n(int64(2*spread)+1))
}

func MissingBatch(received []bool, start int) (base uint16, bitmap byte, indexes []uint16) {
	if len(received) == 0 {
		return 0, 0, nil
	}
	for offset := 0; offset < len(received); offset++ {
		i := (start + offset) % len(received)
		if received[i] {
			continue
		}
		baseInt := (i / 8) * 8
		for bit := 0; bit < 8 && baseInt+bit < len(received) && len(indexes) < MaxChunkBatch; bit++ {
			idx := baseInt + bit
			if !received[idx] {
				bitmap |= 1 << bit
				indexes = append(indexes, uint16(idx))
			}
		}
		return uint16(baseInt), bitmap, indexes
	}
	return 0, 0, nil
}

type Candidate struct {
	ID          publication.ID
	Sources     []NodeID
	SourceIndex int
	Failures    int
	NextAttempt time.Time
}

func (c *Candidate) Source() NodeID {
	if len(c.Sources) == 0 {
		return 0
	}
	source := c.Sources[c.SourceIndex%len(c.Sources)]
	c.SourceIndex = (c.SourceIndex + 1) % len(c.Sources)
	return source
}
func Backoff(failures int) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures > 5 {
		failures = 5
	}
	return time.Second * time.Duration(1<<failures)
}
