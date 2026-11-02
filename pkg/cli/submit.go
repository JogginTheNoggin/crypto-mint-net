package cli

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// submitCmd hands the node an entry or a layer made elsewhere, in the
// hex its API prints (an EntryReply's entry, a LayerReply's layer), so a
// tool that grinds or builds off the node has a command, as the GUI has
// the same call (GREEN-038, D143). The node checks it as it checks one
// from a peer: submitting adds no rule and skips none.
func submitCmd(args []string, out io.Writer) error {
	const usage = "usage: poo submit entry|layer [-series ID] -pool K | -serial N HEX"
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
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(usage)
	}
	b, err := hex.DecodeString(strings.TrimSpace(fs.Arg(0)))
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
