package nodepolicy

import (
	"fmt"
	"math"
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

// bountyLimit returns the bounty setting named as a limit.
func (p *Policy) bountyLimit(name string) (int, bool) {
	var b Bounty
	if p.Bounty != nil {
		b = *p.Bounty
	}
	flag := func(on bool) int {
		if on {
			return 1
		}
		return 0
	}
	amount := func(v uint64) int { return int(min(v, math.MaxInt64)) }
	switch name {
	case BoundBountyPost.Name:
		return flag(b.Post), true
	case BoundBountyTake.Name:
		return flag(b.Take), true
	case BoundBountyMaxOpen.Name:
		return b.MaxOpen, true
	case BoundBountyMaxAmount.Name:
		return amount(b.MaxAmount), true
	case BoundBountyBudget.Name:
		return amount(b.Budget), true
	case BoundBountyMaxLeases.Name:
		return b.MaxLeasesPerGrinder, true
	case BoundBountyLeaseFloor.Name:
		return b.LeaseFloorSeconds, true
	case BoundBountyLeaseCeiling.Name:
		return b.LeaseCeilingSeconds, true
	case BoundBountyHeld.Name:
		return b.HeldOffers, true
	case BoundBountyMinPrice.Name:
		return amount(b.MinPrice), true
	}
	return 0, false
}

// setBountyLimit sets the bounty setting named as a limit. A section left
// with nothing set is removed, so node.json stays as it was.
func (p *Policy) setBountyLimit(name string, v int) error {
	if _, ok := p.bountyLimit(name); !ok {
		return fmt.Errorf("%w: %q is not a settable limit (poo policy limits lists them)", ErrInvalid, name)
	}
	switch {
	case bountyBools[name] && v != 0 && v != 1:
		return fmt.Errorf("%w: %s is 1 (on) or 0 (off), not %d", ErrInvalid, name, v)
	case v < 0:
		return fmt.Errorf("%w: %s is %d; it is never negative", ErrInvalid, name, v)
	}
	if p.Bounty == nil {
		p.Bounty = &Bounty{}
	}
	b := p.Bounty
	switch name {
	case BoundBountyPost.Name:
		b.Post = v == 1
	case BoundBountyTake.Name:
		b.Take = v == 1
	case BoundBountyMaxOpen.Name:
		b.MaxOpen = v
	case BoundBountyMaxAmount.Name:
		b.MaxAmount = uint64(v)
	case BoundBountyBudget.Name:
		b.Budget = uint64(v)
	case BoundBountyMaxLeases.Name:
		b.MaxLeasesPerGrinder = v
	case BoundBountyLeaseFloor.Name:
		b.LeaseFloorSeconds = v
	case BoundBountyLeaseCeiling.Name:
		b.LeaseCeilingSeconds = v
	case BoundBountyHeld.Name:
		b.HeldOffers = v
	case BoundBountyMinPrice.Name:
		b.MinPrice = uint64(v)
	}
	if *b == (Bounty{}) {
		p.Bounty = nil
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

// BountyNotes says what in the bounty settings will not work as a person
// most likely meant (GREEN-062): posting turned on with no spending
// limits, which refuses every offer, and one offer allowed more than the
// whole budget. poo policy limits -set shows them after a change.
func (p Policy) BountyNotes() []string {
	b := p.Bounty
	if b == nil {
		return nil
	}
	var notes []string
	if b.Post {
		switch {
		case b.MaxAmount == 0 && b.Budget == 0:
			notes = append(notes, "bounty.post is on, but bounty.max_amount and bounty.budget are not set, so no offer can be posted")
		case b.MaxAmount == 0:
			notes = append(notes, "bounty.post is on, but bounty.max_amount is not set, so no offer can be posted")
		case b.Budget == 0:
			notes = append(notes, "bounty.post is on, but bounty.budget is not set, so no offer can be posted")
		}
	}
	if b.Budget != 0 && b.MaxAmount > b.Budget {
		notes = append(notes, fmt.Sprintf("bounty.max_amount %d is over bounty.budget %d, so one offer can pay at most %d", b.MaxAmount, b.Budget, b.Budget))
	}
	return notes
}

// OnOff reports whether the bound is an on/off setting (1 on, 0 off),
// such as bounty.post, which poo policy limits -set also takes as on or
// off (GREEN-062).
func (b Bound) OnOff() bool { return b.Min == 1 && b.Max == 1 && b.Default == 0 }
