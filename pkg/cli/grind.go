package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

const grindUsage = `poo grind: the node's hash work (package grind, M11)

  devices [-o FORMAT]       the devices, what each grinds fast, and which
                            one auto picks for each hash
  bench [-series ID] [-device D] [-threads N] [-seconds S] [-o FORMAT]
                            how fast each device grinds the series' hash
                            (every hash without -series), here
  start TARGET [JOB FLAGS]  start a job in the running node; prints it
  run TARGET [JOB FLAGS]    start a job and follow it: one JSON line per
                            event (start, progress, found, waiting, end)
                            until it ends; an interrupt stops the job and
                            exits 0
  jobs [-o FORMAT]          the node's jobs, running and recently ended
  stop ID | -all            stop a job, or every job

TARGET is tip (layers on each pool's selected tip), pending (entries from
pending transactions as they arrive) or an entry hash (layers on it). The
series of the target says which hash to grind: no command names a hash.

JOB FLAGS: -series ID, -pool K or -serial N (default: every followed
pool), -device D (auto, cpu, gpu:0; default: node.json's grind section,
else auto), -threads N (default: every core but one), -for 30s,
-until-from W (with an entry: stop once from(e) reaches W), -hours 22-06
(only within those local hours).

poo build and poo work take -device, -threads and -for too. node.json's
"grind" section sets the defaults and jobs the node runs by itself.
`

func grindCmd(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, grindUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("grind "+sub, out)
	switch sub {
	case "devices":
		return grindDevices(fs, rest, out)
	case "bench":
		return grindBench(fs, rest, out)
	case "start", "run":
		return grindStart(fs, sub == "run", rest, out)
	case "jobs":
		w, o := addWhere(fs), addOutput(fs)
		if err := parseOutput(fs, o, rest, 0); err != nil {
			return err
		}
		cl, err := dialNode(out, w.resolve())
		if err != nil {
			return err
		}
		jobs, err := cl.GrindJobs()
		if err != nil {
			return err
		}
		return o.write(out, jobs)
	case "stop":
		w := addWhere(fs)
		all := fs.Bool("all", false, "stop every job")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *all == (fs.NArg() == 1) || fs.NArg() > 1 {
			return errors.New("usage: poo grind stop ID | -all")
		}
		cl, err := dialNode(out, w.resolve())
		if err != nil {
			return err
		}
		if *all {
			jobs, err := cl.GrindStopAll("")
			if err != nil {
				return err
			}
			return writeJSON(out, jobs)
		}
		id, err := strconv.Atoi(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("job id: %w", err)
		}
		j, err := cl.GrindStop(id)
		if err != nil {
			return err
		}
		return writeJSON(out, j)
	}
	return fmt.Errorf("poo grind %s: unknown; see poo grind help", sub)
}

// parseOutput parses flags with an output flag and n arguments.
func parseOutput(fs *flag.FlagSet, o *outputFlags, args []string, n int) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if fs.NArg() != n {
		return fmt.Errorf("poo %s takes flags only", fs.Name())
	}
	return nil
}

// DevicesReply is what poo grind devices prints.
type devicesReply struct {
	Devices []grind.Device `json:"devices"`
	// Local is true when no node answered and the devices were detected
	// here instead.
	Local bool `json:"local,omitempty"`
}

func grindDevices(fs *flag.FlagSet, args []string, out io.Writer) error {
	w, o := addWhere(fs), addOutput(fs)
	if err := parseOutput(fs, o, args, 0); err != nil {
		return err
	}
	// The node is what grinds, so ask it; without one, look here.
	if cl, err := dialNode(out, w.resolve()); err == nil {
		if r, err := cl.GrindDevices(); err == nil {
			return o.write(out, devicesReply{Devices: r.Devices})
		}
	}
	return o.write(out, devicesReply{Devices: grind.Default.Detect(), Local: true})
}

func grindBench(fs *flag.FlagSet, args []string, out io.Writer) error {
	w := addWhere(fs)
	seriesID := fs.String("series", "", "the series whose hash to grind (default: every hash)")
	device := fs.String("device", "", "a device id (default: every device that can grind the hash)")
	threads := fs.Int("threads", 0, "threads per device (default: every core but one)")
	secs := fs.Float64("seconds", 1, "how long to grind on each device and hash")
	o := addOutput(fs)
	if err := parseOutput(fs, o, args, 0); err != nil {
		return err
	}
	if *secs <= 0 || *secs > 60 {
		return errors.New("-seconds must be above 0 and at most 60")
	}
	ids := hash.IDs()
	if *seriesID != "" {
		id, err := hash.Parse(*seriesID)
		if err != nil {
			return fmt.Errorf("-series: %w", err)
		}
		p, err := nodepolicy.Load(filepath.Join(w.resolve(), nodepolicy.FileName))
		if err != nil {
			return err
		}
		ids = nil
		for _, si := range p.Series {
			if sr, err := series.New(si.Params); err == nil && sr.ID() == id {
				ids = []hash.ID{si.Params.HashID}
			}
		}
		if ids == nil {
			return fmt.Errorf("the node policy does not list series %s", id.Short())
		}
	}
	devs := grind.Default.Detect()
	ctx := contextOf(out)
	rows := []grind.Bench{}
	for _, id := range ids {
		for _, d := range devs {
			if *device != "" && d.ID != *device {
				continue
			}
			p, err := grind.Default.Choose(devs, id, d.ID, *threads)
			if err != nil {
				if *device != "" {
					return err
				}
				continue // this device cannot grind this hash
			}
			b, err := grind.RunBench(ctx, p, time.Duration(*secs*float64(time.Second)))
			if err != nil {
				return err
			}
			rows = append(rows, b)
			if ctx.Err() != nil {
				return o.write(out, rows)
			}
		}
	}
	if *device != "" && len(rows) == 0 {
		return fmt.Errorf("no device %q (poo grind devices lists them)", *device)
	}
	return o.write(out, rows)
}

// jobFlags are the flags of poo grind start and run.
type jobFlags struct {
	c       *clientFlags
	pf      *poolFlags
	device  *string
	threads *int
	dur     *string
	until   *string
	hours   *string
}

func addJob(fs *flag.FlagSet) *jobFlags {
	return &jobFlags{
		c:       addClient(fs),
		pf:      addPool(fs),
		device:  fs.String("device", "", "device: auto, cpu, gpu:0, ... (default: node.json's grind section, else auto)"),
		threads: fs.Int("threads", 0, "threads (default: every core but one)"),
		dur:     fs.String("for", "", "stop after this long: 30s, 2h (default: until stopped)"),
		until:   fs.String("until-from", "", "with an entry target: stop once from(e) reaches this weight"),
		hours:   fs.String("hours", "", "work only within these local hours: 22-06"),
	}
}

// spec reads the job's spec, resolving -series, -pool and -serial with
// the node.
func (j *jobFlags) spec(fs *flag.FlagSet, cl *rpc.Client, target string) (grind.Spec, error) {
	s := grind.Spec{Target: target, Device: *j.device, Threads: *j.threads, For: *j.dur, UntilFrom: *j.until, Hours: *j.hours}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["series"] || set["pool"] || set["serial"] {
		sr, err := j.c.seriesOf(cl)
		if err != nil {
			return s, err
		}
		id := sr.ID()
		s.Series = &id
		if set["pool"] || set["serial"] {
			k, err := j.pf.resolve(fs, sr)
			if err != nil {
				return s, err
			}
			s.Pools = []uint64{k}
		}
	}
	return s, grind.Check(s)
}

// errJobEnded stops following a job.
var errJobEnded = errors.New("the job ended")

func grindStart(fs *flag.FlagSet, follow bool, args []string, out io.Writer) error {
	j := addJob(fs)
	// TARGET comes first, as poo grind help writes it, or after the flags.
	target := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if target == "" && fs.NArg() == 1 {
		target = fs.Arg(0)
	} else if fs.NArg() != 0 {
		target = ""
	}
	if target == "" {
		return fmt.Errorf("usage: poo %s TARGET [flags]; TARGET is tip, pending or an entry hash", fs.Name())
	}
	cl, err := j.c.dial(out)
	if err != nil {
		return err
	}
	s, err := j.spec(fs, cl, target)
	if err != nil {
		return err
	}
	if !follow {
		job, err := cl.GrindStart(s)
		if err != nil {
			return err
		}
		return writeJSON(out, job)
	}
	// Follow: listen first, start once listening, so no event is missed.
	ctx := contextOf(out)
	enc := json.NewEncoder(out)
	id := 0
	err = cl.Watch(ctx, filter.Filter{Kinds: []filter.Kind{filter.KindGrind}}, func(ev watch.Event) error {
		switch {
		case ev.Kind == "hello" && id == 0:
			job, err := cl.GrindStart(s)
			if err != nil {
				return err
			}
			id = job.ID
		case ev.Kind == "end":
			return errors.New("the node stopped its interface (a reload or a stop), and with it the job")
		case ev.Kind == filter.KindGrind && ev.Grind != nil && ev.Grind.Job == id:
			if err := enc.Encode(ev.Grind); err != nil {
				return err
			}
			if ev.Grind.Kind == "end" {
				return errJobEnded
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, errJobEnded):
		return nil
	case ctx.Err() != nil && id != 0:
		// Interrupted: stop the job, keeping what it found, and say so.
		job, serr := cl.GrindStop(id)
		if serr != nil {
			return serr
		}
		return enc.Encode(grind.Event{Job: id, Kind: "end", State: job.State, Reason: "interrupted", Tries: job.Tries})
	case ctx.Err() != nil:
		return nil
	}
	return err
}
