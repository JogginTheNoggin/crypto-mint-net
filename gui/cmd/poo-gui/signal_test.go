//go:build !windows

package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMainProcess is poo-gui's main, run in a child process by
// TestHangupStopsCleanly.
func TestMainProcess(t *testing.T) {
	if os.Getenv("POO_GUI_TEST_MAIN") != "1" {
		t.Skip("run by TestHangupStopsCleanly")
	}
	os.Args = []string{"poo-gui", "-no-open"}
	main()
}

// TestHangupStopsCleanly is GUI-010: closing the terminal (SIGHUP) ends
// poo-gui through its cancel path, exit 0, as SIGTERM does, instead of
// Go's default of exiting at once with the jobs left as they are.
func TestHangupStopsCleanly(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGTERM} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestMainProcess$")
		cmd.Env = append(os.Environ(), "POO_GUI_TEST_MAIN=1", "HOME="+t.TempDir())
		stdin, err := cmd.StdinPipe() // held open: EOF is not a stop
		if err != nil {
			t.Fatal(err)
		}
		defer stdin.Close()
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		up := make(chan bool, 1)
		go func() {
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				if strings.Contains(sc.Text(), "?t=") {
					up <- true
				}
			}
		}()
		select {
		case <-up:
		case <-time.After(20 * time.Second):
			cmd.Process.Kill()
			t.Fatal("poo-gui did not start")
		}
		cmd.Process.Signal(sig)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("%v: poo-gui ended with %v, want a clean exit 0", sig, err)
			}
		case <-time.After(20 * time.Second):
			cmd.Process.Kill()
			t.Fatalf("%v: poo-gui did not stop", sig)
		}
	}
}
