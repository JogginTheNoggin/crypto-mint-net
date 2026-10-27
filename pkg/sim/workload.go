package sim

import (
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Wallets are the simulated holders: keys from labels, which claim mints
// and pay each other through whichever node they are handed to.
type Wallets struct {
	sim    *Sim
	Keys   *testutil.Keys
	labels []string
	owner  map[hash.Hash]string
	// Sent counts transactions the nodes accepted; Refused those they
	// did not (a conflict with a pending one, usually).
	Sent, Refused int
}

// NewWallets makes n holders, labelled h0 to h(n-1).
func NewWallets(s *Sim, n int) *Wallets {
	w := &Wallets{sim: s, Keys: testutil.NewKeys(s.s), owner: map[hash.Hash]string{}}
	for i := 0; i < n; i++ {
		w.Add(fmt.Sprintf("h%d", i))
	}
	return w
}

// Add makes another holder and returns its address.
func (w *Wallets) Add(label string) hash.Hash {
	a := w.Keys.Addr(label)
	if _, ok := w.owner[a]; !ok {
		w.labels = append(w.labels, label)
		w.owner[a] = label
	}
	return a
}

func (w *Wallets) randomLabel() string { return w.labels[w.sim.rnd.Intn(len(w.labels))] }

// Claim claims mint serial for a random holder through node i.
func (w *Wallets) Claim(i int, serial uint64) (*tx.Tx, error) {
	t := w.Keys.Claim(serial, testutil.P(w.randomLabel(), w.sim.s.Supply()))
	return t, w.submit(i, t)
}

// ClaimAll claims every mint, each through a random node.
func (w *Wallets) ClaimAll() error {
	for serial := uint64(0); serial < w.sim.s.Mints(); serial++ {
		if _, err := w.Claim(w.sim.rnd.Intn(len(w.sim.nodes)), serial); err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallets) submit(i int, t *tx.Tx) error {
	if err := w.sim.Submit(i, t); err != nil {
		w.Refused++
		return err
	}
	w.Sent++
	return nil
}

// Pay makes a random payment in mint serial through node i: one or two
// portions the node shows a holder owning, split between a random
// receiver and change, or merged. It returns nil when the node shows
// nothing to spend.
func (w *Wallets) Pay(i int, serial uint64) (*tx.Tx, error) {
	hs, err := w.sim.nodes[i].Eng.Holdings(w.sim.s.ID(), serial)
	if err != nil || len(hs) == 0 {
		return nil, err
	}
	byOwner := map[string][]state.Holding{}
	for _, h := range hs {
		if l, ok := w.owner[h.Portion.Address]; ok {
			byOwner[l] = append(byOwner[l], h)
		}
	}
	if len(byOwner) == 0 {
		return nil, nil
	}
	owners := make([]string, 0, len(byOwner))
	for l := range byOwner {
		owners = append(owners, l)
	}
	sort.Strings(owners)
	from := owners[w.sim.rnd.Intn(len(owners))]
	mine := byOwner[from]
	n := 1
	if len(mine) > 1 && w.sim.rnd.Intn(3) == 0 {
		n = 2 // a merge
	}
	var ins []testutil.In
	var total uint64
	for _, h := range mine[:n] {
		ins = append(ins, testutil.In{Input: h.Input, Owner: from})
		total += h.Portion.Amount
	}
	to := w.randomLabel()
	var outs []testutil.Pay
	if total > 1 && w.sim.rnd.Intn(2) == 0 && to != from {
		pay := 1 + uint64(w.sim.rnd.Int63n(int64(total-1)))
		outs = []testutil.Pay{testutil.P(to, pay), testutil.P(from, total-pay)}
	} else {
		outs = []testutil.Pay{testutil.P(to, total)}
	}
	t := w.Keys.Spend(serial, ins, outs...)
	return t, w.submit(i, t)
}

// RandomPayments starts payments: every period, a random node is handed
// a payment in a random mint, until stop is set.
func (w *Wallets) RandomPayments(period Time, stop *bool) {
	w.sim.Every(period, func() bool {
		if *stop {
			return false
		}
		w.Pay(w.sim.rnd.Intn(len(w.sim.nodes)), uint64(w.sim.rnd.Int63n(int64(w.sim.s.Mints()))))
		return true
	})
}

// Builder is one node building entries and adding layers on its selected
// tip at fixed periods.
type Builder struct {
	Node    int
	Pool    uint64
	stopped bool
}

// Stop ends the builder's work after the current tick.
func (b *Builder) Stop() { b.stopped = true }

// StartBuilder makes node i build an entry from its pending transactions
// every buildEvery (0: never) and try tries nonces on its tip every
// workEvery (0: never), starting at a random offset within each period.
func (s *Sim) StartBuilder(i int, pool uint64, buildEvery, workEvery Time, tries uint64) *Builder {
	b := &Builder{Node: i, Pool: pool}
	n := s.nodes[i]
	if buildEvery > 0 {
		s.After(Time(s.rnd.Int63n(int64(buildEvery))), func() {
			n.Build(pool)
			s.Every(buildEvery, func() bool {
				if b.stopped {
					return false
				}
				n.Build(pool)
				return true
			})
		})
	}
	if workEvery > 0 && tries > 0 {
		s.After(Time(s.rnd.Int63n(int64(workEvery))), func() {
			n.Work(pool, tries)
			s.Every(workEvery, func() bool {
				if b.stopped {
					return false
				}
				n.Work(pool, tries)
				return true
			})
		})
	}
	return b
}
