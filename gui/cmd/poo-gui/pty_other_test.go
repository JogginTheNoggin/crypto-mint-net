//go:build !linux && !windows

package main

import (
	"os"
	"testing"
)

// openTTY makes pseudo-terminals on Linux only; elsewhere the tests that
// need one skip.
func openTTY(t *testing.T) *os.File { return nil }
