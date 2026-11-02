package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
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
	if text, err = fromReply(text, what); err != nil {
		return fmt.Errorf("poo submit %s: %w", what, err)
	}
	b := make([]byte, hex.DecodedLen(len(text)))
	_, err = hex.Decode(b, text)
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
		// The hex goes to the node as it was read, now that it is known
		// to decode: a large entry is not encoded again and copied on the
		// way, and its decoded form is let go first (GUI ORANGE-012).
		parent := e.Parent
		b, e = nil, nil
		r, err := cl.SubmitEntryHex(ref, text)
		if err != nil {
			// Say which entry is missing, so the person knows what to
			// hand in first (GREEN-039).
			if strings.Contains(err.Error(), ledger.ErrUnknownParent.Error()) {
				return fmt.Errorf("%w: submit its parent %s first (poo build -bytes printed it on the node that made it), or let this node take the pool from a peer", err, parent)
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
func submitHex(out io.Writer, file string, args []string, usage string) ([]byte, error) {
	switch {
	case file != "" && len(args) == 0:
	case file == "" && len(args) == 1:
		if args[0] != "-" {
			return []byte(strings.TrimSpace(args[0])), nil
		}
		file = "-"
	default:
		return nil, errors.New(usage)
	}
	var b []byte
	var err error
	if file == "-" {
		b, err = io.ReadAll(inputOf(out))
	} else {
		b, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(b), nil
}

// fromReply takes the hex out of a reply of poo build -bytes or poo work
// -bytes, so one can be piped into poo submit as it is printed
// (GREEN-040): an entry is the reply's "entry"; a layer is a work reply's
// "layer", or a build reply's "layer"."layer". Text that is not a JSON
// object is returned as it is.
func fromReply(text []byte, what string) ([]byte, error) {
	if !bytes.HasPrefix(text, []byte("{")) {
		return text, nil
	}
	var r map[string]json.RawMessage
	if err := json.Unmarshal(text, &r); err != nil {
		return nil, fmt.Errorf("the input looks like a reply but is not JSON: %w", err)
	}
	var s string
	raw, ok := r[what]
	if ok && json.Unmarshal(raw, &s) == nil && s != "" {
		return []byte(s), nil
	}
	if what == "layer" && ok {
		var nested struct {
			Layer string `json:"layer"`
		}
		if json.Unmarshal(raw, &nested) == nil && nested.Layer != "" {
			return []byte(nested.Layer), nil
		}
	}
	return nil, fmt.Errorf("the reply holds no %s bytes: make it with -bytes", what)
}
