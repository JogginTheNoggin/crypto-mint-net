package wire_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-099: a hello in the layout before D186 is still refused (D186), but
// the error names it, so the log tells its owner to upgrade.
func TestBlue034OldHelloNamed(t *testing.T) {
	old := encoding.NewWriter(64).U16(1).Raw(bytes.Repeat([]byte{7}, 32)).VarBytes([]byte("poon")).Bytes()
	_, err := wire.DecodeHello(old)
	if !errors.Is(err, wire.ErrMalformed) || !errors.Is(err, wire.ErrOldHello) {
		t.Fatalf("old hello: %v", err)
	}
	// Other malformed hellos are not called old.
	for _, b := range [][]byte{old[:20], append(wire.Hello{Protocol: 1}.Encode(), 0)} {
		if _, err := wire.DecodeHello(b); !errors.Is(err, wire.ErrMalformed) || errors.Is(err, wire.ErrOldHello) {
			t.Fatalf("%x: %v", b, err)
		}
	}
	if _, err := wire.DecodeHello(wire.Hello{Protocol: 1, Agent: "poon"}.Encode()); err != nil {
		t.Fatal(err)
	}
}
