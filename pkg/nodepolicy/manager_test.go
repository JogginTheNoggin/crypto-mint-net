package nodepolicy_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

func data(t *testing.T, m nodepolicy.Manager, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(m.Dir(name), "pools.log"))
	if err != nil {
		return ""
	}
	return string(b)
}

func TestManagerLifecycle(t *testing.T) {
	m := nodepolicy.Manager{Root: t.TempDir()}
	p := nodepolicy.Default()
	for _, name := range []string{"home", "shop"} {
		if err := m.Save(name, p); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(m.Dir(name), "pools.log"), []byte(name+" data"), 0o600)
	}
	if err := m.Use("home"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := m.Current(); cur != "home" {
		t.Fatalf("current = %q", cur)
	}

	// Save again keeps data.
	p.MaxPending = 5
	if err := m.Save("home", p); err != nil {
		t.Fatal(err)
	}
	if q, _ := m.Load("home"); q.MaxPending != 5 || data(t, m, "home") != "home data" {
		t.Fatal("save lost settings or data")
	}

	// Archive and take out by hand: picks up where it left off.
	if err := m.Archive("home"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Current(); err == nil {
		t.Fatal("archived policy still current")
	}
	if _, err := m.Load("home"); !errors.Is(err, nodepolicy.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := m.Save("home", p); !errors.Is(err, nodepolicy.ErrExists) {
		t.Fatalf("saved over an archived name: %v", err)
	}
	list, _ := m.List()
	if len(list) != 2 || list[0].Name != "shop" || list[0].Archived || list[1].Name != "home" || !list[1].Archived {
		t.Fatalf("list %+v", list)
	}
	if err := m.Unarchive("home", false); err != nil {
		t.Fatal(err)
	}
	if data(t, m, "home") != "home data" {
		t.Fatal("unarchive lost data")
	}

	// Archive and use again: starts from scratch, settings kept.
	m.Archive("home")
	if err := m.Use("home"); err != nil {
		t.Fatal(err)
	}
	if data(t, m, "home") != "" {
		t.Fatal("using an archived policy kept its data")
	}
	if q, _ := m.Load("home"); q.MaxPending != 5 {
		t.Fatal("settings lost")
	}
	list, _ = m.List()
	if len(list) != 2 || !list[0].Current || list[0].Name != "home" {
		t.Fatalf("list %+v", list)
	}

	// Delete removes for good, saved or archived.
	m.Archive("shop")
	for _, name := range []string{"home", "shop"} {
		if err := m.Delete(name); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ = m.List(); len(list) != 0 {
		t.Fatalf("list %+v", list)
	}
	if _, err := m.Current(); err == nil {
		t.Fatal("deleted policy still current")
	}
}

func TestManagerErrors(t *testing.T) {
	m := nodepolicy.Manager{Root: t.TempDir()}
	for _, bad := range []string{"", ".", "..", ".hidden", "a/b", "a b", string(make([]byte, 65))} {
		if err := m.Save(bad, nodepolicy.Default()); !errors.Is(err, nodepolicy.ErrName) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
	for name, f := range map[string]func() error{
		"use":       func() error { return m.Use("none") },
		"archive":   func() error { return m.Archive("none") },
		"unarchive": func() error { return m.Unarchive("none", false) },
		"delete":    func() error { return m.Delete("none") },
		"load":      func() error { _, err := m.Load("none"); return err },
	} {
		if err := f(); !errors.Is(err, nodepolicy.ErrNotFound) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if list, err := m.List(); err != nil || len(list) != 0 {
		t.Fatalf("empty root: %v %v", list, err)
	}
	m.Save("a", nodepolicy.Default())
	m.Archive("a")
	m.Save("b", nodepolicy.Default())
	if err := m.Archive("a"); !errors.Is(err, nodepolicy.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory locks are advisory only on this platform")
	}
	m := nodepolicy.Manager{Root: t.TempDir()}
	m.Save("busy", nodepolicy.Default())
	unlock, err := nodepolicy.Lock(m.Dir("busy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nodepolicy.Lock(m.Dir("busy")); !errors.Is(err, nodepolicy.ErrLocked) {
		t.Fatalf("second lock: %v", err)
	}
	if err := m.Archive("busy"); !errors.Is(err, nodepolicy.ErrLocked) {
		t.Fatalf("archived a running node's policy: %v", err)
	}
	if err := m.Delete("busy"); !errors.Is(err, nodepolicy.ErrLocked) {
		t.Fatalf("deleted a running node's policy: %v", err)
	}
	unlock()
	if err := m.Delete("busy"); err != nil {
		t.Fatal(err)
	}
}
