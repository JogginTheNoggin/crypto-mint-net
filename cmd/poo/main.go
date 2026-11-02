// Command poo is the command-line tool for the Proof of Ownership full node.
// Every command lives in package cli, which the GUI runs too; this is only
// the process around it: arguments, standard streams, and the interrupt
// that stops a long-running command.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Main(ctx, os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})
	stop()
	os.Exit(code)
}
