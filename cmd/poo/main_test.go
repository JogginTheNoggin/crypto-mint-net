package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBinaryRunsCLI builds the command and checks it is package cli with
// the process around it: output, and the exit code of a failed command.
func TestBinaryRunsCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "poo")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "version").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("version: %v %q", err, out)
	}
	cmd := exec.Command(bin, "no-such-command")
	cmd.Env = append(os.Environ(), "POO_HOME="+t.TempDir())
	b, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.HasPrefix(string(b), "poo: unknown command") {
		t.Fatalf("bad command: %v %q", err, b)
	}
}
