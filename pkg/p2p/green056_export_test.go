package p2p

import "time"

// SetPortMapBackoff sets the first retry wait, the longest one and the
// shortest renewal apart, and returns the call that puts them back
// (GREEN-056).
func SetPortMapBackoff(retry, max, renew time.Duration) func() {
	r, m, n, l := pmRetry, pmMaxRetry, pmMinRenew, pmMinLife
	pmRetry, pmMaxRetry, pmMinRenew, pmMinLife = retry, max, renew, min(pmMinLife, renew)
	return func() { pmRetry, pmMaxRetry, pmMinRenew, pmMinLife = r, m, n, l }
}
