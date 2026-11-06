//go:build !windows

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHangupTable pins what SIGHUP does in each case (GUI-010, GUI-011).
func TestHangupTable(t *testing.T) {
	base := []os.Signal{os.Interrupt, syscall.SIGTERM}
	cases := []struct {
		ignored, tty bool
		stopOn       []os.Signal
		absorb       bool
	}{
		{false, true, append(base, syscall.SIGHUP), false}, // the terminal closed
		{false, false, base, true},                         // a daemon's reload
		{true, true, base, false},                          // nohup: left ignored
		{true, false, base, false},
	}
	for _, c := range cases {
		stopOn, absorb := hangup(c.ignored, c.tty)
		if !reflect.DeepEqual(stopOn, c.stopOn) || absorb != c.absorb {
			t.Errorf("hangup(ignored %v, tty %v) = %v, %v; want %v, %v", c.ignored, c.tty, stopOn, absorb, c.stopOn, c.absorb)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Error("a pipe is a terminal")
	}
	if tty := openTTY(t); tty != nil && !isTerminal(tty) {
		t.Error("a pty is not a terminal")
	}
}

// TestMainProcess is poon-gui's main, run in a child process by the tests
// below.
func TestMainProcess(t *testing.T) {
	if os.Getenv("POON_GUI_TEST_MAIN") != "1" {
		t.Skip("run by the signal tests")
	}
	os.Args = []string{"poon-gui", "-no-open"}
	main()
}

// gui is poon-gui's main in a child process, started from stdin, with
// SIGHUP ignored first if ignoreHUP, as nohup does.
type gui struct {
	t    *testing.T
	cmd  *exec.Cmd
	done chan error
}

func startGUI(t *testing.T, stdin *os.File, ignoreHUP bool) *gui {
	t.Helper()
	self := []string{os.Args[0], "-test.run=^TestMainProcess$"}
	if ignoreHUP {
		self = append([]string{"sh", "-c", `trap '' HUP; exec "$@"`, "sh"}, self...)
	}
	cmd := exec.Command(self[0], self[1:]...)
	cmd.Env = append(os.Environ(), "POON_GUI_TEST_MAIN=1", "HOME="+t.TempDir())
	cmd.Stdin = stdin
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	g := &gui{t: t, cmd: cmd, done: make(chan error, 1)}
	up := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "?t=") {
				select {
				case up <- true:
				default:
				}
			}
		}
		io.Copy(io.Discard, out)
	}()
	select {
	case <-up:
	case <-time.After(20 * time.Second):
		cmd.Process.Kill()
		t.Fatal("poon-gui did not start")
	}
	go func() { g.done <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	return g
}

// stopsCleanly sends sig and requires exit 0.
func (g *gui) stopsCleanly(sig syscall.Signal) {
	g.t.Helper()
	g.cmd.Process.Signal(sig)
	select {
	case err := <-g.done:
		if err != nil {
			g.t.Errorf("%v: poon-gui ended with %v, want a clean exit 0", sig, err)
		}
	case <-time.After(20 * time.Second):
		g.t.Fatalf("%v: poon-gui did not stop", sig)
	}
}

// survives sends sig and requires poon-gui to keep running.
func (g *gui) survives(sig syscall.Signal) {
	g.t.Helper()
	g.cmd.Process.Signal(sig)
	select {
	case err := <-g.done:
		g.t.Fatalf("%v: poon-gui ended (%v), want it to keep running", sig, err)
	case <-time.After(time.Second):
	}
}

// TestHangupStopsCleanly is GUI-010: started from a terminal, SIGHUP (the
// terminal closing) ends poon-gui through its cancel path, exit 0, as
// SIGTERM does.
func TestHangupStopsCleanly(t *testing.T) {
	tty := openTTY(t)
	if tty == nil {
		t.Skip("no pseudo-terminal here")
	}
	startGUI(t, tty, false).stopsCleanly(syscall.SIGHUP)
	startGUI(t, tty, false).stopsCleanly(syscall.SIGTERM)
}

// TestHangupNohup is GUI-011: started with SIGHUP ignored, as nohup does,
// poon-gui leaves it ignored and outlives the session, terminal or not.
func TestHangupNohup(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	if tty := openTTY(t); tty != nil {
		g := startGUI(t, tty, true)
		g.survives(syscall.SIGHUP)
		g.stopsCleanly(syscall.SIGTERM)
	}
	r, w, _ := os.Pipe() // w held open: EOF on stdin is not a stop
	defer w.Close()
	g := startGUI(t, r, true)
	g.survives(syscall.SIGHUP)
	g.stopsCleanly(syscall.SIGTERM)
}

// TestHangupNoTerminal is GUI-011: without a terminal, SIGHUP is a
// daemon's reload; it neither stops poon-gui nor kills it the way Go's
// default would, so a node the GUI hosts can reload on it.
func TestHangupNoTerminal(t *testing.T) {
	r, w, _ := os.Pipe()
	defer w.Close()
	g := startGUI(t, r, false)
	g.survives(syscall.SIGHUP)
	g.stopsCleanly(syscall.SIGTERM)
}
