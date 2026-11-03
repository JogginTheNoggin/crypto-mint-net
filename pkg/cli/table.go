package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Table output for the read commands (GREEN-003). JSON stays the default,
// which scripts and the GUI read. -o table prints the same reply for a
// person: it is built from the JSON form, so its columns are the JSON
// field names, in order, and it can never show a field the JSON lacks.

// outputFlags are -o and -wide.
type outputFlags struct {
	format *string
	wide   *bool
}

func addOutput(fs *flag.FlagSet) *outputFlags {
	return &outputFlags{
		format: fs.String("o", "json", "output: json, or table for reading"),
		wide:   fs.Bool("wide", false, "with -o table, print hashes in full"),
	}
}

// check reports an unknown -o, before the command does any work.
func (o *outputFlags) check() error {
	if *o.format != "json" && *o.format != "table" {
		return fmt.Errorf("-o must be json or table, not %q", *o.format)
	}
	return nil
}

// write prints v in the chosen form.
func (o *outputFlags) write(out io.Writer, v any) error {
	if *o.format != "table" {
		return writeJSON(out, v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	n, err := decodeOrdered(json.NewDecoder(bytes.NewReader(b)))
	if err != nil {
		return err
	}
	t := tabler{wide: *o.wide}
	return t.top(out, n)
}

// jnode is a JSON value that keeps the order of an object's keys.
type jnode struct {
	keys  []string // object keys, in order (an object when vals != nil)
	vals  []*jnode
	items []*jnode // array items (an array when isArr)
	isArr bool
	isObj bool
	text  string // a scalar, as printed
	num   bool   // the scalar is a JSON number
}

func decodeOrdered(d *json.Decoder) (*jnode, error) {
	d.UseNumber()
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			n := &jnode{isObj: true}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				v, err := decodeOrdered(d)
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, k.(string))
				n.vals = append(n.vals, v)
			}
			_, err := d.Token()
			return n, err
		}
		n := &jnode{isArr: true}
		for d.More() {
			v, err := decodeOrdered(d)
			if err != nil {
				return nil, err
			}
			n.items = append(n.items, v)
		}
		_, err := d.Token()
		return n, err
	case nil:
		return &jnode{text: "-"}, nil
	case json.Number:
		return &jnode{text: t.String(), num: true}, nil
	default:
		return &jnode{text: fmt.Sprint(t)}, nil
	}
}

type tabler struct{ wide bool }

// top prints an array of objects as one table, and an object as its
// scalar fields, one per line, followed by a titled table for each of its
// arrays of objects.
func (t tabler) top(out io.Writer, n *jnode) error {
	switch {
	case n.isArr:
		return t.table(out, n.items)
	case n.isObj:
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		var subs []int
		for i, k := range n.keys {
			v := n.vals[i]
			if v.isArr && len(v.items) > 0 && v.items[0].isObj {
				subs = append(subs, i)
				continue
			}
			if v.isObj && !t.flat(v) {
				subs = append(subs, i)
				continue
			}
			if v.isObj && len(v.keys) == 0 {
				fmt.Fprintf(tw, "%s\t-\n", k)
				continue
			}
			if v.isObj {
				for j, kk := range v.keys {
					fmt.Fprintf(tw, "%s.%s\t%s\n", k, kk, t.cell(v.vals[j]))
				}
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\n", k, t.cell(v))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		for _, i := range subs {
			fmt.Fprintf(out, "\n%s:\n", n.keys[i])
			if err := t.top(out, n.vals[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := fmt.Fprintln(out, t.cell(n))
		return err
	}
}

// flat reports whether an object has only scalar fields, or arrays of
// scalars, so each field fits in one cell (GREEN-018).
func (t tabler) flat(n *jnode) bool {
	for _, v := range n.vals {
		if v.isObj {
			return false
		}
		if v.isArr && len(v.items) > 0 && (v.items[0].isObj || v.items[0].isArr) {
			return false
		}
	}
	return true
}

// maxFlatKeys is the widest nested object a table spreads into dotted
// columns (plan.device, plan.threads, ...); a wider one is counted.
const maxFlatKeys = 8

// table prints rows of objects under the union of their keys, first seen
// first. An object field becomes dotted columns (input.tid); an array
// field shows its items or, for objects, their count.
func (t tabler) table(out io.Writer, rows []*jnode) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(out, "(none)")
		return err
	}
	if !rows[0].isObj {
		for _, r := range rows {
			fmt.Fprintln(out, t.cell(r))
		}
		return nil
	}
	var cols []string
	seen := map[string]bool{}
	flatRows := make([]map[string]string, len(rows))
	for i, r := range rows {
		m := map[string]string{}
		t.flatten("", r, m, &cols, seen)
		flatRows[i] = m
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.ToUpper(strings.Join(cols, "\t")))
	for _, m := range flatRows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			if v, ok := m[c]; ok {
				vals[i] = v
			} else {
				vals[i] = "-"
			}
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
}

func (t tabler) flatten(prefix string, n *jnode, m map[string]string, cols *[]string, seen map[string]bool) {
	for i, k := range n.keys {
		name := prefix + k
		v := n.vals[i]
		if v.isObj && t.flat(v) && len(v.keys) <= maxFlatKeys {
			t.flatten(name+".", v, m, cols, seen)
			continue
		}
		if !seen[name] {
			seen[name] = true
			*cols = append(*cols, name)
		}
		m[name] = t.cell(v)
	}
}

// cell is a value in one cell: a hash shortened unless -wide, an array of
// scalars joined, an array of objects or a nested object counted.
func (t tabler) cell(n *jnode) string {
	switch {
	case n.isArr:
		if len(n.items) == 0 {
			return "-"
		}
		if n.items[0].isObj || n.items[0].isArr {
			return fmt.Sprintf("[%d]", len(n.items))
		}
		parts := make([]string, len(n.items))
		for i, it := range n.items {
			parts[i] = t.cell(it)
		}
		return strings.Join(parts, ",")
	case n.isObj:
		return fmt.Sprintf("{%d}", len(n.keys))
	}
	s := n.text
	if s == "" {
		return `""`
	}
	if t.wide {
		return s
	}
	if len(s) > 16 && isHex(s) {
		return s[:8] + ".." + s[len(s)-4:]
	}
	// A prefixed hash, such as the grinder's tx:TID target (GREEN-020).
	if i := strings.IndexByte(s, ':'); i > 0 && i <= 8 && len(s)-i-1 > 16 && isHex(s[i+1:]) {
		return s[:i+1] + s[i+1:i+9] + ".." + s[len(s)-4:]
	}
	if n.num {
		return roundNum(s)
	}
	// A time to the nanosecond reads as a second in a table (GREEN-020),
	// and Go's zero time, "never" in every reply that has one (a peer
	// not yet tried, a block not set), as "-" (GREEN-046).
	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		if ts.IsZero() {
			return "-"
		}
		if strings.Contains(s, ".") {
			return ts.Truncate(time.Second).Format(time.RFC3339)
		}
	}
	return s
}

// roundNum keeps at most two decimals of a JSON number with a fraction,
// such as a hash rate, so a column of rates stays readable; -wide and the
// JSON form keep every digit. Whole numbers, such as amounts, are never
// touched.
func roundNum(s string) string {
	dot := strings.IndexByte(s, '.')
	if dot < 0 || strings.ContainsAny(s, "eE") || len(s)-dot-1 <= 2 {
		return s
	}
	var f float64
	if _, err := fmt.Sscan(s, &f); err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func isHex(s string) bool {
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
