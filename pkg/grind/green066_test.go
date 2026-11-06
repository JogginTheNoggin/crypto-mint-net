package grind

import (
	"errors"
	"testing"
)

// GREEN-066: POON_OPENCL takes off, 0, false or no in any case, and the
// OpenCL status line says how many devices or why none.
func TestOpenCLOffAndStatus(t *testing.T) {
	for v, want := range map[string]bool{"off": true, "OFF": true, "0": true, "false": true, " no ": true, "": false, "on": false, "1": false, "bogus": false} {
		if openCLOff(v) != want {
			t.Errorf("openCLOff(%q) = %v", v, !want)
		}
	}
	for _, c := range []struct {
		n    int
		err  error
		want string
	}{
		{0, errors.New("turned off by POON_OPENCL"), "no GPU grinder: turned off by POON_OPENCL"},
		{0, nil, "no GPU grinder: an OpenCL driver is installed but lists no GPU"},
		{1, nil, "OpenCL: 1 device"},
		{3, nil, "OpenCL: 3 devices"},
	} {
		if got := openCLStatusOf(c.n, c.err); got != c.want {
			t.Errorf("openCLStatusOf(%d, %v) = %q, want %q", c.n, c.err, got, c.want)
		}
	}
	if OpenCLStatus() == "" {
		t.Error("OpenCLStatus is empty")
	}
}
