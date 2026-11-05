package p2p

import (
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// ProgressHeld returns, over the chases that yielded and kept their
// progress, how many hashes they have been listed and how many they hold.
func (h *Host) ProgressHeld() (listed, held int) {
	h.progMu.Lock()
	defer h.progMu.Unlock()
	for _, st := range h.prog {
		listed += st.listed
		held += len(st.pending) + len(st.marks) + len(st.branch)
	}
	return listed, held
}

// SetDiskSpace makes the parts' disk space read as f reads it, and
// returns the call that puts statDisk back. Set it before a node starts
// and put it back after the node stops.
func SetDiskSpace(f func(dir string) (free, total uint64, err error)) func() {
	diskSpace = f
	return func() { diskSpace = statDisk }
}

// StrongestNonces orders nonces as a layers-from reply gives them.
var StrongestNonces = strongestNonces

// ProgressCount returns how many yielded chases the host remembers.
func (h *Host) ProgressCount() int {
	h.progMu.Lock()
	defer h.progMu.Unlock()
	return len(h.prog)
}

// ProgressCounted returns, over the kept progress, how many entries have
// their layers counted and the work counted.
func (h *Host) ProgressCounted() (entries int, total weight.Weight) {
	h.progMu.Lock()
	defer h.progMu.Unlock()
	for _, st := range h.prog {
		entries += len(st.counted)
		total = total.Add(st.total)
	}
	return entries, total
}

// Coverage returns the sections the host announces.
func (h *Host) Coverage() []wire.Section { return h.coverage }

// IndexedPools returns how many pools the host has indexed.
func (h *Host) IndexedPools() int { return len(h.sortedPools()) }

// Chasing counts the records of pools held only for a chase (RED-057).
func (h *Host) Chasing() int {
	h.idxMu.RLock()
	defer h.idxMu.RUnlock()
	return len(h.chasing)
}

// SendRelease sends r to every connected peer as it is, unchecked, so a
// test can pass on a release that does not check.
func (h *Host) SendRelease(r series.Release) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.peers {
		p.send(h.releaseMsg(r))
	}
}

// SetReleaseWindow sets the release window for a test and returns a
// function that puts it back.
func SetReleaseWindow(d time.Duration) func() {
	old := releaseWindow
	releaseWindow = d
	return func() { releaseWindow = old }
}

// SetPortMapWaits shortens the port mapping's waits and returns the call
// that puts them back.
func SetPortMapWaits(retry, renew time.Duration) func() {
	r, m, n, l := pmRetry, pmMaxRetry, pmMinRenew, pmMinLife
	pmRetry, pmMaxRetry, pmMinRenew, pmMinLife = retry, retry, renew, min(pmMinLife, renew)
	return func() { pmRetry, pmMaxRetry, pmMinRenew, pmMinLife = r, m, n, l }
}

// SetHelloEdit makes a host built from c change its hello with f.
func SetHelloEdit(c *Config, f func(*wire.Hello)) { c.editHello = f }
