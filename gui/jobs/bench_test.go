package jobs

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// BenchmarkWrite is one line of output written while followers wait on
// Changed and Read from where they were, as event streams do
// (docs/proposals/gui/ORANGE-002).
func BenchmarkWrite(b *testing.B) {
	line := []byte(strings.Repeat("x", 300) + "\n")
	for _, followers := range []int{0, 1, 4} {
		b.Run(fmt.Sprintf("followers=%d", followers), func(b *testing.B) {
			r := New(context.Background(), Options{Main: func(ctx context.Context, _ []string, _ cli.Stdio) int { <-ctx.Done(); return 0 }})
			defer r.Close(5 * time.Second)
			j, _ := r.Start([]string{"x"})
			stop := make(chan struct{})
			for range followers {
				go func() {
					var from int64
					for {
						st := j.Read(from)
						from = st.Next
						select {
						case <-st.Changed:
						case <-stop:
							return
						}
					}
				}()
			}
			w := j.writer(Stdout)
			for b.Loop() {
				w.Write(line)
			}
			close(stop)
		})
	}
}

// BenchmarkStart is starting a job while the list is full of finished
// ones, as it is after views have refreshed for a while.
func BenchmarkStart(b *testing.B) {
	r := New(context.Background(), Options{Main: func(context.Context, []string, cli.Stdio) int { return 0 }})
	defer r.Close(5 * time.Second)
	for range DefaultMaxJobs {
		j, _ := r.Start([]string{"status"})
		j.Wait(context.Background())
	}
	for b.Loop() {
		j, err := r.StartRefresh([]string{"status"})
		if err != nil {
			b.Fatal(err)
		}
		j.Wait(context.Background())
	}
}
