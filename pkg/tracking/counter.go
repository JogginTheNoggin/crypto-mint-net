package tracking

import (
	"math"
	"time"
)

// slots is how many parts a window is cut into. A windowed counter forgets
// one part at a time, so it is exact to within a slot of its window.
const slots = 8

// Counter counts events over a sliding window in fixed memory: it answers
// "how many in the last stretch of time", never "how many ever". The window
// length is not stored; every call passes it, and one counter must always
// be used with the same window.
type Counter struct {
	Counts [slots]uint64 `json:"c"`
	Epochs [slots]int64  `json:"e"`
}

func epoch(now time.Time, window time.Duration) int64 {
	slot := int64(window) / slots
	if slot <= 0 {
		slot = 1
	}
	return now.UnixNano() / slot
}

// Add counts n events at now.
func (c *Counter) Add(now time.Time, window time.Duration, n uint64) {
	e := epoch(now, window)
	i := int(((e % slots) + slots) % slots)
	if c.Epochs[i] != e {
		c.Epochs[i], c.Counts[i] = e, 0
	}
	c.Counts[i] += n
}

// Sum returns the events counted within the window ending at now.
func (c *Counter) Sum(now time.Time, window time.Duration) uint64 {
	e := epoch(now, window)
	var n uint64
	for i := range c.Counts {
		if d := e - c.Epochs[i]; d >= 0 && d < slots {
			n += c.Counts[i]
		}
	}
	return n
}

// Average is a running average: one number, updated with each new value,
// with recent values weighted more.
type Average struct {
	Value float64 `json:"v"`
	N     uint64  `json:"n"`
}

// averageWeight is how much each new value counts.
const averageWeight = 0.2

// Add takes in one value.
func (a *Average) Add(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if a.N == 0 {
		a.Value = v
	} else {
		a.Value += averageWeight * (v - a.Value)
	}
	a.N++
}
