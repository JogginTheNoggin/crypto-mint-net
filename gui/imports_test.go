package gui

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	core = "github.com/JogginTheNoggin/crypto-mint-net"
	self = core + "/gui"
)

// allowed are the only core packages the GUI may import. Everything the
// GUI does goes through the poon commands, so it cannot re-implement or
// bypass them (docs/gui.md, D86).
var allowed = map[string]bool{core + "/pkg/cli": true}

// TestImportsOnlyCLI fails if any file of the GUI module, tests included,
// imports a core package other than pkg/cli, or any module other than the
// standard library and the core.
func TestImportsOnlyCLI(t *testing.T) {
	fset := token.NewFileSet()
	n := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		n++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			switch {
			case p == self || strings.HasPrefix(p, self+"/"):
			case allowed[p]:
			case strings.HasPrefix(p, core+"/") || p == core:
				t.Errorf("%s imports %s: the GUI may use the core only through pkg/cli", path, p)
			case strings.Contains(strings.SplitN(p, "/", 2)[0], "."):
				t.Errorf("%s imports %s: the GUI takes no dependencies beyond the core", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 5 {
		t.Fatalf("only %d files checked", n)
	}
}

// TestCoreDoesNotImportGUI checks the other direction: nothing in the core
// module mentions the GUI module, so the headless tool never contains it.
func TestCoreDoesNotImportGUI(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join("..", "gui") || strings.HasPrefix(d.Name(), ".") && path != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == self || strings.HasPrefix(p, self+"/") {
				t.Errorf("%s imports %s", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("..", "go.mod")); err != nil {
		t.Fatal("the core module is not the parent directory")
	}
}
