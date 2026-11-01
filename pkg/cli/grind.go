package cli

import (
	"context"
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
  start TARGET [JOB FLAGS] [-lease 30s]
                            start a job in the running node; prints it
  run TARGET [JOB FLAGS]    start a job and follow it: one JSON line per
                            event (start, plan, progress, found, waiting,
                            end) until it ends; the job stops if this
                            command goes away, and an interrupt stops it
                            and exits 0
  jobs [-o FORMAT]          the node's jobs, running and recently ended
  stop ID | -all            stop a job, or every job
  schedule [-o FORMAT]      the jobs node.json starts, and its defaults
  schedule add TARGET [SCHEDULE FLAGS]
                            add a job to node.json; a running node starts
                            it at once
  schedule remove N... | -all
                            remove jobs, numbered from 1 as listed
  schedule set [-device D] [-threads N]
                            the defaults for every job, build and work

TARGET is tip (layers on each pool's selected tip), pending (entries from
pending transactions as they arrive), tx:TID (one payment: entries while
it is pending, then layers on its entry, until -until-from W) or an entry
hash (layers on it). The series of the target says which hash to grind:
no command names a hash.

JOB FLAGS: -series ID, -pool K or -serial N (default: every followed
pool), -device D (auto, cpu, gpu:0; default: node.json's grind section,
else auto), -threads N (default: every core but one), -for 30s,
-until-from W (with an entry or tx: stop once from(e) reaches W), -hours
22-06 (only within those local hours). SCHEDULE FLAGS are the same but
-for: a scheduled job runs whenever its hours allow.

poo build and poo work take -device, -threads and -for too.
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
	case "schedule":
		return grindSchedule(fs, rest, out)
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
	var skipped error // why the last device was passed over
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
				skipped = err
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
	// Nothing measured is an error, not an empty list: -threads 2000 used
	// to print [] (GREEN-035).
	if len(rows) == 0 && skipped != nil {
		return skipped
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
	lease   *string
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
		lease:   fs.String("lease", "", "start: stop the job unless it is renewed within this long (POST /v1/grind/ID/renew)"),
	}
}

// spec reads the job's spec, resolving -series, -pool and -serial with
// the node.
func (j *jobFlags) spec(fs *flag.FlagSet, cl *rpc.Client, target string) (grind.Spec, error) {
	s := grind.Spec{Target: target, Device: *j.device, Threads: *j.threads, For: *j.dur, UntilFrom: *j.until, Hours: *j.hours, Lease: *j.lease}
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

// runLease is the lease of a job poo grind run follows; it renews it three
// times as often.
const runLease = 15 * time.Second

// renewer is what renew needs of a client.
type renewer interface {
	GrindRenew(id int) (grind.Job, error)
}

// renew renews job id's lease until ctx ends or the job is gone: done, or
// unknown to the node. A failed request (a timeout, a dropped connection,
// a node busy reloading) is tried again at the next tick while the lease
// may still hold, so one failure does not stop a job whose follower is
// alive (RED-019, decision D120). The first failure is written to warn.
func renew(ctx context.Context, cl renewer, id int, every time.Duration, warn io.Writer) {
	t := time.NewTicker(every)
	defer t.Stop()
	last := time.Now()
	warned := false
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			j, err := cl.GrindRenew(id)
			var re *rpc.Error
			switch {
			case err == nil && j.Done():
				return
			case err == nil:
				last, warned = now, false
			case errors.As(err, &re) && re.Code == rpc.CodeNotFound:
				return
			case now.Sub(last) > runLease:
				return // the lease has run out on the node anyway
			case !warned:
				warned = true
				fmt.Fprintf(warn, "poo: renewing job %d failed, trying again: %v\n", id, err)
			}
		}
	}
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
		return fmt.Errorf("usage: poo %s TARGET [flags]; TARGET is tip, pending, tx:TID or an entry hash", fs.Name())
	}
	if follow && *j.lease != "" {
		return errors.New("-lease is for poo grind start; run keeps its job's lease itself")
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
	// The job has a lease this command renews, so it ends if this command
	// dies without stopping it (GUI-010).
	s.Lease = runLease.String()
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
			go renew(ctx, cl, id, runLease/3, errOf(out))
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

// grindSchedule shows or changes node.json's grind section. With a running
// node the change goes through it and applies at once, without a reload,
// so running jobs go on; otherwise node.json is edited and the node
// applies it when it starts (GREEN-026).
func grindSchedule(fs *flag.FlagSet, args []string, out io.Writer) error {
	op := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		op, args = args[0], args[1:]
	}
	w := addWhere(fs)
	var o *outputFlags
	var c *clientFlags
	var pf *poolFlags
	var device, until, hours, target *string
	var threads, floor, rate *int
	var all *bool
	var expect *string
	switch op {
	case "":
		o = addOutput(fs)
	case "add":
		c = &clientFlags{w: w, series: fs.String("series", "", "series id (default: every followed series)")}
		pf = addPool(fs)
		device = fs.String("device", "", "device: auto, cpu, gpu:0, ...")
		threads = fs.Int("threads", 0, "threads (default: the section's)")
		until = fs.String("until-from", "", "with an entry or tx target: stop once from(e) reaches this weight")
		hours = fs.String("hours", "", "work only within these local hours: 22-06")
		target = new(string)
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			*target, args = args[0], args[1:]
		}
	case "remove":
		all = fs.Bool("all", false, "remove every job")
		expect = fs.String("target", "", "also check that each numbered job has this target (a removal names the whole job anyway, so a stale number is refused)")
	case "set":
		device = fs.String("device", "", "the default device: auto, cpu, gpu:0, ... (auto clears it)")
		threads = fs.Int("threads", 0, "the default threads (0: every core but one)")
		floor = fs.Int("layer-floor", 0, "keep every layer the node's grinding finds at or above this strength, 0 to 256 (0: only the strongest of each search)")
		rate = fs.Int("layer-rate", 0, "the most layers a second layer-floor adds, strongest first (0: the default, 10)")
	default:
		return fmt.Errorf("poo grind schedule %s: unknown; it is add, remove or set", op)
	}
	var ifVersion *string
	if op != "" {
		ifVersion = fs.String("if-version", "", "change only the section with this version, as poo grind schedule printed it (GREEN-034)")
	}
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if o != nil {
		if err := o.check(); err != nil {
			return err
		}
	}
	dir := w.resolve()
	cl, dialErr := dialNode(out, dir)
	var cur nodepolicy.Grind
	var p nodepolicy.Policy
	path := filepath.Join(dir, nodepolicy.FileName)
	online := false
	var readErr error
	if dialErr == nil {
		var r rpc.ScheduleReply
		if r, readErr = cl.GrindSchedule(); readErr == nil {
			cur, online = r.Grind, true
		}
	}
	if !online {
		if op != "" {
			// With no node, node.json is changed under the data
			// directory's lock, which a running node holds, so a change
			// is never written behind a node that answered with an
			// error or did not answer at all (its next write would undo
			// it), nor while one starts or another command writes
			// (GUI-017, D134). A leftover rpc.addr from a crash holds no
			// lock, so the change goes on.
			unlock, err := nodepolicy.Lock(dir)
			if errors.Is(err, nodepolicy.ErrLocked) {
				if readErr != nil {
					return fmt.Errorf("the node is running but its schedule could not be read, so nothing was changed: %w", readErr)
				}
				return fmt.Errorf("a node or another command holds %s, and no node answered, so nothing was changed: try again once it has started or ended: %w", dir, err)
			}
			if err != nil {
				return err
			}
			defer unlock()
		}
		var err error
		if p, err = nodepolicy.Load(path); err != nil {
			return err
		}
		if p.Grind != nil {
			cur = *p.Grind
		}
	}
	if op == "" {
		if len(pos) != 0 {
			return errors.New("usage: poo grind schedule [add|remove|set]")
		}
		return o.write(out, rpc.ScheduleReply{Grind: cur, Version: cur.Version(), Limits: nodepolicy.Limits()})
	}
	if viewOnlyOf(out) {
		return errViewOnly("grind schedule " + op)
	}
	// The change is sent as an edit, which the node applies under its
	// lock to the section it holds, so a change made meanwhile by another
	// client is kept, and a numbered removal names the whole job, so a
	// stale number cannot remove another job (GUI-014, D125).
	var edit nodepolicy.GrindEdit
	switch op {
	case "add":
		if *target == "" && len(pos) == 1 {
			*target = pos[0]
		} else if len(pos) != 0 {
			*target = ""
		}
		if *target == "" {
			return errors.New("usage: poo grind schedule add TARGET [flags]")
		}
		j := nodepolicy.GrindJob{Target: *target, Device: *device, Threads: *threads, Hours: *hours, UntilFrom: *until}
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["series"] || set["pool"] || set["serial"] {
			var sr *series.Series
			var err error
			if online {
				sr, err = c.seriesOf(cl)
			} else {
				sr, err = policySeries(p, *c.series)
			}
			if err != nil {
				return err
			}
			id := sr.ID()
			j.Series = &id
			if set["pool"] || set["serial"] {
				k, err := pf.resolve(fs, sr)
				if err != nil {
					return err
				}
				j.Pools = []uint64{k}
			}
		}
		if err := grind.Check(grind.SpecOf(j)); err != nil {
			return err
		}
		edit.Add = []nodepolicy.GrindJob{j}
	case "remove":
		if *all == (len(pos) > 0) {
			return errors.New("usage: poo grind schedule remove N... | -all")
		}
		if *all {
			edit.RemoveAll = true
			break
		}
		for _, a := range pos {
			n, err := strconv.Atoi(a)
			if err != nil || n < 1 || n > len(cur.Jobs) {
				return fmt.Errorf("no scheduled job %s: they are numbered 1 to %d", a, len(cur.Jobs))
			}
			if *expect != "" && cur.Jobs[n-1].Target != *expect {
				return fmt.Errorf("scheduled job %d grinds %s, not %s: list the schedule again", n, cur.Jobs[n-1].Target, *expect)
			}
			edit.Remove = append(edit.Remove, nodepolicy.GrindRemove{N: n, Job: cur.Jobs[n-1]})
		}
	case "set":
		if len(pos) != 0 {
			return errors.New("poo grind schedule set takes flags only")
		}
		gs := &nodepolicy.GrindSet{}
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "device":
				gs.Device = device
			case "threads":
				gs.Threads = threads
			case "layer-floor":
				gs.LayerFloor = floor
			case "layer-rate":
				gs.LayerRate = rate
			}
		})
		edit.Set = gs
	}
	edit.Version = *ifVersion
	if online {
		r, err := cl.GrindEditSchedule(edit)
		if err != nil {
			return err
		}
		return writeJSON(out, r)
	}
	next, err := edit.Apply(cur)
	if err != nil {
		return err
	}
	p.Grind = &next
	if next.IsZero() {
		p.Grind = nil
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := nodepolicy.Save(path, p); err != nil {
		return err
	}
	return writeJSON(out, rpc.ScheduleReply{Grind: next, Version: next.Version(), Limits: nodepolicy.Limits()})
}

// policySeries returns the series id of the node policy p, or its only
// series when id is empty, for commands that run without a node.
func policySeries(p nodepolicy.Policy, id string) (*series.Series, error) {
	var want hash.Hash
	if id != "" {
		var err error
		if want, err = hash.Parse(id); err != nil {
			return nil, fmt.Errorf("-series: %w", err)
		}
	}
	var found []*series.Series
	for _, si := range p.Series {
		sr, err := series.New(si.Params)
		if err != nil {
			continue
		}
		if id == "" || sr.ID() == want {
			found = append(found, sr)
		}
	}
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) == 0 && id != "":
		return nil, fmt.Errorf("the node policy does not list series %s", want.Short())
	}
	return nil, errors.New("give -series: the node policy lists several series")
}

// parseMixed parses args with flags before, between and after the
// arguments, and returns the arguments.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}
