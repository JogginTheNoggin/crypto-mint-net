package cli

import (
	"strings"
	"testing"
)

// poon doctor's text keeps its columns in line with a name longer than
// eight letters, such as open-files (GREEN-053).
func TestDoctorColumnsLineUp(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-p2p", "off", "-root", root)
	out, _ := runErr("doctor", "-root", root)
	col := -1
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 6 || line[4] != ' ' || strings.HasPrefix(line, "data") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "fix:") {
			if i := strings.Index(line, "fix:"); i != col {
				t.Errorf("fix at column %d, details at %d:\n%s", i, col, out)
			}
			continue
		}
		rest := line[6:]
		i := 6 + strings.Index(rest, " ")
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if col == -1 {
			col = i
		} else if i != col {
			t.Errorf("detail at column %d, want %d:\n%s", i, col, out)
		}
	}
	if col == -1 || !strings.Contains(out, "open-files") {
		t.Fatalf("doctor:\n%s", out)
	}
}
