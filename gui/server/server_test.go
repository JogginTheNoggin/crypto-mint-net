package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/shell"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	s      *Server
	cookie *http.Cookie
}

func newHarness(t *testing.T, o jobs.Options) *harness {
	t.Helper()
	runner := jobs.New(context.Background(), o)
	secret, _ := NewSecret()
	static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>page")}}
	h := &harness{t: t}
	h.srv = httptest.NewUnstartedServer(nil)
	h.srv.Start()
	h.s = New(runner, static, secret, h.srv.Listener.Addr().String())
	h.srv.Config.Handler = h.s
	h.cookie = &http.Cookie{Name: CookieName, Value: secret}
	t.Cleanup(func() { h.srv.Close(); runner.Close(5 * time.Second) })
	return h
}

func (h *harness) do(method, path string, body any, mod func(*http.Request)) *http.Response {
	h.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	req.AddCookie(h.cookie)
	if method == "POST" {
		req.Header.Set(Header, "1")
	}
	if mod != nil {
		mod(req)
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestListenLoopbackOnly(t *testing.T) {
	for _, a := range []string{"0.0.0.0:0", "192.168.1.5:0", ":0", "example.com:80"} {
		if ln, err := Listen(a); err == nil {
			ln.Close()
			t.Errorf("Listen(%q) accepted a non-loopback address", a)
		}
	}
	for _, a := range []string{"", "127.0.0.1:0", "localhost:0"} {
		ln, err := Listen(a)
		if err != nil {
			t.Errorf("Listen(%q): %v", a, err)
			continue
		}
		ln.Close()
	}
}

func TestSignIn(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	noCookie := func(r *http.Request) { r.Header.Del("Cookie") }
	if r := h.do("GET", "/", nil, noCookie); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("page without cookie: %d", r.StatusCode)
	}
	if r := h.do("GET", "/api/jobs", nil, noCookie); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api without cookie: %d", r.StatusCode)
	}
	if r := h.do("GET", "/?t=wrong", nil, noCookie); r.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong secret: %d", r.StatusCode)
	}
	r := h.do("GET", "/?t="+h.cookie.Value, nil, noCookie)
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/" {
		t.Fatalf("secret: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	var set *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == CookieName {
			set = c
		}
	}
	if set == nil || set.Value != h.cookie.Value || !set.HttpOnly || set.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", set)
	}
	if r := h.do("GET", "/", nil, nil); r.StatusCode != http.StatusOK || !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("page: %d, csp %q", r.StatusCode, r.Header.Get("Content-Security-Policy"))
	}
	if h.s.URL() != h.srv.URL+"/?t="+h.cookie.Value {
		t.Fatalf("URL %s", h.s.URL())
	}
}

func TestRequestGuards(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	body := map[string]any{"args": []string{"version"}}
	if r := h.do("GET", "/api/jobs", nil, func(r *http.Request) { r.Host = "evil.example:80" }); r.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("wrong host: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/run", body, func(r *http.Request) { r.Header.Del(Header) }); r.StatusCode != http.StatusForbidden {
		t.Fatalf("no header: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/run", body, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }); r.StatusCode != http.StatusForbidden {
		t.Fatalf("cross origin: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/run", body, func(r *http.Request) { r.Header.Set("Origin", h.srv.URL) }); r.StatusCode != http.StatusOK {
		t.Fatalf("same origin: %d", r.StatusCode)
	}
	if r := h.do("GET", "/api/run", nil, nil); r.StatusCode == http.StatusOK {
		t.Fatalf("GET run: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/run", map[string]any{}, nil); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("no command: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/run", map[string]any{"line": "'open"}, nil); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad line: %d", r.StatusCode)
	}
}

// TestRunSameAsCLI is the GUI's promise: a command run through the GUI's
// API, as args or as a typed line, gives the same bytes and exit code as
// cli.Main, the function the poo binary runs.
func TestRunSameAsCLI(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	for _, line := range []string{
		"version",
		"poo crypto",
		"keygen -seed " + strings.Repeat("cd", 32) + " -alg ml-dsa-44 -hash blake3",
		"series example",
		"series new -mints 100",
		"help",
		"nope",
		"keygen -seed 00",
		`wallet seed -wallet '/no/such dir/w.json'`,
	} {
		args, _ := splitForTest(line)
		var out, errOut bytes.Buffer
		want := cli.Main(context.Background(), args, cli.Stdio{Out: &out, Err: &errOut})
		for _, body := range []map[string]any{{"line": line}, {"args": args}} {
			var got struct {
				Code     int
				Out, Err string
				Command  string
			}
			decode(t, h.do("POST", "/api/run", body, nil), &got)
			if got.Code != want || got.Out != out.String() || got.Err != errOut.String() {
				t.Errorf("%s via %v: code %d out %q err %q; cli.Main code %d out %q err %q",
					line, body, got.Code, trim(got.Out), got.Err, want, trim(out.String()), errOut.String())
			}
		}
	}
}

func trim(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func splitForTest(line string) ([]string, error) { return shell.Split(line) }

func TestJobStreamAndInput(t *testing.T) {
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		s.Out.Write([]byte("passphrase: "))
		line, _ := bufio.NewReader(s.In).ReadString('\n')
		s.Out.Write([]byte("hello " + line))
		s.Err.Write([]byte("bye\n"))
		return 4
	}})
	var j struct {
		ID      string
		Command string
	}
	r := h.do("POST", "/api/jobs", map[string]any{"line": "poo wallet seed -memo 'a b'"}, nil)
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("start: %d", r.StatusCode)
	}
	decode(t, r, &j)
	if j.Command != "poo wallet seed -memo 'a b'" {
		t.Fatalf("command %q", j.Command)
	}
	resp := h.do("GET", "/api/jobs/"+j.ID+"/events", nil, nil)
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	var events []string
	var datas []string
	sentInput := false
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "event: "):
			events = append(events, l[7:])
		case strings.HasPrefix(l, "data: "):
			d := l[6:]
			datas = append(datas, d)
			if d == `{"waiting":true}` && !sentInput {
				sentInput = true
				if r := h.do("POST", "/api/jobs/"+j.ID+"/input", map[string]string{"line": "world"}, nil); r.StatusCode != http.StatusNoContent {
					t.Fatalf("input: %d", r.StatusCode)
				}
			}
		}
		if len(events) > 0 && events[len(events)-1] == "exit" && len(datas) == len(events) {
			break
		}
	}
	all := strings.Join(datas, "\n")
	for _, want := range []string{`"data":"passphrase: "`, `{"waiting":true}`, `"data":"hello world\n"`, `"stream":"err","data":"bye\n"`, `{"code":4}`} {
		if !strings.Contains(all, want) {
			t.Errorf("events lack %s:\n%s", want, all)
		}
	}
	if events[len(events)-1] != "exit" {
		t.Fatalf("last event %s", events[len(events)-1])
	}
	// Input to a finished job is refused; an unknown job is not found.
	if r := h.do("POST", "/api/jobs/"+j.ID+"/input", map[string]string{"line": "x"}, nil); r.StatusCode != http.StatusConflict {
		t.Fatalf("late input: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/jobs/999/cancel", nil, nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown job: %d", r.StatusCode)
	}
	var list []struct {
		ID   string
		Done bool
		Code *int
	}
	decode(t, h.do("GET", "/api/jobs", nil, nil), &list)
	if len(list) != 1 || !list[0].Done || list[0].Code == nil || *list[0].Code != 4 {
		t.Fatalf("list %+v", list)
	}
}

func TestCancelJob(t *testing.T) {
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		<-ctx.Done()
		return 9
	}})
	var j struct{ ID string }
	decode(t, h.do("POST", "/api/jobs", map[string]any{"args": []string{"node"}}, nil), &j)
	if r := h.do("POST", "/api/jobs/"+j.ID+"/cancel", nil, nil); r.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel: %d", r.StatusCode)
	}
	resp := h.do("GET", "/api/jobs/"+j.ID+"/events", nil, nil)
	b := new(bytes.Buffer)
	b.ReadFrom(resp.Body)
	if !strings.Contains(b.String(), "event: exit\ndata: {\"code\":9}") {
		t.Fatalf("events %s", b.String())
	}
}

func TestSplitEndpoint(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	var got struct {
		Args    []string
		Command string
	}
	decode(t, h.do("GET", "/api/split?line="+strings.ReplaceAll("poo pay -memo 'x y'", " ", "%20"), nil, nil), &got)
	if len(got.Args) != 3 || got.Args[2] != "x y" || got.Command != "poo pay -memo 'x y'" {
		t.Fatalf("%+v", got)
	}
}
