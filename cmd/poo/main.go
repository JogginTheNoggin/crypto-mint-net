// Command poo is the command-line tool for the Proof of Ownership full node.
// Every command lives in package cli, which the GUI runs too; this is only
// the process around it: arguments, standard streams, the interrupt that
// stops a long-running command, and reading secrets from a terminal with
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
