// Command perfgui serves the real GUI page and API over a job runner whose
// poon is partly simulated, so the page can be measured under loads a test
// node cannot produce on demand (docs/proposals/gui/ORANGE-001). It is a
// measuring tool only: it is not released and poon-gui never uses it.
//
//	go -C gui run ./perf/perfgui [-rate N] [-events N] [-log N]
//
// It prints a sign-in address. Commands run by the real cli.Main, except:
//
//	watch  prints -events watch events, -rate per second (0: as fast as
//	       it can), in the shape poon watch prints, then waits until stopped
//	node   prints -log log lines a second until stopped, like a busy node
//	grind run  prints a plan, then -events progress lines, -rate per second,
//	       with a found line before every -found'th, like poon grind run
//	big    prints -events lines of output and exits (a large result)
//	bigjson prints a JSON list of -events rows like poon unspent's and exits
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/server"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/web"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

var (
	rate       = flag.Int("rate", 2000, "watch events a second; 0 for as fast as possible")
	events     = flag.Int("events", 20000, "watch events printed by one watch")
	logRate    = flag.Int("log", 50, "node log lines a second")
	listen     = flag.String("listen", "", "address to listen on")
	foundEvery = flag.Int("found", 4, "grind run: a found line before every n'th progress line")
)

func main() {
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ln, err := server.Listen(*listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runner := jobs.New(ctx, jobs.Options{Main: fakeMain})
	srv := server.New(runner, web.FS(), ln.Addr().String())
	u, err := srv.SignInURL(10 * time.Minute)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("perfgui: open %s\n", u)
	srv.Serve(ctx, ln)
	runner.Close(5 * time.Second)
}

func fakeMain(ctx context.Context, args []string, s cli.Stdio) int {
	if len(args) > 0 {
		switch args[0] {
		case "grind":
			if len(args) > 1 && args[1] == "run" {
				return fakeGrind(ctx, s)
			}
		case "watch":
			return fakeWatch(ctx, s)
		case "node":
			return fakeNode(ctx, s)
		case "bigjson":
			// A large JSON result, shaped like poon unspent's rows.
			rows := make([]map[string]any, *events)
			for i := range rows {
				rows[i] = map[string]any{"pool": i % 4, "serial": i % 100, "address": hx(i, 32), "tid": hx(i+1, 32), "portion": i % 7, "amount": fmt.Sprint(i + 1), "height": i / 6, "entry": hx(i+2, 32), "spent": false}
			}
			json.NewEncoder(s.Out).Encode(rows)
			return 0
		case "big":
			for i := 0; i < *events; i++ {
				fmt.Fprintf(s.Out, "line %d of a large result %s\n", i, hx(i, 24))
			}
			return 0
		}
	}
	return cli.Main(ctx, args, s)
}

func hx(i, n int) string {
	b := make([]byte, n)
	for k := range b {
		b[k] = byte(i*31 + k*7)
	}
	return hex.EncodeToString(b)
}

// event is one line in the shape poon watch prints (pkg/watch.Event).
func event(i int) []byte {
	kinds := []string{"tx", "tx", "tx", "entry", "layer", "selected"}
	k := kinds[i%len(kinds)]
	ev := map[string]any{
		"seq": i + 1, "time": time.Now().UTC().Format(time.RFC3339Nano), "kind": k,
		"series": hx(1, 32), "pool": i % 4, "weight": fmt.Sprint(1000 + i), "origin": "peer", "from": "127.0.0.1:7001",
	}
	switch k {
	case "tx":
		ev["tid"] = hx(i, 32)
		ev["tx"] = map[string]any{"serial": i % 100, "inputs": []string{hx(i+1, 32)}, "outputs": []map[string]any{{"address": hx(i+2, 32), "amount": "1"}}}
	case "entry", "selected":
		ev["entry"] = map[string]any{"hash": hx(i, 32), "parent": hx(i-1, 32), "height": i / 6, "tids": []string{hx(i-1, 32), hx(i-2, 32)}, "selected": k == "selected"}
	case "layer":
		ev["layer"] = map[string]any{"entry": hx(i-1, 32), "hash": hx(i, 32), "nonce": i, "strength": 3, "credit": "12"}
	}
	b, _ := json.Marshal(ev)
	return append(b, '\n')
}

// fakeWatch writes one event per Write, as poon watch's json.Encoder does.
func fakeWatch(ctx context.Context, s cli.Stdio) int {
	start := time.Now()
	for i := 0; i < *events; i++ {
		if ctx.Err() != nil {
			return 0
		}
		if *rate > 0 {
			if d := time.Until(start.Add(time.Duration(i) * time.Second / time.Duration(*rate))); d > 0 {
				time.Sleep(d)
			}
		}
		s.Out.Write(event(i))
	}
	<-ctx.Done()
	return 0
}

func fakeNode(ctx context.Context, s cli.Stdio) int {
	fmt.Fprintln(s.Err, "poon node: 1 pools followed (fake, for measuring)")
	t := time.NewTicker(time.Second / time.Duration(max(*logRate, 1)))
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return 0
		case <-t.C:
			fmt.Fprintf(s.Err, "%s poon node: peer 127.0.0.1:%d sent entry %s at height %d\n", time.Now().Format(time.RFC3339), 7000+i%9, hx(i, 32), i)
		}
	}
}

// fakeGrind writes events in the shape poon grind run prints
// (pkg/grind.Event), one per Write, then waits until stopped.
func fakeGrind(ctx context.Context, s cli.Stdio) int {
	enc := json.NewEncoder(s.Out)
	enc.Encode(map[string]any{"job": 1, "kind": "start", "state": "running", "series": hx(1, 32),
		"plan": map[string]any{"device": "cpu", "hash": "sha256", "threads": 3, "grinder": "cpu-sha256", "why": "auto: the fastest device for sha256 on this machine"}})
	start := time.Now()
	for i := 0; i < *events; i++ {
		if ctx.Err() != nil {
			return 0
		}
		if *rate > 0 {
			if d := time.Until(start.Add(time.Duration(i) * time.Second / time.Duration(*rate))); d > 0 {
				time.Sleep(d)
			}
		}
		r := 2.5e6 + float64(i%37)*1e4
		if *foundEvery > 0 && i%*foundEvery == 0 {
			enc.Encode(map[string]any{"job": 1, "kind": "found", "series": hx(1, 32), "pool": i % 4, "tries": i * 1000, "rate": r,
				"layer": hx(i, 32), "entry": hx(i+1, 32), "strength": 8 + i%5})
		}
		enc.Encode(map[string]any{"job": 1, "kind": "progress", "series": hx(1, 32), "pool": i % 4, "state": "running", "tries": i * 1000, "rate": r})
	}
	<-ctx.Done()
	return 0
}
