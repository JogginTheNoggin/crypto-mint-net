// Package logging makes the node's logger: the standard library's
// log/slog, with a level and one of three formats chosen in the node
// policy's log section (GREEN-006).
//
//   - plain (the default) is one line per event as the node always wrote
//     them: the component as a prefix ("p2p: "), the message, then
//     key=value fields, with no time; warnings and errors say so.
//   - text is slog's key=value form with time and level, for log files.
//   - json is one JSON object per line, for log collectors.
//
// The consensus packages never log; they return errors, and the node,
// p2p and daemon packages that call them log what matters.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// Formats.
const (
	Plain = "plain"
	Text  = "text"
	JSON  = "json"
)

// Component is the attribute naming the part of the node an event comes
// from ("p2p", "node", "rpc"); plain prints it as a prefix.
const Component = "component"

// ErrInvalid reports an unknown level or format.
var ErrInvalid = errors.New("logging: invalid setting")

// ParseLevel reads debug, info (the default for ""), warn or error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%w: level %q (debug, info, warn or error)", ErrInvalid, s)
}

// Check reports whether level and format are known; empty means the
// default.
func Check(level, format string) error {
	if _, err := ParseLevel(level); err != nil {
		return err
	}
	switch format {
	case "", Plain, Text, JSON:
		return nil
	}
	return fmt.Errorf("%w: format %q (plain, text or json)", ErrInvalid, format)
}

// New returns a logger writing to w. A nil w discards everything.
func New(w io.Writer, level, format string) (*slog.Logger, error) {
	if err := Check(level, format); err != nil {
		return nil, err
	}
	if w == nil {
		return slog.New(slog.DiscardHandler), nil
	}
	lv, _ := ParseLevel(level)
	opts := &slog.HandlerOptions{Level: lv}
	switch format {
	case Text:
		return slog.New(slog.NewTextHandler(w, opts)), nil
	case JSON:
		return slog.New(slog.NewJSONHandler(w, opts)), nil
	}
	return slog.New(&plain{w: w, level: lv, mu: &sync.Mutex{}}), nil
}

// Discard returns a logger that writes nothing.
func Discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// plain is the default handler.
type plain struct {
	w      io.Writer
	level  slog.Level
	mu     *sync.Mutex // shared by the handlers derived from one
	prefix string      // from the component attribute
	attrs  string      // preformatted " k=v" pairs from With
	group  string      // key prefix from WithGroup
}

func (h *plain) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *plain) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.prefix)
	switch {
	case r.Level >= slog.LevelError:
		b.WriteString("error: ")
	case r.Level >= slog.LevelWarn:
		b.WriteString("warning: ")
	}
	b.WriteString(r.Message)
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, h.group, a)
		return true
	})
	b.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *plain) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range as {
		if a.Key == Component && h.group == "" {
			c.prefix = a.Value.String() + ": "
			continue
		}
		appendAttr(&b, h.group, a)
	}
	c.attrs = b.String()
	return &c
}

func (h *plain) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.group = h.group + name + "."
	return &c
}

func appendAttr(b *strings.Builder, group string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		g := group
		if a.Key != "" {
			g += a.Key + "."
		}
		for _, x := range a.Value.Group() {
			appendAttr(b, g, x)
		}
		return
	}
	v := a.Value.String()
	if v == "" || strings.ContainsAny(v, " \t\n\"=") {
		v = fmt.Sprintf("%q", v)
	}
	fmt.Fprintf(b, " %s%s=%s", group, a.Key, v)
}
