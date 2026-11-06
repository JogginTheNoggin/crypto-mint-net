package web

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestStorageKeysInStore is GUI-037: every name the page keeps in the
// browser's storage is in static/store.js, so a rename of the GUI changes
// one file, and no name from before the poo → poon rename is left (nothing
// is released, so there is no migration and no list of old names).
func TestStorageKeysInStore(t *testing.T) {
	quoted := regexp.MustCompile(`['"\x60]poon?-gui-`)
	err := fs.WalkDir(os.DirFS("static"), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") || strings.HasPrefix(p, "vendor/") {
			return err
		}
		b, err := os.ReadFile("static/" + p)
		if err != nil {
			return err
		}
		src := string(b)
		if strings.Contains(src, "poo-gui-") {
			t.Errorf("%s still names an old poo-gui- storage key", p)
		}
		if p != "store.js" && quoted.MatchString(src) {
			t.Errorf("%s names a storage key itself; use store.js", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("static/store.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HISTORY", "WHERE", "THEME", "WALLET", "WALLET_SERIES"} {
		if !strings.Contains(string(b), "export const "+name+" = PREFIX + ") {
			t.Errorf("store.js does not export %s", name)
		}
	}
}
