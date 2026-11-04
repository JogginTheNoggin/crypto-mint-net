package vectors

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

func encodeWitness(w policy.Witness) []byte {
	enc := encoding.NewWriter(w.Size())
	w.Encode(enc)
	return enc.Bytes()
}
