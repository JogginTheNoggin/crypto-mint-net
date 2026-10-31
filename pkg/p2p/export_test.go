package p2p

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
