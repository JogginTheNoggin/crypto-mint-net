package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// settingsCmd is poo settings: a node's settings, everything in its node
// policy but what it follows, written to one file and read back, to carry
// them to another machine, keep a backup or share a setup (D155). The
// file is a bundle (nodepolicy.ScopeSettings), and import is poo policy
// import's, taking only the settings.
func settingsCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: poo settings export|import (poo help settings)")
	}
	switch args[0] {
	case "export":
		return settingsExportCmd(args[1:], out)
	case "import":
		return importBundleCmd(args[1:], out, true)
	}
	return fmt.Errorf("poo settings: no subcommand %q: export or import", args[0])
}

// settingsExportCmd writes one node policy's settings to a settings
// bundle: without what it follows, and without machine-local settings
// unless -full.
func settingsExportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("settings export", out)
	w := addWhere(fs)
	full := fs.Bool("full", false, "keep machine-local settings (rpc, no_sync, p2p listen and advertise, grind device and threads) and personal ones (registry files, registry URLs with a user, password or query, grind jobs on one payment or entry): for a backup, or a new machine that should run the same way")
	file := fs.String("out", "", "write the settings to this file (default: print them)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo settings export takes flags only")
	}
	p, err := nodepolicy.Load(filepath.Join(w.resolve(), nodepolicy.FileName))
	if err != nil {
		return err
	}
	b := nodepolicy.NewBundle(nodepolicy.ScopeSettings, *full, time.Now())
	b.Policies = []nodepolicy.BundlePolicy{{Policy: nodepolicy.ExportSettings(p, *full)}}
	what := "the settings"
	if *full {
		what = "the settings, machine-local ones too,"
	}
	return writeBundle(out, b, *file, what, nodepolicy.ExportNotes(p, *full))
}
