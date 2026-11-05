package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestDocsNameRealCommands checks that every poo command line in the docs
// names a command the tool has and only flags that command defines, so
// the docs cannot drift from the code (GREEN-010). Proposals are left
// out: they describe commands that do not exist yet, and typos on
// purpose.
func TestDocsNameRealCommands(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{"README.md", "CONTRIBUTING.md", "SECURITY.md", "AGENTS.md"}
	filepath.WalkDir(filepath.Join(root, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "proposals" {
			return filepath.SkipDir
		}
		// White's proposals (docs/white/<layer>/WHITE-NNN-*.md) are
		// proposals too.
		if strings.HasSuffix(p, ".md") && !strings.HasPrefix(d.Name(), "WHITE-") {
			rel, _ := filepath.Rel(root, p)
			files = append(files, rel)
		}
		return nil
	})
	line := regexp.MustCompile("(?:^|`|\\$ |\\./|\\s)poo ([a-z][^`|\\n#]*)")
	flagWord := regexp.MustCompile(`^-[a-z][a-z0-9-]*$`)
	names := commandNames()
	cache := map[string]map[string]bool{}
	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for i, text := range strings.Split(string(b), "\n") {
			for _, m := range line.FindAllStringSubmatch(text, -1) {
				words := strings.Fields(m[1])
				if !slices.Contains(names, words[0]) {
					t.Errorf("%s:%d: poo %s: no such command", f, i+1, words[0])
					continue
				}
				path := []string{words[0]}
				if len(words) > 1 && slices.Contains(subcommands(words[0]), words[1]) {
					path = append(path, words[1])
				}
				key := strings.Join(path, " ")
				if cache[key] == nil {
					cache[key] = map[string]bool{"-h": true}
					for n := range commandFlags(path) {
						cache[key]["-"+n] = true
					}
				}
				for _, w := range words[len(path):] {
					if flagWord.MatchString(w) && !cache[key][w] {
						t.Errorf("%s:%d: poo %s: no flag %s", f, i+1, key, w)
					}
				}
				checked++
			}
		}
	}
	if checked < 50 {
		t.Fatalf("only %d command lines found; is the pattern right?", checked)
	}
}
