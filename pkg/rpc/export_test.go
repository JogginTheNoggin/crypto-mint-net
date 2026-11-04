package rpc

import "time"

// SetWorkTimeBound sets the time bound on a large work request for a test
// and returns a function that puts it back.
func SetWorkTimeBound(d time.Duration) func() {
	old := workTimeBound
	workTimeBound = d
	return func() { workTimeBound = old }
}
