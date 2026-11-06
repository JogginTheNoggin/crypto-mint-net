package grind

import (
	"os"
	"testing"
)

// TestMain lists GPUs without the GPU grinder's driver, so the tests see
// the same devices on a machine with a GPU as on one without; the OpenCL
// tests turn it on for themselves (D208).
func TestMain(m *testing.M) {
	gpuProbe = probeGPUs
	os.Exit(m.Run())
}
