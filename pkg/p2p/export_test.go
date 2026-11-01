package p2p

import "github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"

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
