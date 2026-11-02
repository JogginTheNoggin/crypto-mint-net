//go:build !unix && !windows

package wallet

// lockFile does nothing on this platform: Save still compares the file
// with what was read before it writes, which catches a change made
// between the two, but not one made in the instant of the write (GUI-019,
// D145).
func lockFile(string, func()) (func(), error) { return func() {}, nil }
