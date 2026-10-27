package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSimCommand(t *testing.T) {
	var load struct {
		Agree, Conserve bool
		Settled         int
	}
	cli(t, &load, "sim", "load", "-nodes", "6", "-duration", "10000")
	if !load.Agree || !load.Conserve || load.Settled == 0 {
		t.Fatalf("load %+v", load)
	}
	var parts []struct{ Agree bool }
	cli(t, &parts, "sim", "partition", "-nodes", "6", "-runs", "2", "-split", "10000")
	if len(parts) != 2 || !parts[0].Agree || !parts[1].Agree {
		t.Fatalf("partition %+v", parts)
	}
	var atk struct{ Released int }
	cli(t, &atk, "sim", "attack", "-nodes", "6", "-runs", "2", "-give-up", "5000")
	if atk.Released == 0 {
		t.Fatalf("attack %+v", atk)
	}
	var rows []struct{ Pools uint64 }
	cli(t, &rows, "sim", "sizing", "-nodes", "6", "-sizes", "4,8", "-duration", "5000", "-runs", "1", "-give-up", "5000")
	if len(rows) != 2 || rows[0].Pools != 2 || rows[1].Pools != 1 {
		t.Fatalf("sizing %+v", rows)
	}
	var out bytes.Buffer
	if err := run([]string{"sim", "nope"}, &out); err == nil || !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("unknown scenario: %v", err)
	}
	if err := run([]string{"sim", "sizing", "-sizes", "0"}, &out); err == nil {
		t.Fatal("pool size 0 accepted")
	}
}
