package nodepolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
)

// Manager keeps several node policies under one root directory:
//
//	<root>/policies/<name>/node.json   saved: used again, the node picks up
//	                                   where it left off (its data is beside it)
//	<root>/archive/<name>/...          archived: kept apart
//	<root>/current                     the name of the policy in use
//
// The design leaves open what archiving is for. Here an archived policy
// keeps its data. Using it again (Use) starts from scratch, as the design
// says; taking it out of the archive by hand (Unarchive) picks up where it
// left off.
type Manager struct {
	Root string
}

// Errors returned by a Manager.
var (
	ErrExists   = errors.New("nodepolicy: a node policy with that name exists")
	ErrNotFound = errors.New("nodepolicy: no node policy with that name")
	ErrName     = errors.New("nodepolicy: names use letters, digits, '.', '_' and '-' and do not start with '.'")
)

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`)

func checkName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrName, name)
	}
	return nil
}

// checkNewName checks the name of a node policy about to be made, here or
// from a bundle: as checkName, and also not a name Windows takes for a
// device (con, nul, com1, ...) or changes (a trailing dot), so a node
// policy made or shared on one system can be kept on any (RED-045, D150).
// A node policy already saved under such a name still loads.
func checkNewName(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if strings.HasSuffix(name, ".") {
		return fmt.Errorf("%w: %q ends with '.', which Windows drops", ErrName, name)
	}
	base, _, _ := strings.Cut(strings.ToLower(name), ".")
	switch base {
	case "con", "prn", "aux", "nul", "conin$", "conout$":
		return fmt.Errorf("%w: %q is a device name on Windows", ErrName, name)
	}
	if len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9' {
		return fmt.Errorf("%w: %q is a device name on Windows", ErrName, name)
	}
	return nil
}

// Dir returns the data directory of a saved node policy.
func (m Manager) Dir(name string) string { return filepath.Join(m.Root, "policies", name) }

func (m Manager) archiveDir(name string) string { return filepath.Join(m.Root, "archive", name) }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save stores p under name, creating the policy or replacing its settings.
// Data the node already holds for it is kept.
func (m Manager) Save(name string, p Policy) error {
	check := checkName
	if !exists(m.Dir(name)) {
		check = checkNewName
	}
	if err := check(name); err != nil {
		return err
	}
	if exists(m.archiveDir(name)) {
		return fmt.Errorf("%w: %q is archived", ErrExists, name)
	}
	return Save(filepath.Join(m.Dir(name), FileName), p)
}

// Load reads a saved node policy.
func (m Manager) Load(name string) (Policy, error) {
	if err := checkName(name); err != nil {
		return Policy{}, err
	}
	p, err := Load(filepath.Join(m.Dir(name), FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Policy{}, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return p, err
}

// Entry is one row of List.
type Entry struct {
	Name     string
	Archived bool
	Current  bool
}

// List returns every node policy, saved ones first, each group by name.
func (m Manager) List() ([]Entry, error) {
	cur, _ := m.Current()
	var out []Entry
	for _, group := range []struct {
		dir      string
		archived bool
	}{{"policies", false}, {"archive", true}} {
		des, err := os.ReadDir(filepath.Join(m.Root, group.dir))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		var names []string
		for _, d := range des {
			if d.IsDir() && checkName(d.Name()) == nil {
				names = append(names, d.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, Entry{Name: n, Archived: group.archived, Current: !group.archived && n == cur})
		}
	}
	return out, nil
}

// Current returns the name of the node policy in use, if one is set.
func (m Manager) Current() (string, error) {
	b, err := os.ReadFile(filepath.Join(m.Root, "current"))
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(b))
	return name, checkName(name)
}

// Use makes name the node policy in use. A saved policy picks up where it
// left off. An archived one starts from scratch: its data is deleted and
// its settings are saved again.
func (m Manager) Use(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	switch {
	case exists(m.Dir(name)):
	case exists(m.archiveDir(name)):
		if err := m.Unarchive(name, true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return writeAtomic(filepath.Join(m.Root, "current"), []byte(name+"\n"))
}

// Archive moves a saved node policy, with its data, into the archive. The
// node using it must be stopped first.
func (m Manager) Archive(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if !exists(m.Dir(name)) {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if exists(m.archiveDir(name)) {
		return fmt.Errorf("%w: %q is already archived", ErrExists, name)
	}
	unlock, err := Lock(m.Dir(name))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.Root, "archive"), 0o700); err != nil {
		unlock()
		return err
	}
	unlock()
	if err := os.Rename(m.Dir(name), m.archiveDir(name)); err != nil {
		return err
	}
	if cur, _ := m.Current(); cur == name {
		os.Remove(filepath.Join(m.Root, "current"))
	}
	return nil
}

// Unarchive moves an archived node policy back. With fresh, its data is
// deleted and only its settings are kept, so the node starts from scratch;
// otherwise it picks up where it left off.
func (m Manager) Unarchive(name string, fresh bool) error {
	if err := checkName(name); err != nil {
		return err
	}
	src := m.archiveDir(name)
	if !exists(src) {
		return fmt.Errorf("%w: %q is not archived", ErrNotFound, name)
	}
	if exists(m.Dir(name)) {
		return fmt.Errorf("%w: %q", ErrExists, name)
	}
	if fresh {
		p, err := Load(filepath.Join(src, FileName))
		if err != nil {
			return err
		}
		if err := Save(filepath.Join(m.Dir(name), FileName), p); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	if err := os.MkdirAll(filepath.Join(m.Root, "policies"), 0o700); err != nil {
		return err
	}
	return os.Rename(src, m.Dir(name))
}

// Delete removes a node policy and its data for good, whether saved or
// archived.
func (m Manager) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	found := false
	for _, dir := range []string{m.Dir(name), m.archiveDir(name)} {
		if !exists(dir) {
			continue
		}
		found = true
		unlock, err := Lock(dir)
		if err != nil {
			return err
		}
		unlock()
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if cur, _ := m.Current(); cur == name {
		os.Remove(filepath.Join(m.Root, "current"))
	}
	return nil
}

func writeAtomic(path string, b []byte) error {
	return fsutil.WriteFile(path, b, 0o600) // RED-041, D148
}

// ErrLocked: another process is using the data directory.
var ErrLocked = errors.New("nodepolicy: data directory in use by another node")
