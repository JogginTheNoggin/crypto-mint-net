package p2p

import "time"

// SetPortMapConfirmWait sets how long a dial-back waits for this node's
// listener, and returns the call that puts it back (BLUE-032).
func SetPortMapConfirmWait(d time.Duration) func() {
	old := pmConfirmWait
	pmConfirmWait = d
	return func() { pmConfirmWait = old }
}

// RenewAfter is the wait before a mapping of lifetime life is renewed.
func (h *Host) RenewAfter(life time.Duration) time.Duration { return h.renewAfter(life) }
