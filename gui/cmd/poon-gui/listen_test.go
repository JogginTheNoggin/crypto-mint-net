package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain keeps every test's remembered port out of the user's own
// config directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "poon-gui-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("HOME", dir) // macOS reads $HOME/Library/Application Support
	os.Setenv("AppData", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// TestListenRemembersPort checks that with no -listen poon-gui comes back
// on the port it used last, so the browser's saved settings (kept per
// origin, port included) survive a restart (GUI White G1).
func TestListenRemembersPort(t *testing.T) {
	file := lastPortFile()
	if file == "" {
		t.Skip("no config directory on this system")
	}
	os.Remove(file)
	ln, err := listenGUI("")
	if err != nil {
		t.Fatal(err)
	}
	first := ln.Addr().String()
	ln.Close()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("the port was not kept: %v", err)
	}
	if _, port, _ := net.SplitHostPort(first); strings.TrimSpace(string(b)) != port {
		t.Fatalf("kept %q, listened on %s", b, first)
	}
	ln, err = listenGUI("")
	if err != nil {
		t.Fatal(err)
	}
	if ln.Addr().String() != first {
		t.Errorf("second start on %s, want %s", ln.Addr(), first)
	}
	// The port is taken: any free port, which is then kept.
	ln2, err := listenGUI("")
	if err != nil {
		t.Fatal(err)
	}
	if ln2.Addr().String() == first {
		t.Fatal("listened twice on one port")
	}
	_, port2, _ := net.SplitHostPort(ln2.Addr().String())
	if b, _ := os.ReadFile(file); strings.TrimSpace(string(b)) != port2 {
		t.Errorf("kept %q after the fallback, want %s", b, port2)
	}
	ln.Close()
	ln2.Close()
	// An explicit -listen is used as given and changes nothing kept.
	ln3, err := listenGUI("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln3.Close()
	if b, _ := os.ReadFile(file); strings.TrimSpace(string(b)) != port2 {
		t.Errorf("an explicit -listen changed the kept port to %q", b)
	}
	if fi, err := os.Stat(filepath.Dir(file)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("config folder mode: %v %v", fi, err)
	}
}
