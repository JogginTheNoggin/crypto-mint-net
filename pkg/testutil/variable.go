package testutil

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// VariableParams returns the example series made variable (D163): n
// mints in part 1, parts of m mints after it, pools of p, issued by the
// key "issuer".
func VariableParams(n, p, m uint64) series.Params {
	pr := ExampleParams()
	iss := NewKey(ExampleSeries(), "issuer").Address
	pr.Mints, pr.PoolSize, pr.Issuer = n, p, &iss
	pr.SupplyType, pr.PartSize = series.Variable, m
	return pr
}

// IssuerWitness returns the issuer witness of a series issued by the key
// "issuer", which releases part 1 of a variable series.
func IssuerWitness(s *series.Series) policy.Witness {
	return policy.SingleKeyWitness(NewKey(s, "issuer").Signer, s.IssuerMessageTID())
}

// Release returns the release of part k of s signed by the key "issuer".
func Release(s *series.Series, k uint64) series.Release {
	r, err := s.Unsigned(k)
	if err != nil {
		panic(err)
	}
	r.Witness = policy.SingleKeyWitness(NewKey(s, "issuer").Signer, s.ReleaseMessage(r.ID(s.Hash())))
	return r
}
