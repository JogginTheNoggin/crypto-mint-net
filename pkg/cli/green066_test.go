package cli

import (
	"strings"
	"testing"
)

// GREEN-066: with no GPU listed, poon grind devices and doctor said
// nothing about the GPU grinder, so a machine whose GPU the system does
// not list (or with no driver at all) gave no reason. Both now carry the
// OpenCL status line; the CLI tests run with POON_OPENCL=off.
func TestGrindDevicesSayWhyNoGPU(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	var devs struct {
		OpenCL string `json:"opencl"`
	}
	cli(t, &devs, append([]string{"grind", "devices"}, r...)...)
	if !strings.Contains(devs.OpenCL, "turned off by POON_OPENCL") {
		t.Fatalf("grind devices opencl = %q", devs.OpenCL)
	}
	out := runOut(t, append([]string{"doctor"}, r...)...)
	if !strings.Contains(out, "no GPU grinder: turned off by POON_OPENCL") {
		t.Fatalf("doctor does not say why no GPU grinds:\n%s", out)
	}
}
