package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// View-only mode (GREEN-007 part 2, decision D101): an invocation that
// only looks. It is on when Stdio.ViewOnly is set, when $POO_VIEW_ONLY is
// 1 or true, or when the arguments start with -view-only. Then every
// command that talks to the node dials with the view token
// (rpc.view-token), so the node itself refuses anything but reads, and
// every command not listed in readCommands is refused before it runs, so
// nothing is written locally either.

// ViewOnlyEnv is the environment variable that turns view-only mode on.
const ViewOnlyEnv = "POO_VIEW_ONLY"

// readCommands are the commands that only read: from the node, from local
// files, or nothing at all. A command maps to true when all of it reads,
// or to the subcommands that read. Anything not listed changes something
// and is refused in view-only mode, so a new command is refused until it
// is added here (TestViewOnlyTable checks every command is classified).
var readCommands = map[string]map[string]bool{
	"version": nil, "help": nil, "-h": nil, "--help": nil,
	"crypto": nil, "keygen": nil, "series": nil, "sim": nil, "demo": nil,
	"completion": nil, "__complete": nil, "doctor": nil,
	"status": nil, "tracking": nil, "chases": nil,
	// poo peers with its filters only looks; export writes a file and
	// import changes the book (D157). "-" stands for a first argument
	// that is a flag.
	"peers":   {"-": true},
	"payment": nil, "holdings": nil,
	"pools": nil, "ledger": nil, "pending": nil, "unspent": nil, "tx": nil, "address": nil,
	"watch":  nil,
	"follow": nil, // without flags it only shows; followCmd refuses a change
	"policy": {"list": true, "limits": true},
	// Grinding: devices, bench and jobs only look; start, run and stop
	// change what the node does. schedule only shows without add, remove
	// or set, and refuses those itself.
	"grind": {"devices": true, "bench": true, "jobs": true, "schedule": true, "help": true},
	// The wallet reads only these: the others sign, give out addresses or
	// record payment states in the wallet file, and seed reveals the seed.
	"wallet": {"addresses": true, "balance": true, "help": true},
}

// writeCommands are the commands that change something, listed so the
// table test can tell a forgotten command from a refused one.
var writeCommands = []string{
	"init", "node", "unfollow", "reload", "claim", "pay", "build", "work", "submit", "peer",
	// settings export writes a file, as policy export does, and import
	// changes node.json (D155).
	"settings",
}

// viewOnlyOf reports whether the invocation writing to out is view-only.
func viewOnlyOf(out io.Writer) bool {
	s, ok := out.(*stdio)
	return ok && s.viewOnly
}

// viewOnlyEnv reports whether $POO_VIEW_ONLY turns view-only mode on.
func viewOnlyEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ViewOnlyEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ErrViewOnly is what a command refused in view-only mode returns,
// wrapped.
var ErrViewOnly = errors.New("view-only")

// errViewOnly is the error for a command that would change something.
func errViewOnly(what string) error {
	return fmt.Errorf("%w: poo %s changes things", ErrViewOnly, what)
}

// ReloadsOnHangup reports whether the command line args (without "poo")
// runs a node, which reloads its node policy on SIGHUP; every other
// command should stop on it, as on an interrupt.
func ReloadsOnHangup(args []string) bool {
	for _, a := range args {
		if a == "-view-only" || a == "--view-only" {
			continue
		}
		return a == "node"
	}
	return false
}

// checkViewOnly refuses args unless they only read.
func checkViewOnly(args []string) error {
	if len(args) == 0 {
		return nil
	}
	subs, ok := readCommands[args[0]]
	if !ok {
		return errViewOnly(args[0])
	}
	if subs == nil {
		return nil
	}
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	if sub == "" || subs[sub] || (strings.HasPrefix(sub, "-") && subs["-"]) {
		return nil // no subcommand: the command prints its usage
	}
	return errViewOnly(args[0] + " " + sub)
}

// dialNode dials the node in dir with the token the invocation may use:
// the view token in view-only mode, else the full token.
func dialNode(out io.Writer, dir string) (*rpc.Client, error) {
	if viewOnlyOf(out) {
		return rpc.DialView(dir)
	}
	return rpc.Dial(dir)
}

// readSecret reads a passphrase or seed for the invocation writing to
// out: through Stdio.Secret when the caller gave one (a terminal that
// hides what is typed, a GUI's masked field), else as a line of input
// after printing prompt.
func readSecret(out io.Writer, prompt string) (string, error) {
	if s, ok := out.(*stdio); ok && s.secret != nil {
		return s.secret(prompt)
	}
	fmt.Fprint(out, prompt)
	line, err := inputOf(out).ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", errNoInput
	}
	return strings.TrimRight(line, "\r\n"), nil
}

var errNoInput = errors.New("nothing on standard input")

// readNewSecret reads a new passphrase as readSecret does. When it is
// typed, through Stdio.Secret (a terminal that hides it, or the GUI's
// masked field), it is asked for again and refused unless the two
// match, so a typo never seals the seed under a passphrase nobody knows
// (GUI-020, D145). A line piped on standard input is read once, as a
// script sends it.
func readNewSecret(out io.Writer, prompt string) (string, error) {
	p, err := readSecret(out, prompt)
	if err != nil {
		return "", err
	}
	if s, ok := out.(*stdio); ok && s.secret != nil {
		again, err := s.secret(strings.TrimSuffix(prompt, ": ") + " again: ")
		if err != nil {
			return "", err
		}
		if again != p {
			return "", errors.New("the two new passphrases differ, so nothing was changed")
		}
	}
	return p, nil
}

// SecretFlags returns the names of the flags, in any command, whose value
// can be a secret (a seed), so a front end that keeps or shows command
// lines, such as the GUI's terminal history, can mask their values
// (GUI-006). Passphrases never come from a flag.
func SecretFlags() []string { return []string{"seed", "restore"} }

// secretOutputs are the commands that print a secret: keygen prints the
// new key's seed, wallet seed the wallet's.
var secretOutputs = map[string]map[string]bool{"keygen": nil, "wallet": {"seed": true}}

// SecretOutput reports whether the command line args (without "poo")
// prints a secret, so a front end that keeps output, such as the GUI's
// terminal and job list, can hide it (GUI-009).
func SecretOutput(args []string) bool {
	var words []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		return false
	}
	subs, ok := secretOutputs[words[0]]
	if !ok {
		return false
	}
	return subs == nil || (len(words) > 1 && subs[words[1]])
}

// Error codes, stable whatever the wording of the error (GREEN-022).
const (
	CodeNoNode   = "no_node"   // no node runs where the command looked
	CodeViewOnly = "view_only" // view-only mode refused the command
	CodeNoInput  = "no_input"  // a passphrase or seed was needed and none came
	CodeError    = "error"     // anything else
)

// ErrorCode returns a short code for an error a command returned, for a
// front end to match instead of the message: the node's own code (the
// rpc.Code* values, such as not_found or invalid), or one of the Code*
// values above.
func ErrorCode(err error) string {
	var re *rpc.Error
	var ne *net.OpError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &re):
		return re.Code
	case errors.Is(err, rpc.ErrNotRunning), errors.As(err, &ne) && ne.Op == "dial":
		return CodeNoNode
	case errors.Is(err, ErrViewOnly):
		return CodeViewOnly
	case errors.Is(err, errNoInput):
		return CodeNoInput
	}
	return CodeError
}
