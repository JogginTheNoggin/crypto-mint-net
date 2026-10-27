package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
)

func TestPlain(t *testing.T) {
	var b bytes.Buffer
	l, err := logging.New(&b, "", "")
	if err != nil {
		t.Fatal(err)
	}
	p := l.With(logging.Component, "p2p")
	p.Info("connected", "peer", "ab12", "outbound", true)
	p.Debug("hidden at info")
	p.Warn("registry failed", "location", "https://x", "err", errors.New("no route to host"))
	p.WithGroup("chase").Info("done", "result", "confirmed")
	l.Error("stopped", "why", "")
	want := "p2p: connected peer=ab12 outbound=true\n" +
		"p2p: warning: registry failed location=https://x err=\"no route to host\"\n" +
		"p2p: done chase.result=confirmed\n" +
		"error: stopped why=\"\"\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestLevelsAndFormats(t *testing.T) {
	var b bytes.Buffer
	l, _ := logging.New(&b, "debug", logging.JSON)
	l.With(logging.Component, "node").Debug("compacted", "pool", "ab/1", "from", 10, "to", 4)
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	if m["level"] != "DEBUG" || m["component"] != "node" || m["to"] != 4.0 {
		t.Fatalf("%v", m)
	}
	b.Reset()
	l, _ = logging.New(&b, "warn", logging.Text)
	l.Info("quiet")
	l.Warn("loud", "k", 1)
	if s := b.String(); strings.Contains(s, "quiet") || !strings.Contains(s, "level=WARN msg=loud k=1") {
		t.Fatalf("%q", s)
	}
	for _, c := range [][2]string{{"verbose", ""}, {"", "xml"}} {
		if _, err := logging.New(&b, c[0], c[1]); !errors.Is(err, logging.ErrInvalid) {
			t.Errorf("%v: %v", c, err)
		}
	}
	if lv, _ := logging.ParseLevel("WARNING"); lv != slog.LevelWarn {
		t.Error(lv)
	}
	logging.Discard().Error("nothing")
	if l, _ := logging.New(nil, "", ""); l.Enabled(context.Background(), slog.LevelError) {
		t.Error("a nil writer logs")
	}
}
