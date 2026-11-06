package cli

import (
	"fmt"
	"io"
	"strings"
)

// Help for one command and suggestions for a mistyped one (GREEN-001).
// Both read the usage text, so a new command needs only its usage lines.

// commandNames returns every command the usage names, in order.
func commandNames() []string {
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(usage, "\n") {
		rest, ok := strings.CutPrefix(line, "  poon ")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(rest, " ")
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// commandUsage returns the usage lines of the command name: its own lines
// and their continuation lines, or "" when the usage does not name it.
func commandUsage(name string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(usage, "\n") {
		if rest, ok := strings.CutPrefix(line, "  poon "); ok {
			n, _, _ := strings.Cut(rest, " ")
			in = n == name
		} else if !strings.HasPrefix(line, "    ") {
			in = false
		}
		if in {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// helpCmd prints the whole usage, or one command's usage lines.
func helpCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return nil
	}
	u := commandUsage(args[0])
	if u == "" {
		return unknownCommand(args[0])
	}
	fmt.Fprintf(out, "Usage:\n%s", u)
	if args[0] == "wallet" || args[0] == "sim" {
		fmt.Fprintf(out, "\nMore: poon %s help\n", args[0])
	}
	return nil
}

// unknownCommand is the error for a command the tool does not have: short,
// with the nearest commands when the name looks like a typo of one.
func unknownCommand(name string) error {
	if s := suggest(name, commandNames()); len(s) > 0 {
		return fmt.Errorf("unknown command %q; did you mean %s? (poon help lists every command)", name, strings.Join(s, " or "))
	}
	return fmt.Errorf("unknown command %q (poon help lists every command)", name)
}

// suggest returns the names within a small edit distance of s, nearest
// first, at most three. A name s is a prefix of counts as near.
func suggest(s string, names []string) []string {
	max := 2
	if len(s) <= 3 {
		max = 1
	}
	var out []string
	for d := 0; d <= max; d++ {
		for _, n := range names {
			nd := editDistance(s, n)
			if len(s) >= 3 && strings.HasPrefix(n, s) {
				nd = 0
			}
			if nd == d && len(out) < 3 {
				out = append(out, n)
			}
		}
	}
	return out
}

// editDistance is the Damerau–Levenshtein distance with adjacent
// transpositions (optimal string alignment), so "stauts" is one edit from
// "status".
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
