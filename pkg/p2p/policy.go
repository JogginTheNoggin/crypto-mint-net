package p2p

import (
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// FromPolicy returns the host settings a node policy asks for. Settings the
// policy leaves out take the defaults when the host starts.
func FromPolicy(p nodepolicy.Policy) Config {
	c := Config{
		Listen:    p.P2PListenAddr(),
		Advertise: p.P2P.Advertise,
		Peers:     append([]string(nil), p.P2P.Peers...),
		MaxPeers:  p.P2P.MaxPeers,
		NoNotices: p.P2P.NoNotices,

		Registries:  append([]string(nil), p.P2P.Registries...),
		MaxOutbound: p.P2P.MaxOutbound,
		MaxInbound:  p.P2P.MaxInbound,
		NoRelay:     p.P2P.NoRelay,
		HopLimit:    p.P2P.HopLimit,
		RequestRate: p.P2P.RequestRate,

		AllowLocalPeers: p.P2P.AllowLocalPeers,
		NoDialImported:  p.P2P.NoDialImported,

		UnlistedConns: p.P2P.UnlistedConns,
		StrictReturn:  p.P2P.StrictReturn,
		NoConfirm:     p.P2P.NoConfirm,
	}
	if p.P2P.PrioritySlots > 0 || p.P2P.BackupSlots > 0 || p.P2P.Margin > 0 {
		c.Slots = peers.DefaultSlots()
		if p.P2P.PrioritySlots > 0 {
			c.Slots.Priority = p.P2P.PrioritySlots
		}
		if p.P2P.BackupSlots > 0 {
			c.Slots.Backup = p.P2P.BackupSlots
		}
		if p.P2P.Margin > 0 {
			c.Slots.Margin = p.P2P.Margin
		}
	}
	switch {
	case p.P2P.ReviewSeconds < 0:
		c.ReviewEvery = -1
	case p.P2P.ReviewSeconds > 0:
		c.ReviewEvery = time.Duration(p.P2P.ReviewSeconds) * time.Second
	}
	d := DefaultConfig().Limits
	c.Limits = wire.Limits{
		ChunkSize:  pick(p.P2P.ChunkSize, d.ChunkSize),
		EntryLimit: pick(p.P2P.EntryLimit, d.EntryLimit),
		HashLimit:  pick(p.P2P.HashLimit, d.HashLimit),
		LayerLimit: pick(p.P2P.LayerLimit, d.LayerLimit),
	}
	// The weight check is never off (D153): -1 or a value over the
	// most, as Load also reads them, is the longest interval.
	switch cs := p.P2P.CheckSeconds; {
	case cs < 0 || cs > nodepolicy.MaxCheckSeconds:
		c.CheckEvery = nodepolicy.MaxCheckSeconds * time.Second
	case cs > 0:
		c.CheckEvery = time.Duration(cs) * time.Second
	}
	switch {
	case p.P2P.PingSeconds < 0:
		c.PingEvery = -1
	case p.P2P.PingSeconds > 0:
		c.PingEvery = time.Duration(p.P2P.PingSeconds) * time.Second
	}
	return c
}

func pick(v, d uint32) uint32 {
	if v == 0 {
		return d
	}
	return v
}
