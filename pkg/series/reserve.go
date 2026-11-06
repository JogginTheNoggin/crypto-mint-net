package series

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// The default reserve series (D170), adopted by jerry on 2026-11-04 from
// the "Reserve Currency Default Series" document: what poon init follows
// when it is given no series. It is node policy, not a consensus rule:
// any node may follow any series, and every value below is inside the
// series id, so none can change.
const (
	// ReserveMaxWitness is the reserve series' witness ceiling, 128 KiB,
	// and the ceiling poon series new and poon crypto use when none is given.
	ReserveMaxWitness = 128 * 1024

	// ReserveID is the reserve series' id, pinned: a test checks that
	// Reserve() still builds it, so the series can never change by
	// accident, and poon names this id "the built-in reserve series" only
	// when it matches in full (look-alike ids share a prefix).
	ReserveID = "45772e75a4670fd7c37dd66898550c2e657fb2c35b547335be2ce7a64466f4c5"
)

// Reserve returns the default reserve series' params: a fixed supply of
// 32,768 mints of 10^14 units in pools of 32 (1,024 pools), SHA-512/256,
// entry floor 20 bits and layer floor 24 bits with no entry credit,
// threshold and hashlock policies, Ed25519, ML-DSA-65, ML-DSA-87,
// SLH-DSA-SHA2-128s and SLH-DSA-SHA2-256s keys, a 128 KiB witness
// ceiling and no issuer.
func Reserve() Params {
	return Params{
		HashID:           hash.SHA512_256,
		Mints:            32768,
		Supply:           100_000_000_000_000,
		PoolSize:         32,
		MinLayerStrength: 24,
		MinEntryStrength: 20,
		PolicyTypes:      []policy.TypeID{policy.TypeThreshold, policy.TypeHashlock},
		KeyAlgs: []policy.AlgID{
			policy.AlgEd25519, policy.AlgMLDSA65, policy.AlgMLDSA87,
			policy.AlgSLHDSASHA2128s, policy.AlgSLHDSASHA2256s,
		},
		MaxWitness: ReserveMaxWitness,
	}
}

// IsReserve reports whether id is the built-in reserve series' id, in full.
func IsReserve(id hash.Hash) bool { return id.String() == ReserveID }
