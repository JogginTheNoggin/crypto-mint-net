package shell

import (
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// SecretFlags are the poo flags whose value is a secret, as the core
// declares them (cli.SecretFlags, D106): wallet create -restore and keygen
// -seed take a seed, which restores every key. The GUI keeps a line
// holding one of them out of the terminal's saved history and hides the
// value wherever it shows a job's command line (GUI-006 in
// security/gui/findings). sim -seed is a random-number seed, not a secret.
var SecretFlags = cli.SecretFlags()

// Hidden replaces a secret flag's value in a redacted command line.
const Hidden = "<hidden>"

// secretFlag reports whether arg is a secret flag, and whether it carries
// its value after "=".
func secretFlag(arg string) (secret, inline bool) {
	if !strings.HasPrefix(arg, "-") {
		return false, false
	}
	name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	name, _, inline = strings.Cut(name, "=")
	for _, f := range SecretFlags {
		if name == f {
			return true, inline
		}
	}
	return false, false
}

// keyFileFlags take the path of a file that holds a key or a seed. A path
// is not a secret, but a key pasted where the path goes is (GUI-032): it
// would show in the Jobs list, so a value that LooksLikeKey is hidden.
var keyFileFlags = []string{"key-file", "seed-file"}

// keyFileFlag reports whether args[i] is a key file flag whose value
// LooksLikeKey, whether the value starts after "=", and how many of the
// args after it are part of the value. A key typed as several words (in
// groups, as openssl prints it in lines, or in halves) is several args,
// so the value is read as secretLine reads it in the page: word by word,
// up to the next flag, or a word with / \ or . in it (tested with and
// without that word, as a key may end in a dot), or past keyRun
// characters (GUI-035).
func keyFileFlag(args []string, i int) (secret, inline bool, more int) {
	arg := args[i]
	if !strings.HasPrefix(arg, "-") {
		return false, false, 0
	}
	name, val, inline := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
	found := false
	for _, f := range keyFileFlags {
		found = found || name == f
	}
	if !found {
		return false, false, 0
	}
	v, n, j := "", 0, i+1
	if inline {
		v = val
		if strings.ContainsAny(v, "/\\.") || len(v) > keyRun {
			return LooksLikeKey(v), true, 0
		}
	}
	for ; j < len(args); j++ {
		x := args[j]
		if strings.HasPrefix(x, "-") && len(x) > 1 {
			break
		}
		w := x
		if v != "" {
			w = v + " " + x
		}
		if v != "" && len(w) > keyRun {
			break
		}
		if strings.ContainsAny(x, "/\\.") {
			if !LooksLikeKey(v) && LooksLikeKey(w) {
				return true, inline, n + 1
			}
			break
		}
		v, n = w, n+1
	}
	return LooksLikeKey(v), inline, n
}

// keyRun is the most characters after a key file flag read as one value:
// far past any key written with separators (an SLH-DSA-256s secret key is
// 128 bytes, 383 characters as colon hex). api.js uses the same.
const keyRun = 512

// LooksLikeKey reports whether s, given where a path goes, is a key: 32
// or more hex digits in a row once the separators a key is often printed
// with are taken out (colons, spaces, dashes, a dot at the end), and no /
// or \ or other dot in it. The Wallet view's looksLikeKey is the same
// test (GUI-032, GUI/Red R36B).
func LooksLikeKey(s string) bool {
	t := strings.TrimRight(strings.TrimSpace(s), ".")
	if strings.ContainsAny(t, "/\\.") {
		return false
	}
	run := 0
	for _, c := range t {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			run++
			if run >= 32 {
				return true
			}
		case c == ':' || c == '-' || c == ' ' || c == '\t' || c == '\n' || c == '\r':
		default:
			run = 0
		}
	}
	return false
}

// command returns args' first word that is not a flag: the poo command.
func command(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// Redact returns args with each secret flag's value replaced by Hidden,
// and whether it replaced any.
func Redact(args []string) ([]string, bool) {
	if command(args) == "sim" {
		return args, false
	}
	var out []string
	for i := 0; i < len(args); i++ {
		secret, inline := secretFlag(args[i])
		more := 1
		if !secret {
			secret, inline, more = keyFileFlag(args, i)
		} else if inline {
			more = 0
		}
		if !secret {
			continue
		}
		if out == nil {
			out = append([]string(nil), args...)
		}
		if inline {
			name, _, _ := strings.Cut(args[i], "=")
			out[i] = name + "=" + Hidden
		}
		for ; more > 0 && i+1 < len(args); more-- {
			i++
			out[i] = Hidden
		}
	}
	if out == nil {
		return args, false
	}
	return out, true
}

// SecretOutput reports whether a command prints a secret, as the core
// declares it (cli.SecretOutput, D110): wallet seed prints the wallet's
// seed, and keygen the seed of the key it makes. The GUI does not keep
// such a job's output once it has been read (GUI-009 in
// security/gui/findings).
func SecretOutput(args []string) bool { return cli.SecretOutput(args) }
