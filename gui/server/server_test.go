package server

import (
	"bufio"
	"bytes"
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
	static := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>page")}}
	h := &harness{t: t}
	h.srv = httptest.NewUnstartedServer(nil)
	h.srv.Start()
	h.s = New(runner, static, h.srv.Listener.Addr().String())
	h.srv.Config.Handler = h.s
	t.Cleanup(func() { h.srv.Close(); runner.Close(5 * time.Second) })
	h.cookie = h.signIn(h.signInPath(time.Minute))
	if h.cookie == nil {
		t.Fatal("sign-in set no cookie")
	}
	return h
}

// signInPath returns a new sign-in address, as a path.
func (h *harness) signInPath(ttl time.Duration) string {
	h.t.Helper()
	u, err := h.s.SignInURL(ttl)
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.TrimPrefix(u, h.srv.URL)
}

// signIn opens path without a cookie and returns the cookie it set.
func (h *harness) signIn(path string) *http.Cookie {
	h.t.Helper()
	r := h.do("GET", path, nil, func(r *http.Request) { r.Header.Del("Cookie") })
	for _, c := range r.Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
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
	if h.cookie != nil {
		req.AddCookie(h.cookie)
	}
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
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	path := h.signInPath(time.Minute)
	if !strings.HasPrefix(path, "/?t=") {
		t.Fatalf("sign-in path %s", path)
	}
	r := h.do("GET", path, nil, noCookie)
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/" {
		t.Fatalf("token: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	var set *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == CookieName {
			set = c
		}
	}
	if set == nil || !set.HttpOnly || set.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", set)
	}
	// GUI-001: the cookie is not the token, so the address is not the
	// credential.
	if strings.Contains(path, set.Value) {
		t.Fatal("the session cookie is the sign-in token")
	}
	h.cookie = set
	if r := h.do("GET", "/", nil, nil); r.StatusCode != http.StatusOK || !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("page: %d, csp %q", r.StatusCode, r.Header.Get("Content-Security-Policy"))
	}
	// A signed-in browser that opens a used address again stays signed in.
	if r := h.do("GET", path, nil, nil); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("used address with a cookie: %d", r.StatusCode)
	}
	if h.s.Home() != h.srv.URL+"/" {
		t.Fatalf("Home %s", h.s.Home())
	}
}

// TestSignInTokenOneUse is GUI-001: a sign-in address left in a process
// list, the browser's history or a log does not sign anyone else in.
func TestSignInTokenOneUse(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	path := h.signInPath(time.Minute)
	if h.signIn(path) == nil {
		t.Fatal("first use did not sign in")
	}
	if c := h.signIn(path); c != nil {
		t.Fatal("a used token signed in again")
	}
	if r := h.do("GET", path, nil, func(r *http.Request) { r.Header.Del("Cookie") }); r.StatusCode != http.StatusForbidden {
		t.Fatalf("used token: %d", r.StatusCode)
	}
	// Each sign-in is its own session.
	a, b := h.signIn(h.signInPath(time.Minute)), h.signIn(h.signInPath(time.Minute))
	if a == nil || b == nil || a.Value == b.Value {
		t.Fatalf("sessions %v %v", a, b)
	}
}

func TestSignInTokenExpires(t *testing.T) {
	h := newHarness(t, jobs.Options{})
	now := time.Now()
	h.s.now = func() time.Time { return now }
	path := h.signInPath(time.Minute)
	now = now.Add(time.Minute)
	if c := h.signIn(path); c != nil {
		t.Fatal("an expired token signed in")
	}
}

// TestMaxStreams is GUI-004: event streams open at once are bounded.
func TestMaxStreams(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return 0
	}})
	h.s.MaxStreams = 2
	var j struct{ ID string }
	decode(t, h.do("POST", "/api/jobs", map[string]any{"args": []string{"x"}}, nil), &j)
	path := "/api/jobs/" + j.ID + "/events"
	for range 2 {
		if r := h.do("GET", path, nil, nil); r.StatusCode != http.StatusOK {
			t.Fatalf("stream: %d", r.StatusCode)
		}
	}
	r := h.do("GET", path, nil, nil)
	if r.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("stream past the limit: %d", r.StatusCode)
	}
}

// TestMaxRunningJobs is GUI-004 through the API: a start past the limit
// is refused with 503.
func TestMaxRunningJobs(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	h := newHarness(t, jobs.Options{MaxRunning: 1, Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return 0
	}})
	body := map[string]any{"args": []string{"x"}}
	if r := h.do("POST", "/api/jobs", body, nil); r.StatusCode != http.StatusCreated {
		t.Fatalf("first: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/jobs", body, nil); r.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("past the limit: %d", r.StatusCode)
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
			if d == `{"secret":false,"waiting":true}` && !sentInput {
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
	for _, want := range []string{`"data":"passphrase: "`, `{"secret":false,"waiting":true}`, `"data":"hello world\n"`, `"stream":"err","data":"bye\n"`, `{"code":4}`} {
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
	if len(got.Args) != 3 || got.Args[2] != "x y" || got.Command != shell.Quote([]string{"pay", "-memo", "x y"}) {
		t.Fatalf("%+v", got)
	}
	// GUI-002: a line a shell would split differently is refused.
	for _, bad := range []string{"a%0Ab", "a%20%7C%20b", "%24HOME"} {
		if shell.Native == shell.Windows && bad == "%24HOME" {
			continue
		}
		if r := h.do("GET", "/api/split?line="+bad, nil, nil); r.StatusCode != http.StatusBadRequest {
			t.Errorf("split %s: %d", bad, r.StatusCode)
		}
	}
	if r := h.do("POST", "/api/jobs", map[string]any{"line": "a\nb c"}, nil); r.StatusCode != http.StatusBadRequest {
		t.Errorf("job line with a newline: %d", r.StatusCode)
	}
	var g struct{ Grammar string }
	decode(t, h.do("GET", "/api/grammar", nil, nil), &g)
	if g.Grammar != shell.Native.String() {
		t.Fatalf("grammar %q", g.Grammar)
	}
}

// TestSecretState checks that a job reading a secret through
// cli.Stdio.Secret says so in its state event, so the page masks that
// line and only that line.
func TestSecretState(t *testing.T) {
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		if _, err := s.Secret("passphrase: "); err != nil {
			return 2
		}
		return 0
	}})
	var j struct{ ID string }
	decode(t, h.do("POST", "/api/jobs", map[string]any{"args": []string{"wallet", "addresses"}}, nil), &j)
	resp := h.do("GET", "/api/jobs/"+j.ID+"/events", nil, nil)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if sc.Text() == `data: {"secret":true,"waiting":true}` {
			h.do("POST", "/api/jobs/"+j.ID+"/input", map[string]string{"line": "pw"}, nil)
		}
		if sc.Text() == "event: exit" {
			return
		}
	}
	t.Fatal("no exit event")
}

func TestInfo(t *testing.T) {
	for _, view := range []bool{false, true} {
		h := newHarness(t, jobs.Options{ViewOnly: view})
		var got struct {
			Grammar  string
			ViewOnly bool
		}
		decode(t, h.do("GET", "/api/info", nil, nil), &got)
		if got.ViewOnly != view || got.Grammar != shell.Native.String() {
			t.Fatalf("view-only %v: info %+v", view, got)
		}
	}
}

// TestJobsHideSecretFlags is GUI-006: a seed passed on the command line is
// hidden in the jobs list, while the command still gets it.
func TestJobsHideSecretFlags(t *testing.T) {
	seed := strings.Repeat("ab", 32)
	got := make(chan []string, 1)
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		got <- args
		return 0
	}})
	r := h.do("POST", "/api/jobs", map[string]any{"line": "keygen -seed " + seed}, nil)
	var started struct {
		Args     []string
		Command  string
		Redacted bool
	}
	decode(t, r, &started)
	if a := <-got; len(a) != 3 || a[2] != seed {
		t.Fatalf("the command got %q", a)
	}
	var list []struct {
		Args     []string
		Command  string
		Redacted bool
	}
	decode(t, h.do("GET", "/api/jobs", nil, nil), &list)
	for _, ji := range append(list, started) {
		if strings.Contains(ji.Command, seed) || strings.Contains(strings.Join(ji.Args, " "), seed) || !ji.Redacted {
			t.Fatalf("seed shown: %+v", ji)
		}
	}
	var info struct{ SecretFlags []string }
	decode(t, h.do("GET", "/api/info", nil, nil), &info)
	if len(info.SecretFlags) == 0 {
		t.Fatal("no secretFlags in /api/info")
	}
}

// TestStreamJoinsAndResumes: adjacent writes of one stream arrive as one
// chunk event, each event's id is the seq to read next, and a browser
// that reconnects with Last-Event-ID gets only what follows (ORANGE-002).
func TestStreamJoinsAndResumes(t *testing.T) {
	release := make(chan struct{})
	h := newHarness(t, jobs.Options{Main: func(ctx context.Context, args []string, s cli.Stdio) int {
		for i := 0; i < 5; i++ {
			s.Out.Write([]byte(fmt.Sprintf("o%d\n", i)))
		}
		s.Err.Write([]byte("e\n"))
		s.Out.Write([]byte("last\n"))
		<-release
		return 0
	}})
	var j struct{ ID string }
	decode(t, h.do("POST", "/api/jobs", map[string]any{"args": []string{"x"}}, nil), &j)
	// Wait for all seven writes, then read them at once.
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		jb, _ := h.s.runner.Get(j.ID)
		if jb.Read(0).Next == 7 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("writes did not arrive")
		}
	}
	read := func(lastID string) (ids, datas []string) {
		resp := h.do("GET", "/api/jobs/"+j.ID+"/events", nil, func(r *http.Request) {
			if lastID != "" {
				r.Header.Set("Last-Event-ID", lastID)
			}
		})
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			l := sc.Text()
			if strings.HasPrefix(l, "id: ") {
				ids = append(ids, l[4:])
			}
			if strings.HasPrefix(l, "data: {\"seq\"") {
				datas = append(datas, l[6:])
			}
			if strings.HasPrefix(l, "data: {\"secret\"") {
				return
			}
		}
		return
	}
	ids, datas := read("")
	// next is where to follow again from (?from=), past every joined write.
	want := []string{`{"seq":0,"stream":"out","data":"o0\no1\no2\no3\no4\n","next":5}`, `{"seq":5,"stream":"err","data":"e\n","next":6}`, `{"seq":6,"stream":"out","data":"last\n","next":7}`}
	if strings.Join(datas, "|") != strings.Join(want, "|") || strings.Join(ids, ",") != "5,6,7" {
		t.Fatalf("ids %v, events:\n%s", ids, strings.Join(datas, "\n"))
	}
	if _, datas := read("6"); len(datas) != 1 || datas[0] != want[2] {
		t.Fatalf("resumed from 6: %v", datas)
	}
	close(release)
}

// TestSecretOutputNotReplayed is GUI-009 through the API, with the real
// keygen: the seed it prints is given out once and then cannot be read
// again from the job's event stream, from ?from=0 or with Last-Event-ID.
func TestSecretOutputNotReplayed(t *testing.T) {
	seed := strings.Repeat("cd", 32)
	h := newHarness(t, jobs.Options{})
	var run struct {
		ID  string
		Out string
	}
	r := h.do("POST", "/api/jobs", map[string]any{"args": []string{"keygen", "-seed", seed}}, nil)
	var j struct{ ID string }
	decode(t, r, &j)
	read := func(mod func(*http.Request)) string {
		var b bytes.Buffer
		b.ReadFrom(h.do("GET", "/api/jobs/"+j.ID+"/events?from=0", nil, mod).Body)
		return b.String()
	}
	if first := read(nil); !strings.Contains(first, seed) || !strings.Contains(first, "event: exit") {
		t.Fatalf("first read: %s", first)
	}
	for _, mod := range []func(*http.Request){nil, func(r *http.Request) { r.Header.Set("Last-Event-ID", "0") }} {
		again := read(mod)
		if strings.Contains(again, seed) || !strings.Contains(again, `"withheld":true`) {
			t.Fatalf("read again: %s", again)
		}
	}
	var list []struct {
		ID                     string
		SecretOutput, Withheld bool
	}
	decode(t, h.do("GET", "/api/jobs", nil, nil), &list)
	found := false
	for _, ji := range list {
		if ji.ID == j.ID {
			found = ji.SecretOutput && ji.Withheld
		}
	}
	if !found {
		t.Fatalf("jobs list does not mark it: %+v", list)
	}

	// /api/run gives the output once, too.
	decode(t, h.do("POST", "/api/run", map[string]any{"args": []string{"keygen", "-seed", seed}}, nil), &run)
	if !strings.Contains(run.Out, seed) {
		t.Fatalf("run: %q", run.Out)
	}
	j.ID = run.ID
	if again := read(nil); strings.Contains(again, seed) {
		t.Fatalf("run output replayed: %s", again)
	}
}
