package p2p

import (
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
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

		Registries:     append([]string(nil), p.P2P.Registries...),
		PeersPerPolicy: p.P2P.PeersPerPolicy,
		MaxOutbound:    p.P2P.MaxOutbound,
		NoRelay:        p.P2P.NoRelay,
		HopLimit:       p.P2P.HopLimit,
		RequestRate:    p.P2P.RequestRate,
	}
	d := DefaultConfig().Limits
	c.Limits = wire.Limits{
		ChunkSize:  pick(p.P2P.ChunkSize, d.ChunkSize),
		EntryLimit: pick(p.P2P.EntryLimit, d.EntryLimit),
		HashLimit:  pick(p.P2P.HashLimit, d.HashLimit),
		LayerLimit: pick(p.P2P.LayerLimit, d.LayerLimit),
	}
	switch {
	case p.P2P.CheckSeconds < 0:
		c.CheckEvery = -1
	case p.P2P.CheckSeconds > 0:
		c.CheckEvery = time.Duration(p.P2P.CheckSeconds) * time.Second
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
