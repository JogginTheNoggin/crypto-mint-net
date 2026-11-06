package nodepolicy

import (
	"fmt"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
)

// Bounty is the node's part in the bounty market (D196). It is off by
// default. It changes only what this node offers, takes and pays, never
// what any node accepts into a ledger.
type Bounty struct {
	// Post lets this node post offers; Take lets it hold peers' offers
	// and grind them.
	Post bool `json:"post,omitempty"`
	Take bool `json:"take,omitempty"`
	// MaxOpen bounds this node's open offers (default 16, at most 256).
	MaxOpen int `json:"max_open,omitempty"`
	// MaxAmount bounds what one offer pays in all; Budget what all open
	// offers and unpaid layers owe together. Posting needs both.
	MaxAmount uint64 `json:"max_amount,omitempty"`
	Budget    uint64 `json:"budget,omitempty"`
	// MaxLeasesPerGrinder bounds the leases one grinder holds at once
	// (default 4, at most 64).
	MaxLeasesPerGrinder int `json:"max_leases_per_grinder,omitempty"`
	// LeaseFloorSeconds and LeaseCeilingSeconds bound a lease (defaults
	// 60 and 3600; 10 to 86400).
	LeaseFloorSeconds   int `json:"lease_floor_seconds,omitempty"`
	LeaseCeilingSeconds int `json:"lease_ceiling_seconds,omitempty"`
	// HeldOffers bounds the peers' offers kept (default 256, at most
	// 4096).
	HeldOffers int `json:"held_offers,omitempty"`
	// MinPrice: peers' offers paying less per layer are not kept.
	MinPrice uint64 `json:"min_price,omitempty"`
}

// Bounds of the bounty settings.
const (
	MaxBountyOpen     = 256
	MaxBountyLeases   = 64
	MinBountyLeaseSec = 10
	MaxBountyLeaseSec = 86400
	MaxBountyHeld     = 4096
)

func (b *Bounty) validate() error {
	if b == nil {
		return nil
	}
	switch {
	case b.MaxOpen < 0 || b.MaxOpen > MaxBountyOpen:
		return fmt.Errorf("max_open must be 0 to %d", MaxBountyOpen)
	case b.MaxLeasesPerGrinder < 0 || b.MaxLeasesPerGrinder > MaxBountyLeases:
		return fmt.Errorf("max_leases_per_grinder must be 0 to %d", MaxBountyLeases)
	case b.LeaseFloorSeconds != 0 && (b.LeaseFloorSeconds < MinBountyLeaseSec || b.LeaseFloorSeconds > MaxBountyLeaseSec):
		return fmt.Errorf("lease_floor_seconds must be %d to %d", MinBountyLeaseSec, MaxBountyLeaseSec)
	case b.LeaseCeilingSeconds != 0 && (b.LeaseCeilingSeconds < MinBountyLeaseSec || b.LeaseCeilingSeconds > MaxBountyLeaseSec):
		return fmt.Errorf("lease_ceiling_seconds must be %d to %d", MinBountyLeaseSec, MaxBountyLeaseSec)
	case b.LeaseFloorSeconds != 0 && b.LeaseCeilingSeconds != 0 && b.LeaseCeilingSeconds < b.LeaseFloorSeconds:
		return fmt.Errorf("lease_ceiling_seconds is under lease_floor_seconds")
	case b.HeldOffers < 0 || b.HeldOffers > MaxBountyHeld:
		return fmt.Errorf("held_offers must be 0 to %d", MaxBountyHeld)
	}
	return nil
}

// BountyConfig returns the market settings, and false when the policy
// neither posts nor takes.
func (p Policy) BountyConfig() (bounty.Config, bool) {
	b := p.Bounty
	if b == nil || !b.Post && !b.Take {
		return bounty.Config{}, false
	}
	return bounty.Config{
		Post: b.Post, Take: b.Take, MaxOpen: b.MaxOpen, MaxAmount: b.MaxAmount, Budget: b.Budget,
		MaxLeasesPerGrinder: b.MaxLeasesPerGrinder,
		LeaseFloor:          time.Duration(b.LeaseFloorSeconds) * time.Second,
		LeaseCeiling:        time.Duration(b.LeaseCeilingSeconds) * time.Second,
		Held:                b.HeldOffers, MinPrice: b.MinPrice,
	}, true
}
