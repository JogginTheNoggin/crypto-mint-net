package node_test

import (
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// intFields calls f with the path and value of every integer field of v,
// a struct, its nested structs and the structs its pointers point to
// (allocated when nil).
func intFields(path string, v reflect.Value, f func(string, reflect.Value)) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := v.Field(i)
		name := path + sf.Name
		if fv.Kind() == reflect.Pointer {
			if fv.Type().Elem().Kind() != reflect.Struct && !isInt(fv.Type().Elem().Kind()) {
				continue
			}
			if fv.IsNil() {
				fv.Set(reflect.New(fv.Type().Elem()))
			}
			fv = fv.Elem()
		}
		switch {
		case fv.Kind() == reflect.Struct:
			intFields(name+".", fv, f)
		case isInt(fv.Kind()):
			f(name, fv)
		}
	}
}

func isInt(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// RED-063, D160: every integer setting of a node policy at the largest
// value its type holds is refused by Validate, or the node opens with it,
// and its peer book and tracker start, without a panic. seen_size at
// 10^15 crashed the node at start.
func TestEveryIntegerSettingAtItsMaximum(t *testing.T) {
	var names []string
	probe := nodepolicy.Default()
	intFields("", reflect.ValueOf(&probe).Elem(), func(name string, _ reflect.Value) { names = append(names, name) })
	if len(names) < 30 {
		t.Fatalf("found only %d integer settings: %v", len(names), names)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			pol := nodepolicy.Default()
			pol.RPC.Listen = "off"
			intFields("", reflect.ValueOf(&pol).Elem(), func(n string, v reflect.Value) {
				if n != name {
					return
				}
				if v.CanInt() {
					v.SetInt(math.MaxInt64 >> (64 - v.Type().Bits()))
				} else {
					v.SetUint(math.MaxUint64 >> (64 - v.Type().Bits()))
				}
			})
			if pol.Validate() != nil {
				return
			}
			dir := t.TempDir()
			if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol); err != nil {
				if strings.Contains(err.Error(), "invalid") {
					return
				}
				t.Fatal(err)
			}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s at its maximum panics: %v", name, r)
				}
			}()
			n, err := node.Open(dir, node.Options{})
			if err != nil {
				return
			}
			defer n.Close()
			_ = p2p.FromPolicy(n.Policy())
			book, err := peers.Open("", n.Policy().BookLimits())
			if err != nil {
				t.Fatal(err)
			}
			book.Fit(time.Now())
			if _, err := tracking.New(n.Policy().TrackingConfig(), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
