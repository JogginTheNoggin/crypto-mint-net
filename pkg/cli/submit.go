package cli

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// submitCmd hands the node an entry or a layer made elsewhere, in the
// hex its API prints (an EntryReply's entry, a LayerReply's layer), so a
// tool that grinds or builds off the node has a command, as the GUI has
// the same call (GREEN-038, D143). The node checks it as it checks one
// from a peer: submitting adds no rule and skips none.
func submitCmd(args []string, out io.Writer) error {
	const usage = "usage: poo submit entry|layer [-series ID] -pool K | -serial N HEX | -f FILE | -"
	if len(args) == 0 {
		return errors.New(usage)
	}
	what := args[0]
	if what != "entry" && what != "layer" {
		return fmt.Errorf("poo submit: unknown %q; %s", what, usage)
	}
	fs := newFlagSet("submit "+what, out)
	c := addClient(fs)
	pf := addPool(fs)
	file := fs.String("f", "", "read the "+what+" in hex from this file, - for standard input")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	text, err := submitHex(out, *file, fs.Args(), usage)
	if err != nil {
		return err
	}
	b, err := hex.DecodeString(text)
	if err != nil {
		return fmt.Errorf("poo submit %s: the %s is not hex: %w", what, what, err)
	}
	// Decoded here so a malformed one is refused before the node is asked.
	var e *entry.Entry
	var l layer.Layer
	if what == "entry" {
		e, err = entry.Decode(b)
	} else {
		l, err = layer.Decode(b)
	}
	if err != nil {
		return fmt.Errorf("poo submit %s: %w", what, err)
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	pool, err := pf.resolve(fs, s)
	if err != nil {
		return err
	}
	ref := rpc.PoolRef{Series: s.ID(), Pool: pool}
	if what == "entry" {
		r, err := cl.SubmitEntry(ref, e)
		if err != nil {
			// Say which entry is missing, so the person knows what to
			// hand in first (GREEN-039).
			if strings.Contains(err.Error(), ledger.ErrUnknownParent.Error()) {
				return fmt.Errorf("%w: submit its parent %s first (poo build -bytes printed it on the node that made it), or let this node take the pool from a peer", err, e.Parent)
			}
			return err
		}
		r.Entry = ""
		return writeJSON(out, r)
	}
	r, err := cl.SubmitLayer(ref, l)
	if err != nil {
		return err
	}
	r.Layer = ""
	return writeJSON(out, r)
}

// submitHex returns the hex poo submit was given: the one argument, or,
// with -f FILE or the argument "-", a file's or standard input's whole
// text, trimmed. One argument may be at most 128 KiB on Linux, so an
// entry over 64 KiB is handed in through a file or standard input, as the
// GUI hands one of any size (RED-038, D144).
func submitHex(out io.Writer, file string, args []string, usage string) (string, error) {
	switch {
	case file != "" && len(args) == 0:
	case file == "" && len(args) == 1:
		if args[0] != "-" {
			return strings.TrimSpace(args[0]), nil
		}
		file = "-"
	default:
		return "", errors.New(usage)
	}
	var b []byte
	var err error
	if file == "-" {
		b, err = io.ReadAll(inputOf(out))
	} else {
		b, err = os.ReadFile(file)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
