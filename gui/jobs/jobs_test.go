package jobs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

func wait(t *testing.T, j *Job) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	code, err := j.Wait(ctx)
	if err != nil {
		t.Fatalf("job %v did not finish", j.Args)
	}
	return code
}

// TestSameAsCLI runs commands as jobs and directly through cli.Main and
// requires the same output and exit code: a job is the headless command.
func TestSameAsCLI(t *testing.T) {
	r := New(context.Background(), Options{})
	defer r.Close(5 * time.Second)
	for _, args := range [][]string{
		{"version"},
		{"crypto"},
		{"keygen", "-seed", strings.Repeat("ab", 32)},
		{"series", "example"},
		{"help"},
		{"no-such-command"},
		{"keygen", "-seed", "00"},
	} {
		var out, errOut bytes.Buffer
		want := cli.Main(context.Background(), args, cli.Stdio{Out: &out, Err: &errOut})
		j, err := r.Start(args)
		if err != nil {
			t.Fatal(err)
		}
		if got := wait(t, j); got != want {
			t.Errorf("%v: exit %d, cli.Main %d", args, got, want)
		}
		if j.Output(Stdout) != out.String() || j.Output(Stderr) != errOut.String() {
			t.Errorf("%v: output differs from cli.Main", args)
		}
	}
}

func TestInputAndWaiting(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		fmt.Fprint(s.Out, "passphrase: ")
		b := make([]byte, 64)
		n, _ := s.In.Read(b)
		fmt.Fprintf(s.Out, "got %q", b[:n])
		return 0
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	deadline := time.Now().Add(5 * time.Second)
	for !j.Read(0).Waiting {
		if time.Now().After(deadline) {
			t.Fatal("job never waited for input")
		}
		time.Sleep(time.Millisecond)
	}
	if err := j.Input("secret"); err != nil {
		t.Fatal(err)
	}
	wait(t, j)
	if got := j.Output(Stdout); got != `passphrase: got "secret\n"` {
		t.Fatalf("output %q", got)
	}
	if j.Input("late") == nil {
		t.Fatal("input accepted after the job ended")
	}
}

func TestCancelEndsReadAndCommand(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		_, err := io.ReadAll(s.In) // blocks until cancelled
		if err != nil {
			return 2
		}
		<-ctx.Done()
		return 1
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	j.Cancel()
	if code := wait(t, j); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestReadFollowsAndDrops(t *testing.T) {
	r := New(context.Background(), Options{MaxOutput: 10, Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		for i := range 5 {
			fmt.Fprintf(s.Out, "line%d", i) // 5 bytes each
		}
		fmt.Fprint(s.Err, "bad")
		return 3
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	wait(t, j)
	st := j.Read(0)
	if !st.Dropped || !st.Done || st.Code != 3 || st.Next != 6 {
		t.Fatalf("state %+v", st)
	}
	if len(st.Chunks) != 2 || st.Chunks[0].Data != "line4" || st.Chunks[1].Stream != Stderr {
		t.Fatalf("chunks %+v", st.Chunks)
	}
	if st := j.Read(6); st.Dropped || len(st.Chunks) != 0 {
		t.Fatalf("read past the end: %+v", st)
	}
}

func TestChangedSignals(t *testing.T) {
	release := make(chan struct{})
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		<-release
		fmt.Fprint(s.Out, "x")
		return 0
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	st := j.Read(0)
	close(release)
	select {
	case <-st.Changed:
	case <-time.After(5 * time.Second):
		t.Fatal("no change signalled")
	}
}

func TestTrimKeepsRunningJobs(t *testing.T) {
	block := make(chan struct{})
	r := New(context.Background(), Options{MaxJobs: 2, Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		if len(args) > 0 && args[0] == "block" {
			<-block
		}
		return 0
	}})
	defer r.Close(5 * time.Second)
	long, _ := r.Start([]string{"block"})
	var last *Job
	for range 5 {
		last, _ = r.Start(nil)
		wait(t, last)
	}
	r.Start(nil)
	if _, ok := r.Get(long.ID); !ok {
		t.Fatal("a running job was dropped")
	}
	if n := len(r.List()); n > 3 {
		t.Fatalf("%d jobs kept", n)
	}
	close(block)
}

func TestCloseCancelsAndRefuses(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		<-ctx.Done()
		return 0
	}})
	j, _ := r.Start(nil)
	if err := r.Close(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if !j.Done() {
		t.Fatal("job still running after Close")
	}
	if _, err := r.Start(nil); err != ErrClosed {
		t.Fatalf("Start after Close: %v", err)
	}
}

// TestMaxRunning checks the cap on jobs running at once (GUI-004).
func TestMaxRunning(t *testing.T) {
	block := make(chan struct{})
	r := New(context.Background(), Options{MaxRunning: 2, Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return 0
	}})
	defer r.Close(5 * time.Second)
	a, _ := r.Start(nil)
	if _, err := r.Start(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(nil); err != ErrBusy {
		t.Fatalf("third running job: %v, want ErrBusy", err)
	}
	a.Cancel()
	wait(t, a)
	if _, err := r.Start(nil); err != nil {
		t.Fatalf("after one stopped: %v", err)
	}
	close(block)
}

// TestMaxTotalOutput checks finished jobs are dropped to keep their output
// within the total limit (GUI-004).
func TestMaxTotalOutput(t *testing.T) {
	r := New(context.Background(), Options{MaxTotalOutput: 100, Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		fmt.Fprint(s.Out, strings.Repeat("x", 40))
		return 0
	}})
	defer r.Close(5 * time.Second)
	for range 10 {
		wait(t, must(r.Start(nil)))
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		total := 0
		for _, j := range r.List() {
			total += len(j.Output(Stdout))
		}
		if total <= 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("finished jobs keep %d bytes", total)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func must(j *Job, err error) *Job {
	if err != nil {
		panic(err)
	}
	return j
}

// TestTrimDropsRefreshesFirst checks a view's timer runs never push the
// user's own finished commands out while finished refreshes remain.
func TestTrimDropsRefreshesFirst(t *testing.T) {
	r := New(context.Background(), Options{MaxJobs: 3, Main: func(ctx context.Context, args []string, s cli.Stdio) int { return 0 }})
	defer r.Close(5 * time.Second)
	mine, _ := r.Start([]string{"mine"})
	wait(t, mine)
	for range 10 {
		j, err := r.StartRefresh([]string{"status"})
		if err != nil {
			t.Fatal(err)
		}
		wait(t, j)
	}
	if _, ok := r.Get(mine.ID); !ok {
		t.Fatal("a user's job was dropped while refreshes were kept")
	}
	if n := len(r.List()); n > 3 {
		t.Fatalf("%d jobs kept", n)
	}
	// With only user jobs left, the oldest goes as before.
	for range 3 {
		j, _ := r.Start([]string{"later"})
		wait(t, j)
	}
	// A finished job's trim runs just after it ends, so give it a moment.
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		if _, ok := r.Get(mine.ID); !ok {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the oldest user job was kept past the limit")
		}
	}
}

// TestSecret checks the job's cli.Stdio.Secret: the prompt goes to
// standard error, State.Secret is true only while the secret is read, the
// line is not in the output, and input after it is left for In.
func TestSecret(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		v, err := s.Secret("passphrase: ")
		if err != nil {
			return 2
		}
		line, _ := bufio.NewReader(s.In).ReadString('\n')
		fmt.Fprintf(s.Out, "%d %q", len(v), line)
		return 0
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	deadline := time.Now().Add(5 * time.Second)
	for st := j.Read(0); !st.Waiting || !st.Secret; st = j.Read(0) {
		if time.Now().After(deadline) {
			t.Fatal("job never waited for a secret")
		}
		time.Sleep(time.Millisecond)
	}
	if err := j.Input("hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := j.Input("next"); err != nil {
		t.Fatal(err)
	}
	wait(t, j)
	if got := j.Output(Stdout); got != `7 "next\n"` {
		t.Fatalf("stdout %q", got)
	}
	if got := j.Output(Stderr); got != "passphrase: \n" {
		t.Fatalf("stderr %q", got)
	}
	if j.Read(0).Secret {
		t.Fatal("still reading a secret after the job ended")
	}
}

// TestSecretAskedAgain checks that a second secret read right after the
// first is a new ask: State.Asks counts up, so the page shows its field
// again even if it never saw Waiting false in between (GUI-020's second
// "new passphrase again" prompt).
func TestSecretAskedAgain(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		a, _ := s.Secret("new passphrase: ")
		b, _ := s.Secret("new passphrase again: ")
		fmt.Fprint(s.Out, a == b)
		return 0
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	asked := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for st := j.Read(0); !st.Waiting || st.Asks < n; st = j.Read(0) {
			if time.Now().After(deadline) {
				t.Fatalf("ask %d never came", n)
			}
			time.Sleep(time.Millisecond)
		}
	}
	asked(1)
	if err := j.Input("hunter2"); err != nil {
		t.Fatal(err)
	}
	asked(2)
	if err := j.Input("hunter2"); err != nil {
		t.Fatal(err)
	}
	wait(t, j)
	if got := j.Output(Stdout); got != "true" {
		t.Fatalf("stdout %q", got)
	}
}

func TestSecretEndsOnCancel(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		if _, err := s.Secret("seed: "); err != nil {
			return 3
		}
		return 0
	}})
	defer r.Close(5 * time.Second)
	j, _ := r.Start(nil)
	for !j.Read(0).Waiting {
		time.Sleep(time.Millisecond)
	}
	j.Cancel()
	if code := wait(t, j); code != 3 {
		t.Fatalf("code %d", code)
	}
}

// TestViewOnly runs jobs through the core with Options.ViewOnly: reads
// run as they do headless, and a command that changes something is
// refused by the core itself, exactly as poo -view-only refuses it.
func TestViewOnly(t *testing.T) {
	r := New(context.Background(), Options{ViewOnly: true})
	defer r.Close(5 * time.Second)
	if !r.ViewOnly() {
		t.Fatal("ViewOnly() is false")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"version"},
		{"crypto"},
		{"init", "-root", dir},
		{"claim", "-root", dir},
	} {
		var out, errOut bytes.Buffer
		want := cli.Main(context.Background(), append([]string{"-view-only"}, args...), cli.Stdio{Out: &out, Err: &errOut})
		j, err := r.Start(args)
		if err != nil {
			t.Fatal(err)
		}
		if got := wait(t, j); got != want {
			t.Errorf("%v: exit %d, poo -view-only %d", args, got, want)
		}
		if j.Output(Stdout) != out.String() || j.Output(Stderr) != errOut.String() {
			t.Errorf("%v: output differs from poo -view-only", args)
		}
	}
	j, _ := r.Start([]string{"init", "-root", dir})
	if wait(t, j) == 0 || !strings.Contains(j.Output(Stderr), "view-only") {
		t.Fatalf("init was not refused: %q", j.Output(Stderr))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("view-only init wrote %d files", len(entries))
	}
}

// TestSecretOutputGivenOnce is GUI-009: once a command that prints a
// secret has ended, its output goes to the first reader that reads it to
// the end and is then dropped; a status check does not count as reading.
func TestSecretOutputGivenOnce(t *testing.T) {
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		fmt.Fprintln(s.Out, "SECRET-SEED")
		return 0
	}})
	defer r.Close(5 * time.Second)
	j := must(r.Start([]string{"wallet", "seed"}))
	wait(t, j)
	if st := j.Read(1 << 62); st.Withheld {
		t.Fatal("a status check dropped the output")
	}
	st := j.Read(0)
	if len(st.Chunks) != 1 || !strings.Contains(st.Chunks[0].Data, "SECRET-SEED") || !st.Done {
		t.Fatalf("first read: %+v", st)
	}
	st = j.Read(0)
	if len(st.Chunks) != 0 || !st.Dropped || !st.Withheld || !j.Withheld() {
		t.Fatalf("second read gave the secret again: %+v", st)
	}
	if out, _ := j.Outputs(); out != "" {
		t.Fatalf("Outputs after: %q", out)
	}

	// Outputs, as /api/run reads it, also counts as reading.
	k := must(r.Start([]string{"keygen"}))
	wait(t, k)
	if out, _ := k.Outputs(); !strings.Contains(out, "SECRET-SEED") {
		t.Fatalf("Outputs: %q", out)
	}
	if st := k.Read(0); len(st.Chunks) != 0 || !st.Withheld {
		t.Fatalf("read after Outputs: %+v", st)
	}

	// Other commands keep their output.
	o := must(r.Start([]string{"status"}))
	wait(t, o)
	o.Read(0)
	if st := o.Read(0); len(st.Chunks) != 1 || st.Withheld {
		t.Fatalf("an ordinary job lost its output: %+v", st)
	}
}

// TestSecretOutputExpires checks a secret output no one reads is dropped
// after SecretKeep.
func TestSecretOutputExpires(t *testing.T) {
	defer func(d time.Duration) { SecretKeep = d }(SecretKeep)
	SecretKeep = 10 * time.Millisecond
	r := New(context.Background(), Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		fmt.Fprintln(s.Out, "SECRET-SEED")
		return 0
	}})
	defer r.Close(5 * time.Second)
	j := must(r.Start([]string{"keygen"}))
	wait(t, j)
	deadline := time.Now().Add(5 * time.Second)
	for !j.Withheld() {
		if time.Now().After(deadline) {
			t.Fatal("never dropped")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
