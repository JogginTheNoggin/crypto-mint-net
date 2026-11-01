package p2p_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// paceSetup gives a a small request budget over a short window, and a
// heavier ledger whose newest entries carry no layers but the last, so
// checking its work takes more requests than one window allows.
func paceSetup(t *testing.T, pace int) (a, b *tnode) {
	t.Helper()
	tc := tracking.DefaultConfig()
	tc.Budgets[tracking.KindRequest] = 30
	tc.Windows.Budget = 3 * time.Second
	tr, err := tracking.New(tc, "")
	must(t, err)
	a = startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	b = startNode(t, "b", func(c *p2p.Config) {
		c.Limits.HashLimit = 16
		c.RequestTimeout = time.Second
		c.Chase.YieldWait = 200 * time.Millisecond
		c.RequestPace = pace
		c.RequestPaceWindow = 3 * time.Second
	})
	grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	bareRun(t, a, 600)
	return a, b
}

// ORANGE-018: a chase that needs more requests than the sender's request
// budget allows in one window paces itself, keeps what it took in, and
// reaches the heavier ledger (D115).
func TestLongChaseIsPaced(t *testing.T) {
	a, b := paceSetup(t, 20)
	b.connect(a)
	eventuallyFor(t, time.Minute, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
	if !strings.Contains(b.log.String(), "requests to the sender paced") {
		t.Fatal("the chase was never paced")
	}
	if strings.Contains(b.log.String(), "request timed out") {
		t.Fatalf("a request was dropped by the sender's budget:\n%s", b.log)
	}
}

// Without pacing, the same chase runs into the sender's budget: a request
// goes unanswered and the chase ends with nothing (the failure
// TestLongChaseIsPaced guards against).
func TestUnpacedChaseHitsTheBudget(t *testing.T) {
	a, b := paceSetup(t, -1)
	b.connect(a)
	eventuallyFor(t, 30*time.Second, "b's request to time out", func() bool { return strings.Contains(b.log.String(), "request timed out") }, a, b)
}
