package grind

import "testing"

// GREEN-064: minute_strength is log2(rate × 60), rounded down.
func TestMinuteStrength(t *testing.T) {
	for _, c := range []struct {
		rate float64
		want int
	}{{0, 0}, {0.01, 0}, {1, 5}, {1 << 20, 25}, {27e6, 30}} {
		if got := minuteStrength(c.rate); got != c.want {
			t.Fatalf("minuteStrength(%v) = %d, want %d", c.rate, got, c.want)
		}
	}
}
