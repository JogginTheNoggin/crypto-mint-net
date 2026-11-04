package vectors

import (
	"encoding/hex"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// Variable are the vectors of a variable supply series (format version 2,
// PoO section 16.8, D163), kept beside the frozen version 1 vectors in
// testdata/variable.json: the series, its part ids and a release of part
// 2 by its issuer.
type Variable struct {
	SeriesEncoding        string `json:"series_encoding"`
	SeriesID              string `json:"series_id"`
	IssuerAddress         string `json:"issuer_address"`
	Part1ID               string `json:"part_1_id"`
	Part2ID               string `json:"part_2_id"`
	Part3ID               string `json:"part_3_id"`
	Part2ReleaseMessage   string `json:"part_2_release_message"`
	Part2ReleaseEncoding  string `json:"part_2_release_encoding"`
	Part2FirstSerial      uint64 `json:"part_2_first_serial"`
	Part2LastSerial       uint64 `json:"part_2_last_serial"`
	IssuerWitnessEncoding string `json:"issuer_witness_encoding"`
}

// VariableParams is the example series of the variable supply document:
// N = 50,000, M = 10,000, S = 1, P = 1,000, with the example series' hash,
// policy types and algorithms, issued by the testutil key "issuer".
func VariableParams() series.Params {
	p := testutil.ExampleParams()
	iss := testutil.NewKey(testutil.ExampleSeries(), "issuer").Address
	p.Mints, p.Supply, p.PoolSize = 50_000, 1, 1_000
	p.SupplyType, p.PartSize, p.Issuer = series.Variable, 10_000, &iss
	return p
}

// ComputeVariable builds the variable supply vectors.
func ComputeVariable() Variable {
	s := testutil.MustSeries(VariableParams())
	key := testutil.NewKey(s, "issuer")
	p1, _ := s.PartID(1)
	p2, _ := s.PartID(2)
	p3, _ := s.PartID(3)
	r, err := s.Unsigned(2)
	if err != nil {
		panic(err)
	}
	r.Witness = policy.SingleKeyWitness(key.Signer, s.ReleaseMessage(p2))
	if err := s.CheckRelease(r); err != nil {
		panic(err)
	}
	first, last, _ := s.PartRange(2)
	iw := policy.SingleKeyWitness(key.Signer, s.IssuerMessageTID())
	return Variable{
		SeriesEncoding:        hex.EncodeToString(s.Params().Encode()),
		SeriesID:              s.ID().String(),
		IssuerAddress:         key.Address.String(),
		Part1ID:               p1.String(),
		Part2ID:               p2.String(),
		Part3ID:               p3.String(),
		Part2ReleaseMessage:   s.ReleaseMessage(p2).String(),
		Part2ReleaseEncoding:  hex.EncodeToString(r.Encode()),
		Part2FirstSerial:      first,
		Part2LastSerial:       last,
		IssuerWitnessEncoding: witnessHex(iw),
	}
}

func witnessHex(w policy.Witness) string {
	return hex.EncodeToString(encodeWitness(w))
}
