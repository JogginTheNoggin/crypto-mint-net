package shell

import "strings"

// SecretFlags are the poo flags whose value is a secret: wallet create
// -restore and keygen -seed take a seed, which restores every key, and
// node -seed is refused but would still carry one. The GUI keeps a line
// holding one of them out of the terminal's saved history and hides the
// value wherever it shows a job's command line (GUI-006 in
// security/gui/findings). sim -seed is a random-number seed, not a secret.
var SecretFlags = []string{"restore", "seed"}

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
		if !secret {
			continue
		}
		if out == nil {
			out = append([]string(nil), args...)
		}
		if inline {
			name, _, _ := strings.Cut(args[i], "=")
			out[i] = name + "=" + Hidden
		} else if i+1 < len(args) {
			i++
			out[i] = Hidden
		}
	}
	if out == nil {
		return args, false
	}
	return out, true
}
