// Package shell splits a line typed in the GUI's terminal into the command
// line a shell would pass to poo, so a line works the same in both.
package shell

import (
	"errors"
	"strings"
)

// ErrUnterminated is returned for a line that ends inside quotes.
var ErrUnterminated = errors.New("unterminated quote")

// Split splits line into words the way a POSIX shell does for the forms
// people type: words are separated by spaces or tabs; '...' is literal;
// "..." is literal except that \" and \\ are a quote and a backslash; and
// outside quotes a backslash escapes a space, a quote or a backslash. Any
// other backslash is kept, so Windows paths such as C:\Users\a work
// unquoted. A leading "poo" word is dropped, so "poo status" and "status"
// are the same command. Nothing else is interpreted: no variables, globs,
// pipes or redirection.
func Split(line string) ([]string, error) {
	var (
		words []string
		cur   strings.Builder
		in    bool // inside a word
		quote byte // the open quote, or 0
	)
	// Every special character is ASCII, so bytes are split exactly and any
	// other byte is copied as it is.
	rs := []byte(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteByte(r)
			}
		case quote == '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\'):
				i++
				cur.WriteByte(rs[i])
			default:
				cur.WriteByte(r)
			}
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == '\\' && i+1 < len(rs) && strings.IndexByte(" \t'\"\\", rs[i+1]) >= 0:
			i++
			cur.WriteByte(rs[i])
			in = true
		default:
			cur.WriteByte(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, ErrUnterminated
	}
	if in {
		words = append(words, cur.String())
	}
	if len(words) > 0 && words[0] == "poo" {
		words = words[1:]
	}
	return words, nil
}

// Quote returns a command line for args that Split turns back into args,
// for showing the command a view or form runs.
func Quote(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "poo")
	for _, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n\r'\"\\") {
			parts = append(parts, a)
			continue
		}
		parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'"'"'`)+"'")
	}
	return strings.Join(parts, " ")
}
