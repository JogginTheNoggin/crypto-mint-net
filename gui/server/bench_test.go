package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// benchServer is a signed-in server whose jobs run main, for the
// benchmarks below (docs/proposals/gui/ORANGE-002).
func benchServer(b *testing.B, main jobs.MainFunc) (*httptest.Server, *http.Cookie) {
	b.Helper()
	runner := jobs.New(context.Background(), jobs.Options{Main: main, MaxJobs: 1 << 20})
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	s := New(runner, fstest.MapFS{"index.html": {Data: []byte("page")}}, srv.Listener.Addr().String())
	srv.Config.Handler = s
	b.Cleanup(func() { srv.Close(); runner.Close(5 * time.Second) })
	u, _ := s.SignInURL(time.Minute)
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(u)
	if err != nil {
		b.Fatal(err)
	}
	resp.Body.Close()
	for _, k := range resp.Cookies() {
		if k.Name == CookieName {
			return srv, k
		}
	}
	b.Fatal("no cookie")
	return nil, nil
}

func benchPost(b *testing.B, srv *httptest.Server, ck *http.Cookie, path, body string) *http.Response {
	req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
	req.AddCookie(ck)
	req.Header.Set(Header, "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.Fatal(err)
	}
	return resp
}

// BenchmarkRunCommand is one short command through POST /api/run: the
// GUI's own cost per command, around cli.Main.
func BenchmarkRunCommand(b *testing.B) {
	for _, c := range []struct {
		name string
		main jobs.MainFunc
	}{
		{"noop", func(ctx context.Context, args []string, s cli.Stdio) int { fmt.Fprintln(s.Out, "{}"); return 0 }},
		{"poo-crypto", cli.Main},
	} {
		b.Run(c.name, func(b *testing.B) {
			srv, ck := benchServer(b, c.main)
			for b.Loop() {
				resp := benchPost(b, srv, ck, "/api/run", `{"args":["crypto"]}`)
				resp.Body.Close()
			}
		})
	}
}

// BenchmarkStream is a job printing lines as fast as it can, read
// through GET /api/jobs/{id}/events, as the Watch view reads poo watch.
func BenchmarkStream(b *testing.B) {
	line := `{"seq":1,"kind":"tx","pool":0,"tid":"` + strings.Repeat("ab", 32) + `","weight":"1000","origin":"peer"}` + "\n"
	for _, n := range []int{10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			srv, ck := benchServer(b, func(ctx context.Context, args []string, s cli.Stdio) int {
				for i := 0; i < n; i++ {
					s.Out.Write([]byte(line))
				}
				<-ctx.Done()
				return 0
			})
			var events, bytes, lines int
			for b.Loop() {
				resp := benchPost(b, srv, ck, "/api/jobs", `{"args":["watch"]}`)
				var j struct{ ID string }
				json.NewDecoder(resp.Body).Decode(&j)
				resp.Body.Close()
				req, _ := http.NewRequest("GET", srv.URL+"/api/jobs/"+j.ID+"/events?from=0", nil)
				req.AddCookie(ck)
				ev, err := http.DefaultClient.Do(req)
				if err != nil {
					b.Fatal(err)
				}
				sc := bufio.NewScanner(ev.Body)
				sc.Buffer(make([]byte, 1<<20), 1<<24)
				got := 0
				for got < n && sc.Scan() {
					t := sc.Text()
					bytes += len(t) + 1
					if t == "event: chunk" {
						events++
					}
					if strings.HasPrefix(t, "data: {\"seq\"") {
						var c jobs.Chunk
						json.Unmarshal([]byte(t[6:]), &c)
						k := strings.Count(c.Data, "\n")
						got += k
						lines += k
					}
				}
				ev.Body.Close()
				benchPost(b, srv, ck, "/api/jobs/"+j.ID+"/cancel", "").Body.Close()
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(lines), "ns/line")
			b.ReportMetric(float64(events)/float64(lines), "events/line")
			b.ReportMetric(float64(bytes)/float64(lines), "bytes/line")
		})
	}
}
