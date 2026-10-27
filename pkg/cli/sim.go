package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/sim"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

const simUsage = `poo sim: run the network simulator on the worked example's series

Usage: poo sim SCENARIO [flags]

  load        payments under load: agreement, conservation, wait, sibling work
  partition   split the network, heal it: the heavier side must win
  attack      double spends with a head start against a receiver's threshold
  sizing      the sizing sweep: pool sizes at the same total rate and power

Every run is deterministic for its -seed. Results are JSON.
`

// simFlags are the network flags every scenario takes.
type simFlags struct {
	seed                         *int64
	runs                         *int
	nodes, degree, builders      *int
	tries                        *uint64
	minLat, maxLat, work, build  *int64
	duration, payEvery, k        *int64
	share                        *float64
	threshold, sizes             *string
	headStart, giveUp, partition *int64
}

func addSim(fs *flag.FlagSet) *simFlags {
	d := sim.DefaultNet()
	return &simFlags{
		seed:      fs.Int64("seed", 1, "first seed"),
		runs:      fs.Int("runs", 1, "runs, with seeds from -seed up"),
		nodes:     fs.Int("nodes", d.Nodes, "full nodes"),
		degree:    fs.Int("degree", d.Degree, "random links each node starts, besides the ring"),
		builders:  fs.Int("builders", d.Builders, "nodes that build and add work"),
		tries:     fs.Uint64("tries", d.Tries, "nonces per builder per pool per work period"),
		minLat:    fs.Int64("min-latency", int64(d.MinLatency), "least one-way link delay, ms"),
		maxLat:    fs.Int64("max-latency", int64(d.MaxLatency), "greatest one-way link delay, ms"),
		work:      fs.Int64("work-every", int64(d.WorkEvery), "work period, ms"),
		build:     fs.Int64("build-every", int64(d.BuildEvery), "how often builders bundle, ms"),
		k:         fs.Int64("k", 0, "inactive branches each node keeps (0: default)"),
		duration:  fs.Int64("duration", 60000, "how long payments run, ms"),
		payEvery:  fs.Int64("pay-every", 300, "one payment every this many ms"),
		share:     fs.Float64("share", 0.3, "attack: the attacker's share of all work"),
		threshold: fs.String("threshold", "5000", "attack: the from(e) the receiver waits for"),
		headStart: fs.Int64("head-start", 2000, "attack: ms the attacker grinds before paying"),
		giveUp:    fs.Int64("give-up", 60000, "attack: ms the attacker keeps trying after release"),
		partition: fs.Int64("split", 30000, "partition: how long the network stays split, ms"),
		sizes:     fs.String("sizes", "1,2,4,8", "sizing: pool sizes to try"),
	}
}

func (f *simFlags) net() sim.Net {
	return sim.Net{
		Nodes: *f.nodes, Degree: *f.degree, Builders: *f.builders, Tries: *f.tries,
		MinLatency: sim.Time(*f.minLat), MaxLatency: sim.Time(*f.maxLat),
		WorkEvery: sim.Time(*f.work), BuildEvery: sim.Time(*f.build), K: int(*f.k),
	}
}

func (f *simFlags) attack() (sim.AttackConfig, error) {
	var th weight.Weight
	if err := th.UnmarshalText([]byte(*f.threshold)); err != nil {
		return sim.AttackConfig{}, fmt.Errorf("-threshold: %w", err)
	}
	return sim.AttackConfig{Net: f.net(), Share: *f.share, Threshold: th, HeadStart: sim.Time(*f.headStart), GiveUp: sim.Time(*f.giveUp)}, nil
}

func simCmd(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, simUsage)
		return nil
	}
	fs := newFlagSet("sim "+args[0], out)
	f := addSim(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *f.runs < 1 {
		return errors.New("-runs must be at least 1")
	}
	s := testutil.ExampleSeries()
	n := f.net()
	var results []any
	switch args[0] {
	case "load":
		for i := 0; i < *f.runs; i++ {
			r, err := sim.Load(*f.seed+int64(i), n, s, sim.Time(*f.payEvery), sim.Time(*f.duration))
			if err != nil {
				return err
			}
			results = append(results, r)
		}
	case "partition":
		for i := 0; i < *f.runs; i++ {
			r, err := sim.Partition(*f.seed+int64(i), n, s, sim.Time(*f.partition))
			if err != nil {
				return err
			}
			results = append(results, r)
		}
	case "attack":
		c, err := f.attack()
		if err != nil {
			return err
		}
		won, released := 0, 0
		var runs []sim.AttackReport
		for i := 0; i < *f.runs; i++ {
			r, err := sim.Attack(*f.seed+int64(i), c, s)
			if err != nil {
				return err
			}
			runs = append(runs, r)
			if r.Released {
				released++
				if r.Won {
					won++
				}
			}
		}
		share := 0.0
		if released > 0 {
			share = float64(won) / float64(released)
		}
		return writeJSON(out, map[string]any{"share": c.Share, "threshold": c.Threshold, "released": released, "won": won, "won_share": share, "runs": runs})
	case "sizing":
		var sizes []uint64
		for _, v := range strings.Split(*f.sizes, ",") {
			n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
			if err != nil || n == 0 {
				return fmt.Errorf("-sizes: %q is not a pool size", v)
			}
			sizes = append(sizes, n)
		}
		c, err := f.attack()
		if err != nil {
			return err
		}
		rows, err := sim.Sizing(*f.seed, n, testutil.ExampleParams(), sizes, sim.Time(*f.payEvery), sim.Time(*f.duration), c, *f.runs)
		if err != nil {
			return err
		}
		return writeJSON(out, rows)
	default:
		return fmt.Errorf("unknown scenario %q\n\n%s", args[0], simUsage)
	}
	if len(results) == 1 {
		return writeJSON(out, results[0])
	}
	return writeJSON(out, results)
}
