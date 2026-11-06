// Command poon is the command-line tool for the Proof of Ownership full node.
// Every command lives in package cli, which the GUI runs too; this is only
// the process around it: arguments, standard streams, the interrupt that
// stops a long-running command (an interrupt, a stop, or a hangup), and reading secrets from a terminal with
// echo off.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

func main() {
	// A hangup (a closed terminal or a GUI's terminal tab) stops a command
	// like an interrupt does, so a followed job is stopped, not left
	// running (GUI-010); poon node takes it as a reload instead. Under
	// nohup, which ignores hangups, it is left ignored (GUI-011).
	sigs := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if !cli.ReloadsOnHangup(os.Args[1:]) && !signal.Ignored(syscall.SIGHUP) {
		sigs = append(sigs, syscall.SIGHUP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), sigs...)
	s := cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		s.Secret = readHidden
	}
	code := cli.Main(ctx, os.Args[1:], s)
	stop()
	os.Exit(code)
}

// readHidden reads a passphrase or seed from the terminal without echoing
// it (decision D101). The prompt goes to standard error, so standard
// output stays the command's result.
func readHidden(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
