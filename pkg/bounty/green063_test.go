package bounty

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GREEN-063: Done returns the last n finished offers oldest first, and
// skips lines it cannot read (too long, or no offer) without failing.
func TestDoneListsTheLastOffers(t *testing.T) {
	dir := t.TempDir()
	b, err := Open(Config{Post: true, MaxAmount: 10, Budget: 10}, filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := b.Done(5); err != nil || len(got) != 0 {
		t.Fatalf("no done file: %v %v", got, err)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf(`{"format":1,"kind":%q}`, DoneFileKind))
	for i := 1; i <= 5; i++ {
		lines = append(lines, fmt.Sprintf(`{"id":"%064x","state":"closed"}`, i))
		if i == 3 {
			lines = append(lines, `{"not":"an offer"}`, `{"id":"`+strings.Repeat("a", maxDoneLine)+`"}`)
		}
	}
	data := strings.Join(lines, "\n") + "\n" + `{"id":"cut sh` // a line being written
	if err := os.WriteFile(filepath.Join(dir, DoneFileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := b.Done(3)
	if err != nil || skipped != 2 || len(got) != 3 {
		t.Fatalf("Done(3) = %d offers, %d skipped, %v", len(got), skipped, err)
	}
	for i, m := range got {
		if want := fmt.Sprintf("%064x", i+3); m.ID.String() != want {
			t.Fatalf("offer %d is %s, want %s", i, m.ID, want)
		}
	}
	if got, _, _ := b.Done(100000); len(got) != 5 {
		t.Fatalf("Done past the cap: %d", len(got))
	}
}
