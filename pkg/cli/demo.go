package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// demoNet is a few in-process engines that forward to each other.
type demoNet struct {
	nodes []*engine.Engine
	key   engine.PoolKey
	out   io.Writer
	quiet bool
}

type demoItem struct {
	tx    *tx.Tx
	entry *entry.Entry
	layer *layer.Layer
}

func (d *demoNet) say(format string, a ...any) {
	if !d.quiet {
		fmt.Fprintf(d.out, format+"\n", a...)
	}
}

// flood forwards an item kept by node from to the others, the way the
// forwarding rules say, until nobody forwards anything new.
func (d *demoNet) flood(from int, it demoItem) {
	type msg struct {
		to int
		it demoItem
	}
	var q []msg
	push := func(skip int, it demoItem) {
		for i := range d.nodes {
			if i != skip {
				q = append(q, msg{i, it})
			}
		}
	}
	push(from, it)
	for len(q) > 0 {
		m := q[0]
		q = q[1:]
		var fwd bool
		switch {
		case m.it.tx != nil:
			r, _ := d.nodes[m.to].HandleTx(m.it.tx, engine.Pushed)
			fwd = r.Forward
		case m.it.entry != nil:
			r, _ := d.nodes[m.to].HandleEntry(d.key, m.it.entry, engine.Pushed)
			fwd = r.Forward
		case m.it.layer != nil:
			r, _ := d.nodes[m.to].HandleLayer(d.key, *m.it.layer, engine.Pushed)
			fwd = r.Forward
		}
		if fwd {
			push(m.to, m.it)
		}
	}
}

func (d *demoNet) status(label string) engine.PoolStatus {
	st, _ := d.nodes[0].Status(d.key)
	agree := true
	for _, n := range d.nodes[1:] {
		o, _ := n.Status(d.key)
		agree = agree && o.Tip == st.Tip && o.Weight.Cmp(st.Weight) == 0
	}
	d.say("  %-34s tip %s  height %d  weight %-8s all %d nodes agree: %v",
		label, st.Tip.Short(), st.Height, st.Weight, len(d.nodes), agree)
	return st
}

func (d *demoNet) work(node int, e *entry.Entry, start, tries uint64) {
	s, _ := d.nodes[node].Series(d.key.Series)
	l, r, err := d.nodes[node].Work(d.key, e.Hash(s), start, tries)
	if err != nil && !errors.Is(err, engine.ErrSeen) {
		return
	}
	d.say("    node %d adds a layer of strength %d on %s", node, l.Strength(s.Hash()), e.Hash(s).Short())
	_ = r
	d.flood(node, demoItem{layer: &l})
}

func demo(out io.Writer, quiet bool) error {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	d := &demoNet{key: engine.PoolKey{Series: s.ID(), Index: 1}, out: out, quiet: quiet}
	for i := 0; i < 4; i++ {
		e := engine.New(engine.DefaultConfig())
		e.AddSeries(s)
		if err := e.FollowSeries(s.ID()); err != nil {
			return err
		}
		d.nodes = append(d.nodes, e)
	}
	d.say("Series %s: %d mints of %d units, pool size %d. Following pool 1 (mints 4 to 7).",
		s.ID().Short(), s.Mints(), s.Supply(), s.PoolSize())
	d.say("Four full nodes forward to each other in process.\n")

	submit := func(node int, t *tx.Tx) error {
		if _, err := d.nodes[node].HandleTx(t, engine.Pushed); err != nil {
			return err
		}
		d.flood(node, demoItem{tx: t})
		return nil
	}
	build := func(node int) (*entry.Entry, error) {
		e, _, err := d.nodes[node].Build(d.key, 0)
		if err != nil {
			return nil, err
		}
		d.say("    node %d builds entry %s with %d transaction(s)", node, e.Hash(s).Short(), len(e.Txs))
		d.flood(node, demoItem{entry: e})
		return e, nil
	}

	d.say("1. Alice claims mint 4, Bob claims mint 5.")
	ca, cb := k.Claim(4, testutil.P("alice", 100)), k.Claim(5, testutil.P("bob", 100))
	if err := submit(0, ca); err != nil {
		return err
	}
	if err := submit(1, cb); err != nil {
		return err
	}
	e1, err := build(2)
	if err != nil {
		return err
	}
	d.work(3, e1, 0, 4000)
	d.status("after the claims")

	d.say("\n2. Alice pays the store 30 units; Bob pays Dave 40. One entry protects both mints.")
	pay := k.Spend(4, []testutil.In{k.Out(ca, 0, "alice")}, testutil.P("store", 30), testutil.P("alice2", 70))
	bob := k.Spend(5, []testutil.In{k.Out(cb, 0, "bob")}, testutil.P("dave", 40), testutil.P("bob2", 60))
	if err := submit(0, pay); err != nil {
		return err
	}
	if err := submit(1, bob); err != nil {
		return err
	}
	e2, err := build(0)
	if err != nil {
		return err
	}
	d.work(1, e2, 1<<32, 4000)
	d.work(2, e2, 2<<32, 4000)
	honest := d.status("after the payments")
	p, _ := d.nodes[3].Payment(d.key, pay.TID(s.Hash()))
	d.say("  the store reads from(e) = %s for its payment", p.From)

	d.say("\n3. Alice signs a second spend of her claimed portion and grinds a rival entry on E1.")
	rivalTx := k.Spend(4, []testutil.In{k.Out(ca, 0, "alice")}, testutil.P("alice-hidden", 100))
	rival, _, err := entry.Build(s, e1.Hash(s), []*tx.Tx{rivalTx}, 0)
	if err != nil {
		return err
	}
	if _, err := d.nodes[3].HandleEntry(d.key, rival, engine.Local); err != nil {
		return err
	}
	d.flood(3, demoItem{entry: rival})
	var won engine.PoolStatus
	for i := uint64(0); i < 64; i++ {
		d.work(3, rival, (10+i)<<32, 20000)
		if won = d.status("rival grinding"); won.Tip == rival.Hash(s) {
			break
		}
	}
	if won.Tip != rival.Hash(s) {
		return fmt.Errorf("demo: rival never overtook")
	}
	p, _ = d.nodes[0].Payment(d.key, pay.TID(s.Hash()))
	pb, _ := d.nodes[0].Payment(d.key, bob.TID(s.Hash()))
	d.say("  the store's payment is now selected: %v (it was reversed)", p.Selected)
	d.say("  Bob's payment to Dave is carried, pending again: %v", pb.Pending)

	d.say("\n4. Bob's payment is bundled again on the rival's tip, and the store's side adds work to E2.")
	if _, err := build(1); err != nil {
		return err
	}
	var back engine.PoolStatus
	for i := uint64(0); i < 64; i++ {
		d.work(2, e2, (100+i)<<32, 20000)
		if back = d.status("store defending"); back.Tip != rival.Hash(s) && back.Weight.Greater(won.Weight) {
			if bt, _ := d.nodes[0].Payment(d.key, pay.TID(s.Hash())); bt.Selected {
				break
			}
		}
	}
	p, _ = d.nodes[0].Payment(d.key, pay.TID(s.Hash()))
	fmt.Fprintf(out, "\nSummary: honest weight %s, rival won at %s, final weight %s; store's payment selected again: %v.\n",
		honest.Weight, won.Weight, back.Weight, p.Selected)
	if !p.Selected {
		return fmt.Errorf("demo: the honest ledger did not return")
	}
	return nil
}
