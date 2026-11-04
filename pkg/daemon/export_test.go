package daemon

// SetBeforeReopen sets the hook that runs in the gap of a reload, and
// returns a func that clears it.
func SetBeforeReopen(f func(dir string)) func() {
	beforeReopen = f
	return func() { beforeReopen = nil }
}
