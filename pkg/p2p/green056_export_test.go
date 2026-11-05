package p2p

import "time"

// SetPortMapBackoff sets the first retry wait, the longest one and the
// shortest renewal apart, and returns the call that puts them back
// (GREEN-056).
func SetPortMapBackoff(retry, max, renew time.Duration) func() {
	r, m, n := pmRetry, pmMaxRetry, pmMinRenew
	pmRetry, pmMaxRetry, pmMinRenew = retry, max, renew
	return func() { pmRetry, pmMaxRetry, pmMinRenew = r, m, n }
}
