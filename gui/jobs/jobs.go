// Package jobs runs poo command lines for the GUI. A job is one call of
// cli.Main, the function the poo binary runs, with its own context, input
// and output: the GUI's terminal, views and forms all run commands this
// way and no other (docs/gui.md).
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/shell"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// SecretKeep is how long a job whose output holds a secret (shell.
// SecretOutput) keeps that output after it ends, when no one reads it to
// the end sooner (GUI-009 in security/gui/findings).
var SecretKeep = time.Minute

// Limits on what a runner runs and keeps (GUI-004 in
// security/gui/findings).
const (
	DefaultMaxOutput      = 4 << 20  // bytes of output kept per job
	DefaultMaxJobs        = 64       // jobs kept, finished ones dropped first
	DefaultMaxRunning     = 16       // jobs running at once
	DefaultMaxTotalOutput = 64 << 20 // bytes of output kept by finished jobs
)

// Stream names a chunk's stream.
type Stream string

// The two output streams.
const (
	Stdout Stream = "out"
	Stderr Stream = "err"
)

// Chunk is one write of a job's output. Seq counts chunks from 0.
type Chunk struct {
	Seq    int64  `json:"seq"`
	Stream Stream `json:"stream"`
	Data   string `json:"data"`
}

// MainFunc is the signature of cli.Main.
type MainFunc func(ctx context.Context, args []string, s cli.Stdio) int

// Runner starts and keeps jobs.
type Runner struct {
	ctx       context.Context
	cancel    context.CancelFunc
	main      MainFunc
	maxOutput int
	maxJobs   int
	maxRun    int
	maxTotal  int
	keepNew   time.Duration
	viewOnly  bool

	mu    sync.Mutex
	next  int
	jobs  map[string]*Job
	order []string // ids, oldest first
	wg    sync.WaitGroup
}

// Options adjust a runner; zero values mean the defaults.
type Options struct {
	MaxOutput      int
	MaxJobs        int
	MaxRunning     int
	MaxTotalOutput int
	// KeepNew keeps a finished job started less than this long ago from
	// being dropped, so a page that has just started it can still follow
	// it: a quick refresh could otherwise end and be dropped before its
	// events were asked for. Zero keeps no job that way.
	KeepNew time.Duration
	// ViewOnly runs every job in the core's view-only mode
	// (cli.Stdio.ViewOnly, decision D101): commands dial the node with
	// its view token and anything that changes something is refused.
	ViewOnly bool
	// Main replaces cli.Main; only tests set it.
	Main MainFunc
}

// New returns a runner whose jobs are cancelled when ctx is done or the
// runner is closed.
func New(ctx context.Context, o Options) *Runner {
	r := &Runner{main: cli.Main, maxOutput: DefaultMaxOutput, maxJobs: DefaultMaxJobs, maxRun: DefaultMaxRunning, maxTotal: DefaultMaxTotalOutput, jobs: map[string]*Job{}}
	if o.Main != nil {
		r.main = o.Main
	}
	if o.MaxOutput > 0 {
		r.maxOutput = o.MaxOutput
	}
	if o.MaxJobs > 0 {
		r.maxJobs = o.MaxJobs
	}
	if o.MaxRunning > 0 {
		r.maxRun = o.MaxRunning
	}
	if o.MaxTotalOutput > 0 {
		r.maxTotal = o.MaxTotalOutput
	}
	r.keepNew = o.KeepNew
	r.viewOnly = o.ViewOnly
	r.ctx, r.cancel = context.WithCancel(ctx)
	return r
}

// ViewOnly reports whether the runner's jobs run in view-only mode.
func (r *Runner) ViewOnly() bool { return r.viewOnly }

// ErrClosed is returned by Start after Close.
var ErrClosed = errors.New("jobs: runner closed")

// ErrBusy is returned by Start while the most jobs allowed are running.
var ErrBusy = errors.New("jobs: too many commands running; stop one first")

// Start runs args (without the program name) as a new job.
func (r *Runner) Start(args []string) (*Job, error) { return r.start(args, false) }

// StartRefresh runs args as a refresh: a view re-running a command on a
// timer. Finished refreshes are dropped before any other finished job, so
// they never push the user's own commands out of the list (GREEN-011).
func (r *Runner) StartRefresh(args []string) (*Job, error) { return r.start(args, true) }

func (r *Runner) start(args []string, refresh bool) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return nil, ErrClosed
	}
	if r.running() >= r.maxRun {
		return nil, ErrBusy
	}
	r.next++
	ctx, cancel := context.WithCancel(r.ctx)
	j := &Job{
		ID:           strconv.Itoa(r.next),
		Args:         slices.Clone(args),
		Started:      time.Now(),
		Refresh:      refresh,
		SecretOutput: shell.SecretOutput(args),
		cancel:       cancel,
		max:          r.maxOutput,
		changed:      make(chan struct{}),
		done:         make(chan struct{}),
	}
	j.in = newInput(ctx)
	j.in.onWait = j.touch
	r.jobs[j.ID] = j
	r.order = append(r.order, j.ID)
	r.trim()
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		code := r.main(ctx, j.Args, cli.Stdio{In: j.in, Out: j.writer(Stdout), Err: j.writer(Stderr), Secret: j.readSecret, ViewOnly: r.viewOnly})
		cancel()
		j.finish(code)
		r.mu.Lock()
		r.trim()
		r.mu.Unlock()
	}()
	return j, nil
}

func (r *Runner) running() int {
	n := 0
	for _, j := range r.jobs {
		if !j.Done() {
			n++
		}
	}
	return n
}

// trim drops the oldest finished jobs while more jobs are kept than the
// limit, or finished jobs keep more output than the total limit. Running
// jobs are never dropped; their output is bounded per job and their
// number by MaxRunning.
func (r *Runner) trim() {
	total := 0
	for _, j := range r.jobs {
		if j.Done() {
			total += j.outputSize()
		}
	}
	// Finished refreshes go first, then other finished jobs, oldest first.
	// A job started within keepNew stays until a later trim.
	now := time.Now()
	for _, refreshOnly := range []bool{true, false} {
		for i := 0; (len(r.order) > r.maxJobs || total > r.maxTotal) && i < len(r.order); {
			id := r.order[i]
			if j := r.jobs[id]; j.Done() && (j.Refresh || !refreshOnly) && now.Sub(j.Started) >= r.keepNew {
				total -= j.outputSize()
				delete(r.jobs, id)
				r.order = slices.Delete(r.order, i, i+1)
				continue
			}
			i++
		}
	}
}

// Get returns the job with id, if kept.
func (r *Runner) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

// List returns the kept jobs, oldest first.
func (r *Runner) List() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.jobs[id])
	}
	return out
}

// Close cancels every job and waits for them to return, at most timeout.
// A cancelled poo node closes its stores before it returns, so callers
// should give it a few seconds.
func (r *Runner) Close(timeout time.Duration) error {
	r.mu.Lock()
	r.cancel()
	r.mu.Unlock()
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("jobs: still running after %v", timeout)
	}
}

// Job is one run of a command line.
type Job struct {
	ID      string
	Args    []string
	Started time.Time
	Refresh bool // started by StartRefresh

	cancel context.CancelFunc
	in     *input
	max    int
	secret bool // a passphrase or seed is being read (readSecret)
	// SecretOutput is true for a command that prints a secret, such as
	// wallet seed: once the job has ended, its output is given out once,
	// to the first reader that reads it to the end, and then dropped.
	SecretOutput bool
	withheld     bool // that output was dropped

	mu      sync.Mutex
	chunks  []Chunk
	size    int   // bytes in chunks
	first   int64 // seq of chunks[0]
	nextSeq int64
	changed chan struct{} // closed and replaced on every change
	done    chan struct{}
	code    int
}

// Cancel stops the job, as an interrupt stops a command in a shell.
func (j *Job) Cancel() { j.cancel() }

// Input sends one line to the job's input. It does not block.
func (j *Job) Input(line string) error { return j.in.add(line + "\n") }

// InputPart sends text to the job's input with no end of line: the start
// of a line too long for one request, which the next Input ends
// (ORANGE-013).
func (j *Job) InputPart(text string) error { return j.in.add(text) }

// EOF ends the job's input, as Ctrl-D does in a shell: the command reads
// what was sent, then io.EOF (GUI-021). Cancel is not an end of input.
func (j *Job) EOF() error { return j.in.end() }

// Done reports whether the job has returned.
func (j *Job) Done() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// Wait waits for the job to return and gives its exit code.
func (j *Job) Wait(ctx context.Context) (int, error) {
	select {
	case <-j.done:
		return j.code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// State is a view of a job's output from some sequence number on.
type State struct {
	Chunks []Chunk
	// Dropped is true when output from the asked sequence number on was
	// dropped to stay within the limit; Chunks then starts later.
	Dropped bool
	Next    int64 // the sequence number to ask for next
	Done    bool
	Code    int // the exit code, once Done
	// Waiting is true while the job is blocked reading its input.
	Waiting bool
	// Asks counts the times the job has started waiting for input, so
	// a second prompt right after the first, such as a new passphrase
	// asked again, is a change even if Waiting never read false between.
	Asks int
	// Secret is true while what it waits for is a passphrase or a seed,
	// so the GUI masks that line and only that line.
	Secret bool
	// Withheld is true when the output was dropped because it held a
	// secret; Dropped is then true for any read of it.
	Withheld bool
	// Changed is closed when anything above changes.
	Changed <-chan struct{}
}

// Read returns the output from sequence number from on.
func (j *Job) Read(from int64) State {
	j.mu.Lock()
	defer j.mu.Unlock()
	waiting, asks := j.in.waits()
	s := State{Next: j.nextSeq, Changed: j.changed, Waiting: waiting, Asks: asks, Secret: j.secret}
	// A read from past the end, such as a status check, reads no output.
	reads := from <= j.nextSeq
	if from < j.first {
		s.Dropped, from = true, j.first
	}
	if from < j.nextSeq {
		s.Chunks = slices.Clone(j.chunks[from-j.first:])
	}
	select {
	case <-j.done:
		s.Done, s.Code = true, j.code
		// This reader has the output to the end, so a secret in it is
		// not given out again.
		if j.SecretOutput && reads {
			j.withhold()
		}
	default:
	}
	s.Withheld = j.withheld
	return s
}

// withhold drops the output of a job whose output holds a secret.
func (j *Job) withhold() {
	if j.withheld {
		return
	}
	j.withheld = true
	j.first = j.nextSeq
	j.chunks, j.size = nil, 0
	j.notify()
}

// Withheld reports whether the job's output was dropped because it held a
// secret.
func (j *Job) Withheld() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.withheld
}

// Outputs returns all kept output of both streams, as Output does, and
// counts as reading it to the end.
func (j *Job) Outputs() (out, errOut string) {
	out, errOut = j.Output(Stdout), j.Output(Stderr)
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.SecretOutput && j.Done() {
		j.withhold()
	}
	return out, errOut
}

// Output returns all kept output of one stream, joined.
func (j *Job) Output(stream Stream) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var b []byte
	for _, c := range j.chunks {
		if c.Stream == stream {
			b = append(b, c.Data...)
		}
	}
	return string(b)
}

func (j *Job) outputSize() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.size
}

func (j *Job) notify() {
	close(j.changed)
	j.changed = make(chan struct{})
}

func (j *Job) write(stream Stream, p []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.chunks = append(j.chunks, Chunk{Seq: j.nextSeq, Stream: stream, Data: string(p)})
	j.nextSeq++
	j.size += len(p)
	for j.size > j.max && len(j.chunks) > 1 {
		j.size -= len(j.chunks[0].Data)
		j.chunks = j.chunks[1:]
		j.first++
	}
	j.notify()
}

func (j *Job) finish(code int) {
	j.in.close()
	j.mu.Lock()
	defer j.mu.Unlock()
	j.code = code
	close(j.done)
	j.notify()
	if j.SecretOutput {
		time.AfterFunc(SecretKeep, func() {
			j.mu.Lock()
			defer j.mu.Unlock()
			j.withhold()
		})
	}
}

// readSecret is the job's cli.Stdio.Secret: the core calls it for every
// passphrase and seed (decision D101). Like the headless terminal, it
// writes the prompt to standard error and reads one line, which is never
// written to the output; while it waits, State.Secret is true.
func (j *Job) readSecret(prompt string) (string, error) {
	j.setSecret(true)
	defer j.setSecret(false)
	j.write(Stderr, []byte(prompt))
	line, err := j.in.readLine()
	j.write(Stderr, []byte("\n"))
	return line, err
}

func (j *Job) setSecret(on bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.secret = on
	j.notify()
}

// touch signals a change that is not output, such as the job starting or
// stopping to wait for input.
func (j *Job) touch() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.notify()
}

type streamWriter struct {
	j      *Job
	stream Stream
}

func (w streamWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.j.write(w.stream, p)
	}
	return len(p), nil
}

func (j *Job) writer(s Stream) io.Writer { return streamWriter{j, s} }

// input is a job's standard input: lines sent from the GUI, read by the
// command. A read blocks until a line arrives. It returns io.EOF after an
// end of input (EOF), and an error, not io.EOF, once the job is cancelled
// or has returned, so a command never waits forever. A command that
// reads to the end, such as submit -f -, then fails rather than act on
// what was sent before Stop, as Ctrl-C does in a shell (GUI-021).
type input struct {
	ctx    context.Context
	mu     sync.Mutex
	buf    []byte
	closed bool
	eof    bool // ended by EOF: buf is still read, then io.EOF
	nwait  int
	asks   int // the times a read started waiting
	wake   chan struct{}
	onWait func() // called when a reader starts or stops waiting
}

func newInput(ctx context.Context) *input {
	in := &input{ctx: ctx, wake: make(chan struct{})}
	go func() {
		<-ctx.Done()
		in.close()
	}()
	return in
}

var (
	errInputClosed = errors.New("jobs: the job has ended")
	errInputEnded  = errors.New("jobs: the job's input has ended")
	// errStopped is what a read returns once the job is stopped or has
	// returned: not io.EOF, so it never reads as the end of what was sent.
	errStopped = fmt.Errorf("input: the job was stopped: %w", context.Canceled)
)

func (in *input) add(s string) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return errInputClosed
	}
	if in.eof {
		return errInputEnded
	}
	in.buf = append(in.buf, s...)
	close(in.wake)
	in.wake = make(chan struct{})
	return nil
}

// end marks the end of input: what was sent is still read, then io.EOF.
func (in *input) end() error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return errInputClosed
	}
	if !in.eof {
		in.eof = true
		close(in.wake)
		in.wake = make(chan struct{})
	}
	return nil
}

func (in *input) close() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return
	}
	in.closed = true
	in.buf = nil // a passphrase sent but never read is not kept
	close(in.wake)
}

// waits reports whether a read waits now, and how many have started to.
func (in *input) waits() (bool, int) {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.nwait > 0, in.asks
}

// errNoSecret is what readLine returns when the job ends first.
var errNoSecret = errors.New("nothing was entered")

// readLine reads one line, without its line ending, and nothing after it,
// so what the command reads next is left in place.
func (in *input) readLine() (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		if _, err := in.Read(b); err != nil {
			if len(line) == 0 {
				return "", errNoSecret
			}
			return string(line), nil
		}
		if b[0] == '\n' {
			return strings.TrimSuffix(string(line), "\r"), nil
		}
		line = append(line, b[0])
	}
}

func (in *input) Read(p []byte) (int, error) {
	in.mu.Lock()
	for len(in.buf) == 0 && !in.closed && !in.eof {
		wake := in.wake
		in.nwait++
		in.asks++
		cb := in.onWait
		in.mu.Unlock()
		if cb != nil {
			cb()
		}
		<-wake
		in.mu.Lock()
		in.nwait--
		cb = in.onWait
		if cb != nil {
			in.mu.Unlock()
			cb()
			in.mu.Lock()
		}
	}
	defer in.mu.Unlock()
	if in.closed {
		return 0, errStopped
	}
	if len(in.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, in.buf)
	in.buf = in.buf[n:]
	return n, nil
}
