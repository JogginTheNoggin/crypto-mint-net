package p2p_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// Black's C-6 (D129): a sender whose replies keep timing out partway,
// here because b does not pace itself and a drops what is over its
// budget, still brings b to its heavier ledger over many chases. A
// chase that took something in before it stalled does not double the
// backoff, and kept progress lives past the backoff, so it is never lost
// just before its next turn, however short its time to live.
func TestSlowSenderIsReachedOverManyChases(t *testing.T) {
	tc := tracking.DefaultConfig()
	tc.Budgets[tracking.KindRequest] = 30
	tc.Windows.Budget = time.Second
	tr, err := tracking.New(tc, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	b := startNode(t, "b", func(c *p2p.Config) {
		c.Limits.HashLimit = 16
		c.RequestTimeout = 300 * time.Millisecond
		c.RequestPace = -1 // every chase runs until a request is dropped
		c.Chase.YieldWait = 50 * time.Millisecond
		c.Chase.Backoff, c.Chase.MaxBackoff = 200*time.Millisecond, 2*time.Second
		c.ProgressTTL = 300 * time.Millisecond // shorter than the longest backoff
		c.CheckEvery = 100 * time.Millisecond  // so the sweep runs
	})
	grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	bareRun(t, a, 2000)
	b.connect(a)
	eventuallyFor(t, 2*time.Minute, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
	if n := strings.Count(b.log.String(), "result=slowed"); n < 8 {
		t.Fatalf("reached after %d slowed chases, want a branch that needs more than eight:\n%s", n, b.log)
	}
}
