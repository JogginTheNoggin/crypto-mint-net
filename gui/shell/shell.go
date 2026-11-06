// Package shell splits a line typed in the GUI's terminal into the command
// line the platform's shell would pass to poon, so a line works the same in
// both. A line that would mean something more in a real shell (a pipe, a
// variable, a glob) is refused instead of being given a meaning the shell
// would not give it (docs/gui.md, The terminal's grammar).
package shell

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// ErrUnterminated is returned for a line that ends inside quotes or with
// a backslash.
var ErrUnterminated = errors.New("unterminated quote")

// ErrNewline is returned for a line holding a CR or LF: a shell would end
// the command there, and a terminal line is one line.
var ErrNewline = errors.New("a command line is one line: it may not contain CR or LF")

// UnsupportedError is returned for a character a real shell would treat as
// syntax that the GUI's terminal does not run, such as | or $.
type UnsupportedError struct {
	Char byte
	Pos  int // byte offset in the line
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("the terminal runs one poon command and no shell syntax: quote %q (at byte %d) to pass it to poon", e.Char, e.Pos)
}

// Grammar is a shell's word splitting.
type Grammar int

// The grammars Split knows.
const (
	// POSIX is sh word splitting: words are separated by spaces or tabs;
	// '...' is literal; in "..." a backslash escapes only $ ` " and \;
	// outside quotes a backslash escapes any character; # at the start
	// of a word begins a comment. Unquoted | & ; < > ( ) $ ` * ? [ and a
	// leading ~ are refused, as is $ or ` inside "...".
	POSIX Grammar = iota
	// Windows is cmd.exe passing a line to poon.exe, split the way Go
	// splits a Windows command line: words are separated by spaces or
	// tabs; "..." groups, and "" inside quotes is a quote; backslashes are
	// literal except before a quote, where 2n backslashes give n and open
	// or close quotes and 2n+1 give n and a literal quote. Single quotes
	// are ordinary characters. Unquoted & | < > ^ are refused.
	Windows
)

// Native is the grammar of the shell poon runs from on this platform.
var Native = native()

func native() Grammar {
	if runtime.GOOS == "windows" {
		return Windows
	}
	return POSIX
}

func (g Grammar) String() string {
	if g == Windows {
		return "windows"
	}
	return "posix"
}

// Split splits line with the Native grammar.
func Split(line string) ([]string, error) { return Native.Split(line) }

// Quote quotes args with the Native grammar.
func Quote(args []string) string { return Native.Quote(args) }

// Split splits line into words. A leading "poon" word is dropped, so
// "poon status" and "status" are the same command, as the terminal's
// prompt already reads "poon". Nothing else is interpreted.
func (g Grammar) Split(line string) ([]string, error) {
	if strings.ContainsAny(line, "\r\n") {
		return nil, ErrNewline
	}
	var words []string
	var err error
	if g == Windows {
		words, err = splitWindows(line)
	} else {
		words, err = splitPOSIX(line)
	}
	if err != nil {
		return nil, err
	}
	if len(words) > 0 && words[0] == "poon" {
		words = words[1:]
	}
	return words, nil
}

// Every special character is ASCII, so lines are split as bytes and any
// other byte is copied as it is.

func splitPOSIX(line string) ([]string, error) {
	var (
		words []string
		cur   strings.Builder
		in    bool // inside a word
		quote byte // the open quote, or 0
	)
	for i := 0; i < len(line); i++ {
		r := line[i]
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
			case r == '$' || r == '`':
				return nil, &UnsupportedError{r, i}
			case r == '\\' && i+1 < len(line) && strings.IndexByte("$`\"\\", line[i+1]) >= 0:
				i++
				cur.WriteByte(line[i])
			default:
				cur.WriteByte(r)
			}
		case r == ' ' || r == '\t':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		case r == '#' && !in:
			i = len(line) // a comment runs to the end of the line
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == '\\':
			if i+1 == len(line) {
				return nil, ErrUnterminated // a shell would read another line
			}
			i++
			cur.WriteByte(line[i])
			in = true
		case strings.IndexByte("|&;<>()$`*?[", r) >= 0, r == '~' && !in:
			return nil, &UnsupportedError{r, i}
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
	return words, nil
}

func splitWindows(line string) ([]string, error) {
	var (
		words []string
		cur   strings.Builder
		in    bool // inside a word
		quote bool
	)
	for i := 0; i < len(line); i++ {
		r := line[i]
		switch {
		case r == '\\':
			n := 0
			for i < len(line) && line[i] == '\\' {
				n++
				i++
			}
			if i < len(line) && line[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 {
					cur.WriteByte('"')
				} else {
					quote = !quote
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
				i--
			}
			in = true
		case r == '"':
			if quote && i+1 < len(line) && line[i+1] == '"' {
				i++
				cur.WriteByte('"')
			} else {
				quote = !quote
			}
			in = true
		case quote:
			cur.WriteByte(r)
		case r == ' ' || r == '\t':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		case strings.IndexByte("&|<>^", r) >= 0:
			return nil, &UnsupportedError{r, i}
		default:
			cur.WriteByte(r)
			in = true
		}
	}
	if quote {
		return nil, ErrUnterminated
	}
	if in {
		words = append(words, cur.String())
	}
	return words, nil
}

// safePOSIX are the bytes a POSIX word may hold unquoted.
const safePOSIX = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.,/:@%+="

// Quote returns a command line for args that Split turns back into args,
// for showing the command a view or form runs. An arg holding CR or LF
// cannot be typed on one line, so Split refuses the line Quote gives for
// it.
func (g Grammar) Quote(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "poon")
	for _, a := range args {
		parts = append(parts, g.quoteWord(a))
	}
	return strings.Join(parts, " ")
}

func (g Grammar) quoteWord(a string) string {
	if g == Windows {
		if a != "" && !strings.ContainsAny(a, " \t\"&|<>^") {
			return a
		}
		// Backslashes before a quote, and at the end before the closing
		// quote, are doubled.
		var b strings.Builder
		b.WriteByte('"')
		n := 0
		for i := 0; i < len(a); i++ {
			switch a[i] {
			case '\\':
				n++
			case '"':
				b.WriteString(strings.Repeat(`\`, n+1))
				n = 0
			default:
				n = 0
			}
			b.WriteByte(a[i])
		}
		b.WriteString(strings.Repeat(`\`, n))
		b.WriteByte('"')
		return b.String()
	}
	if a != "" && strings.Trim(a, safePOSIX) == "" {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'"'"'`) + "'"
}
