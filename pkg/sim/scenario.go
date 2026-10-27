package sim

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Net is the shape of a network for a scenario.
type Net struct {
	Nodes                  int  `json:"nodes"`
	Degree                 int  `json:"degree"`
	MinLatency, MaxLatency Time `json:"-"`
	// Builders is how many nodes build and work, spread evenly.
	Builders int `json:"builders"`
	// Tries is each builder's nonces per WorkEvery: its power.
	Tries     uint64 `json:"tries"`
	WorkEvery Time   `json:"work_every_ms"`
	// BuildEvery is how often a builder bundles pending transactions.
	BuildEvery Time `json:"build_every_ms"`
	// K is every node's top K (0: the default).
	K int `json:"k"`
}

// DefaultNet is a small busy network: 16 nodes, 4 builders.
func DefaultNet() Net {
	return Net{Nodes: 16, Degree: 2, MinLatency: 20, MaxLatency: 200, Builders: 4, Tries: 64, WorkEvery: 500, BuildEvery: 2 * Second}
}

func (n Net) sim(seed int64, s *series.Series) (*Sim, error) {
	cfg := Config{Seed: seed, Nodes: n.Nodes, Degree: n.Degree, MinLatency: n.MinLatency, MaxLatency: n.MaxLatency, Series: s}
	if n.K != 0 {
		ec := engine.DefaultConfig()
		ec.Ledger.K = n.K
		cfg.Engine = &ec
	}
	return New(cfg)
}

// builders starts the net's builders on every pool, skipping the nodes in
// skip.
func (n Net) builders(s *Sim, skip map[int]bool) []*Builder {
	var bs []*Builder
	if n.Builders == 0 {
		return nil
	}
	step := n.Nodes / n.Builders
	if step == 0 {
		step = 1
	}
	for i := 0; i < n.Nodes && len(bs) < n.Builders*int(s.s.PoolCount()); i += step {
		if skip[i] {
			continue
		}
		for k := uint64(0); k < s.s.PoolCount(); k++ {
			bs = append(bs, s.StartBuilder(i, k, n.BuildEvery, n.WorkEvery, n.Tries))
		}
	}
	return bs
}

func stopAll(bs []*Builder) {
	for _, b := range bs {
		b.Stop()
	}
}

// LoadReport is what a run under load measured.
type LoadReport struct {
	Seed     int64 `json:"seed"`
	Agree    bool  `json:"agree"`
	Conserve bool  `json:"conserve"`
	Sent     int   `json:"sent"`
	Refused  int   `json:"refused"`
	// Settled counts accepted payments selected at the end, and
	// MeanWait their mean time from submission to selection at the node
	// they were handed to, in ms.
	Settled  int     `json:"settled"`
	MeanWait float64 `json:"mean_wait_ms"`
	// SiblingShare is the share of layer credit on entries outside the
	// final selected ledgers: work the network spent on siblings.
	SiblingShare float64 `json:"sibling_share"`
	Stats        Stats   `json:"stats"`
}

// Load runs a network under random payments for d, stops, lets it settle
// and reports agreement, conservation, settlement and sibling work: the
// design's "order and agreement", "conservation" and "sibling entries
// under load" tests.
func Load(seed int64, n Net, s *series.Series, payEvery, d Time) (LoadReport, error) {
	sm, err := n.sim(seed, s)
	if err != nil {
		return LoadReport{}, err
	}
	w := NewWallets(sm, 2*n.Nodes)
	if err := w.ClaimAll(); err != nil {
		return LoadReport{}, err
	}
	bs := n.builders(sm, nil)
	type sent struct {
		node int
		tid  hash.Hash
		at   Time
		pool uint64
	}
	var all []sent
	stop := false
	sm.Every(payEvery, func() bool {
		if stop {
			return false
		}
		node := sm.rnd.Intn(n.Nodes)
		serial := uint64(sm.rnd.Int63n(int64(s.Mints())))
		if t, err := w.Pay(node, serial); err == nil && t != nil {
			all = append(all, sent{node, t.TID(s.Hash()), sm.now, s.PoolOf(serial)})
		}
		return true
	})
	// Watch when each payment is first selected at its node.
	first := map[hash.Hash]Time{}
	sm.Every(100, func() bool {
		for _, p := range all {
			if _, ok := first[p.tid]; ok {
				continue
			}
			if pay, err := sm.nodes[p.node].Eng.Payment(sm.Pool(p.pool), p.tid); err == nil && pay.Selected {
				first[p.tid] = sm.now
			}
		}
		return !stop
	})
	sm.Run(d)
	stop = true
	stopAll(bs)
	sm.Settle(60 * Second)
	r := LoadReport{Seed: seed, Agree: true, Conserve: true, Sent: w.Sent, Refused: w.Refused, Stats: sm.stats}
	for k := uint64(0); k < s.PoolCount(); k++ {
		r.Agree = r.Agree && sm.Agree(k)
	}
	for i := range sm.nodes {
		if sm.CheckConservation(i) != nil {
			r.Conserve = false
		}
	}
	var wait Time
	for _, p := range all {
		if pay, err := sm.nodes[0].Eng.Payment(sm.Pool(p.pool), p.tid); err == nil && pay.Selected {
			r.Settled++
			if t, ok := first[p.tid]; ok {
				wait += t - p.at
			}
		}
	}
	if r.Settled > 0 {
		r.MeanWait = float64(wait) / float64(r.Settled)
	}
	var sel, other weight.Weight
	for k := uint64(0); k < s.PoolCount(); k++ {
		a, b := sm.Work(0, k)
		sel, other = sel.Add(a), other.Add(b)
	}
	r.SiblingShare = share(other, sel.Add(other))
	return r, nil
}

// share returns a/b as a float, 0 when b is 0.
func share(a, b weight.Weight) float64 {
	if b.IsZero() {
		return 0
	}
	fa, fb := toFloat(a), toFloat(b)
	return fa / fb
}

func toFloat(w weight.Weight) float64 {
	var f float64
	for _, c := range w.Bytes() {
		f = f*256 + float64(c)
	}
	return f
}

// PartitionReport is what a partition and heal measured.
type PartitionReport struct {
	Seed int64 `json:"seed"`
	// SplitAgree: each side agreed within itself before the heal, and
	// the sides differed.
	SidesDiffered bool `json:"sides_differed"`
	// Winner is the side whose ledger every node selected after the
	// heal (0 or 1), or -1 when the nodes did not agree.
	Winner      int  `json:"winner"`
	HeavierSide int  `json:"heavier_side"`
	Agree       bool `json:"agree"`
	Conserve    bool `json:"conserve"`
	// Carried counts payments made on the losing side that were selected
	// after the heal and a rebuild; Lost those that were not. The sides
	// pay in different mints, so an uncontested payment is never lost.
	Carried int `json:"carried"`
	// Contested counts losing-side payments that spent a portion another
	// payment also spent (handed to another node), or spent what such a
	// payment made: not counted above.
	Contested int   `json:"contested"`
	Lost      int   `json:"lost"`
	Stats     Stats `json:"stats"`
}

// Partition splits the network in two for d while both sides build and
// pay in pool 1 (each side in its own mints), heals it, and reports
// which ledger won: the heavier one must, every node must agree on it,
// and the losing side's payments, which conflict with nothing, must be
// carried into the winner (the design's "switch correctness" and
// "displaced transactions" tests on a network).
func Partition(seed int64, n Net, s *series.Series, d Time) (PartitionReport, error) {
	sm, err := n.sim(seed, s)
	if err != nil {
		return PartitionReport{}, err
	}
	w := NewWallets(sm, 2*n.Nodes)
	if err := w.ClaimAll(); err != nil {
		return PartitionReport{}, err
	}
	bs := n.builders(sm, nil)
	sm.Run(10 * Second)
	var a, b []int
	for i := 0; i < n.Nodes; i++ {
		if i < n.Nodes/2 {
			a = append(a, i)
		} else {
			b = append(b, i)
		}
	}
	sm.Partition(a, b)
	type made struct {
		side int
		tid  hash.Hash
		pool uint64
		ins  []tx.Input
	}
	var all []made
	stop := false
	sm.Every(400, func() bool {
		if stop {
			return false
		}
		// Each side pays in its own half of pool 1's mints, so nothing
		// the losing side paid conflicts with the winner.
		node := sm.rnd.Intn(n.Nodes)
		side := 0
		if node >= n.Nodes/2 {
			side = 1
		}
		first, last := s.PoolRange(1)
		half := (last - first + 1) / 2
		serial := first + uint64(side)*half + uint64(sm.rnd.Int63n(int64(half)))
		if t, err := w.Pay(node, serial); err == nil && t != nil {
			all = append(all, made{side, t.TID(s.Hash()), 1, t.Inputs})
		}
		return true
	})
	sm.Run(d)
	stop = true
	r := PartitionReport{Seed: seed, Winner: -1}
	stopAll(bs)
	sm.Settle(5 * Second)
	ta, wa := sm.Tip(a[0], 1)
	tb, wb := sm.Tip(b[0], 1)
	r.SidesDiffered = ta != tb
	if wb.Greater(wa) {
		r.HeavierSide = 1
	}
	sm.Heal()
	sm.Settle(60 * Second)
	r.Agree = sm.Agree(1)
	if t, _ := sm.Tip(0, 1); r.Agree {
		switch t {
		case ta:
			r.Winner = 0
		case tb:
			r.Winner = 1
		default:
			// Built on after the heal by nobody (builders stopped), so a
			// tip other than either side's means a mixed result.
			r.Winner = -1
		}
	}
	r.Conserve = true
	for i := range sm.nodes {
		if sm.CheckConservation(i) != nil {
			r.Conserve = false
		}
	}
	// Payments of the losing side come back as pending at the nodes that
	// held them; build them into the winner, as builders would. A chain
	// of payments comes back one entry per link, so build in rounds.
	for round := 0; round < 64; round++ {
		built := false
		for i := range sm.nodes {
			if e := sm.nodes[i].Build(1); e != nil {
				sm.nodes[i].WorkOn(1, e.Hash(s), n.Tries)
				built = true
				break
			}
		}
		sm.Settle(10 * Second)
		if !built {
			break
		}
	}
	// Two payments handed to different nodes can spend the same portion:
	// a real double spend, of which at most one survives. Those are left
	// out of the count.
	spends := map[tx.Input]int{}
	for _, m := range all {
		for _, in := range m.ins {
			spends[in]++
		}
	}
	// So are payments that spend what a contested payment made.
	contested := map[hash.Hash]bool{}
	for _, m := range all { // in order made, so parents come first
		for _, in := range m.ins {
			if spends[in] > 1 || contested[in.TID] {
				contested[m.tid] = true
			}
		}
	}
	for _, m := range all {
		if m.side == r.Winner || m.pool != 1 {
			continue
		}
		if contested[m.tid] {
			r.Contested++
			continue
		}
		if pay, err := sm.nodes[0].Eng.Payment(sm.Pool(m.pool), m.tid); err == nil && pay.Selected {
			r.Carried++
		} else {
			r.Lost++
		}
	}
	r.Stats = sm.stats
	return r, nil
}

// AttackConfig sets up a double spend against a receiver.
type AttackConfig struct {
	Net Net
	// Share is the attacker's share of all the network's work, 0 to 1,
	// all of it put on the attacked pool.
	Share float64 `json:"share"`
	// Threshold is the from(e) the receiver waits for before releasing
	// the goods.
	Threshold weight.Weight `json:"threshold"`
	// HeadStart is how long the attacker grinds on its rival entry
	// before paying the receiver (pre-grinding at checkout).
	HeadStart Time `json:"head_start_ms"`
	// GiveUp is how long after the goods are released the attacker
	// keeps trying.
	GiveUp Time `json:"give_up_ms"`
}

// AttackReport is one attack's outcome.
type AttackReport struct {
	Seed int64 `json:"seed"`
	// Released: the receiver saw from(e) reach the threshold.
	Released bool `json:"released"`
	// ReleasedAfter is the time from payment to release, in ms.
	ReleasedAfter Time `json:"released_after_ms"`
	// Won: after the goods were released, the receiver's node selected
	// the rival and the payment was void.
	Won bool `json:"won"`
}

// Attack runs one double spend: the attacker (a private node with Share
// of the work) claims a mint, pays the receiver from it in public, and
// grinds in secret on a rival entry that pays itself instead, on the same
// parent. Once the goods are out and the rival is heavier at its own
// node, it publishes the rival and keeps grinding on it in public; it
// stops when the receiver's node drops the payment or at GiveUp. This is
// the design's "pre-grinding at checkout" test.
func Attack(seed int64, c AttackConfig, s *series.Series) (AttackReport, error) {
	if c.Share <= 0 || c.Share >= 1 {
		return AttackReport{}, errors.New("sim: attack share must be between 0 and 1")
	}
	n := c.Net
	n.Nodes++ // the attacker is the extra, last node
	sm, err := n.sim(seed, s)
	if err != nil {
		return AttackReport{}, err
	}
	atk := sm.nodes[n.Nodes-1]
	recv := 0
	keys := testutil.NewKeys(s)
	const serial = 4
	pool := s.PoolOf(serial)
	// The attacker claims the mint in public, and it settles.
	claim := keys.Claim(serial, testutil.P("attacker", s.Supply()))
	if err := sm.Submit(atk.ID, claim); err != nil {
		return AttackReport{}, err
	}
	bs := n.builders(sm, map[int]bool{atk.ID: true})
	sm.Run(10 * Second)
	parent, _ := sm.Tip(recv, pool)
	if ok, _ := locate(sm, recv, pool, claim.TID(s.Hash())); !ok {
		return AttackReport{}, errors.New("sim: the claim did not settle")
	}
	in := testutil.In{Input: tx.Input{TID: claim.TID(s.Hash())}, Owner: "attacker"}
	honest := keys.Spend(serial, []testutil.In{in}, testutil.P("receiver", s.Supply()))
	rivalTx := keys.Spend(serial, []testutil.In{in}, testutil.P("attacker-2", s.Supply()))
	// The attacker's rival entry, on the same parent, kept secret.
	atk.Private = true
	rival, err := atk.BuildOn(pool, parent, []*tx.Tx{rivalTx})
	if err != nil {
		return AttackReport{}, err
	}
	rh := rival.Hash(s)
	// The share is of all the network's work: an attacker can put all of
	// its power on the one pool it attacks, while honest builders spread
	// theirs over every pool.
	honestTries := n.Tries * uint64(n.Builders) * s.PoolCount()
	atkTries := uint64(float64(honestTries) * c.Share / (1 - c.Share))
	if atkTries == 0 {
		atkTries = 1
	}
	grinding := true
	sm.Every(n.WorkEvery, func() bool {
		if grinding {
			atk.WorkOn(pool, rh, atkTries)
		}
		return grinding
	})
	sm.Run(c.HeadStart)
	// Pay the receiver in public.
	r := AttackReport{Seed: seed}
	paidAt := sm.now
	if err := sm.Submit(recv, honest); err != nil {
		return r, err
	}
	htid := honest.TID(s.Hash())
	var releasedAt Time
	done := false
	sm.Every(100, func() bool {
		if done {
			return false
		}
		if !r.Released {
			pay, err := sm.nodes[recv].Eng.Payment(sm.Pool(pool), htid)
			if err == nil && pay.Selected && !c.Threshold.Greater(pay.From) {
				r.Released, releasedAt = true, sm.now
				r.ReleasedAfter = sm.now - paidAt
			}
			return true
		}
		// Goods are out: publish the rival as soon as it is heavier at
		// the attacker's own node (which then selects it), and keep
		// grinding on it in public until the receiver's node drops the
		// payment or the attacker gives up.
		if atk.Private {
			if t, _ := sm.Tip(atk.ID, pool); t == rh {
				atk.Release()
			}
		} else if pay, err := sm.nodes[recv].Eng.Payment(sm.Pool(pool), htid); err != nil || !pay.Selected {
			grinding, done = false, true
			return false
		}
		if sm.now-releasedAt > c.GiveUp {
			grinding, done = false, true
			return false
		}
		return true
	})
	for limit := sm.now + 10*60*Second + c.GiveUp; !done && sm.now < limit; {
		sm.Run(Second)
	}
	grinding, done = false, true
	stopAll(bs)
	sm.Settle(60 * Second)
	if r.Released {
		pay, err := sm.nodes[recv].Eng.Payment(sm.Pool(pool), htid)
		r.Won = err != nil || !pay.Selected
	}
	return r, nil
}

func locate(sm *Sim, i int, pool uint64, tid hash.Hash) (bool, hash.Hash) {
	var h hash.Hash
	var ok bool
	sm.nodes[i].Eng.Ledger(sm.Pool(pool), func(p *ledger.Pool) { h, ok = p.Locate(tid) })
	return ok, h
}

// descends reports whether entry tip's ledger at node i holds entry h.
func descends(sm *Sim, i int, pool uint64, tip, h hash.Hash) bool {
	found := false
	sm.nodes[i].Eng.Ledger(sm.Pool(pool), func(p *ledger.Pool) {
		for cur := tip; ; {
			if cur == h {
				found = true
				return
			}
			e, ok := p.Entry(cur)
			if !ok {
				return
			}
			cur = e.Parent
		}
	})
	return found
}

// SizingRow is one pool size of a sizing sweep.
type SizingRow struct {
	PoolSize uint64 `json:"pool_size"`
	Pools    uint64 `json:"pools"`
	// From a Load run: payments settled, their mean wait, and the share
	// of work on siblings.
	Settled      int     `json:"settled"`
	MeanWait     float64 `json:"mean_wait_ms"`
	SiblingShare float64 `json:"sibling_share"`
	// From attack runs at the given share and threshold: the share of
	// released payments reversed.
	AttackWon float64 `json:"attack_won"`
}

// Sizing is the design's "sizing sweep": for each pool size, the same
// mints, the same total payment rate and the same total builder power
// (each builder's tries split evenly over the pools), it measures the
// wait for a payment to be selected, the work lost to siblings, and how
// often an attacker with the given share and head start reverses a
// payment the receiver released at threshold.
func Sizing(seed int64, n Net, base series.Params, sizes []uint64, payEvery, d Time, atk AttackConfig, attackRuns int) ([]SizingRow, error) {
	var rows []SizingRow
	for _, size := range sizes {
		p := base
		p.PoolSize = size
		s, err := series.New(p)
		if err != nil {
			return nil, err
		}
		nn := n
		nn.Tries = n.Tries * base.Mints / base.PoolSize / s.PoolCount()
		if nn.Tries == 0 {
			nn.Tries = 1
		}
		lr, err := Load(seed, nn, s, payEvery, d)
		if err != nil {
			return nil, err
		}
		row := SizingRow{PoolSize: size, Pools: s.PoolCount(), Settled: lr.Settled, MeanWait: lr.MeanWait, SiblingShare: lr.SiblingShare}
		if attackRuns > 0 {
			a := atk
			a.Net = nn
			won, _, _, err := AttackSweep(a, s, attackRuns)
			if err != nil {
				return nil, err
			}
			row.AttackWon = won
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// AttackSweep runs Attack for seeds 0 to runs-1 and returns the share of
// released payments the attacker reversed, and the mean release time.
func AttackSweep(c AttackConfig, s *series.Series, runs int) (won float64, released int, meanRelease float64, err error) {
	wins := 0
	var total Time
	for seed := int64(0); seed < int64(runs); seed++ {
		r, err := Attack(seed, c, s)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("seed %d: %w", seed, err)
		}
		if r.Released {
			released++
			total += r.ReleasedAfter
			if r.Won {
				wins++
			}
		}
	}
	if released > 0 {
		won = float64(wins) / float64(released)
		meanRelease = float64(total) / float64(released)
	}
	return won, released, meanRelease, nil
}
