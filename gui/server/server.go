// Package server serves the GUI: the embedded page and an API that runs
// poo command lines as jobs. It listens only on a loopback address, and
// every request needs a session cookie, which a browser gets by opening a
// one-time sign-in address (docs/gui.md, Security).
//
// API (JSON; changes are POSTs carrying the X-Poo-GUI header):
//
//	POST /api/jobs              {args} or {line}  start a job: {id, args, command};
//	                            [refresh]         refresh: a view re-running it on a timer
//	GET  /api/jobs                                the kept jobs
//	GET  /api/jobs/{id}/events?from=N             the job's output as server-sent
//	                                              events: chunk, dropped, state, exit
//	POST /api/jobs/{id}/input   {line}            send a line to the job's input
//	POST /api/jobs/{id}/cancel                    stop the job
//	POST /api/run               {args} or {line}  run to the end: {code, out, err, command}
//	GET  /api/split?line=                         the args a typed line is: {args, command}
//	GET  /api/grammar                             the terminal's grammar: {grammar}, posix or windows
//	GET  /api/info                                {grammar, viewOnly}: viewOnly when jobs run in the
//	                                              core's view-only mode (poo-gui -view-only)
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/shell"
)

// CookieName is the session cookie a sign-in sets.
const CookieName = "poo_gui"

// Header must be sent with every POST; a cross-site page cannot send it
// without a preflight, which the server never answers.
const Header = "X-Poo-GUI"

// MaxBody bounds a request body.
const MaxBody = 1 << 20

// DefaultMaxStreams bounds the event streams open at once (GUI-004 in
// security/gui/findings).
const DefaultMaxStreams = 32

// Server is the GUI's HTTP server.
type Server struct {
	runner *jobs.Runner
	static fs.FS
	host   string // the Host requests must carry, such as 127.0.0.1:4321
	mux    *http.ServeMux
	// Heartbeat is how often an idle event stream sends a comment.
	Heartbeat time.Duration
	// MaxStreams bounds the event streams open at once.
	MaxStreams int

	mu       sync.Mutex
	tokens   map[string]time.Time // hashes of unused sign-in tokens, and when each expires
	sessions map[string]bool      // hashes of session cookies
	streams  int
	now      func() time.Time
}

// newToken returns 32 random bytes in hex.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hash is the key a token or cookie is kept under, so the server holds no
// usable credential and a map lookup leaks nothing useful about one.
func hash(t string) string {
	h := sha256.Sum256([]byte(t))
	return string(h[:])
}

// Listen opens a loopback listener on addr, refusing any other address.
// An empty addr picks a free port on 127.0.0.1.
func Listen(addr string) (net.Listener, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if host == "localhost" {
		ip = net.IPv4(127, 0, 0, 1)
		addr = net.JoinHostPort(ip.String(), addr[len(host)+1:])
	}
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("server: %s is not a loopback address; the GUI only listens on this machine", addr)
	}
	return net.Listen("tcp", addr)
}

// New returns a server for requests addressed to host (the listener's
// address), serving static as the page.
func New(runner *jobs.Runner, static fs.FS, host string) *Server {
	s := &Server{
		runner: runner, static: static, host: host, mux: http.NewServeMux(),
		Heartbeat: 15 * time.Second, MaxStreams: DefaultMaxStreams,
		tokens: map[string]time.Time{}, sessions: map[string]bool{}, now: time.Now,
	}
	s.mux.HandleFunc("POST /api/jobs", s.post(s.startJob))
	s.mux.HandleFunc("GET /api/jobs", s.listJobs)
	s.mux.HandleFunc("GET /api/jobs/{id}/events", s.events)
	s.mux.HandleFunc("POST /api/jobs/{id}/input", s.post(s.input))
	s.mux.HandleFunc("POST /api/jobs/{id}/cancel", s.post(s.cancel))
	s.mux.HandleFunc("POST /api/run", s.post(s.run))
	s.mux.HandleFunc("GET /api/split", s.split)
	s.mux.HandleFunc("GET /api/grammar", s.grammar)
	s.mux.HandleFunc("GET /api/info", s.info)
	s.mux.Handle("GET /", http.FileServerFS(static))
	return s
}

// Home is the GUI's address without a sign-in token.
func (s *Server) Home() string { return "http://" + s.host + "/" }

// SignInURL returns a new sign-in address. Its token works once, within
// ttl: the first visit swaps it for a session cookie that is not derived
// from it, so the address left in a process list, the browser's history
// or a log signs nobody in (GUI-001 in security/gui/findings).
func (s *Server) SignInURL(ttl time.Duration) (string, error) {
	t, err := newToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, exp := range s.tokens {
		if !now.Before(exp) {
			delete(s.tokens, k)
		}
	}
	s.tokens[hash(t)] = now.Add(ttl)
	return s.Home() + "?t=" + url.QueryEscape(t), nil
}

// signIn spends token t and returns a new session cookie value.
func (s *Server) signIn(t string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := hash(t)
	exp, ok := s.tokens[k]
	if !ok {
		return "", false
	}
	delete(s.tokens, k)
	if !s.now().Before(exp) {
		return "", false
	}
	c, err := newToken()
	if err != nil {
		return "", false
	}
	s.sessions[hash(c)] = true
	return c, true
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	// The Host check blocks DNS rebinding: a page on another name that
	// resolves to 127.0.0.1 still sends its own name.
	if r.Host != s.host {
		http.Error(w, "wrong host", http.StatusMisdirectedRequest)
		return
	}
	if t := r.URL.Query().Get("t"); t != "" && r.URL.Path == "/" {
		if c, ok := s.signIn(t); ok {
			http.SetCookie(w, &http.Cookie{Name: CookieName, Value: c, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		} else if !s.authed(r) {
			http.Error(w, "This sign-in address was already used or has expired. Press Enter in the window running poo-gui for a new one.", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.authed(r) {
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			writeErr(w, http.StatusUnauthorized, "not signed in: open a sign-in address from poo-gui")
			return
		}
		http.Error(w, "Not signed in. Press Enter in the window running poo-gui to print a sign-in address, and open it.", http.StatusUnauthorized)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[hash(c.Value)]
}

// post guards a handler that changes something.
func (s *Server) post(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(Header) != "1" {
			writeErr(w, http.StatusForbidden, "missing "+Header+" header")
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.host {
			writeErr(w, http.StatusForbidden, "cross-origin request")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
		f(w, r)
	}
}

// command is a request body naming a command line: args, or a typed line.
type command struct {
	Args []string `json:"args"`
	Line *string  `json:"line"`
	// Refresh marks a view re-running a command on a timer; it changes
	// only which finished jobs are dropped first (jobs.StartRefresh).
	Refresh bool `json:"refresh"`
}

func (c command) resolve() ([]string, error) {
	if c.Line != nil {
		if c.Args != nil {
			return nil, errors.New("give args or line, not both")
		}
		return shell.Split(*c.Line)
	}
	if c.Args == nil {
		return nil, errors.New("give args or line")
	}
	return c.Args, nil
}

func readCommand(r *http.Request) ([]string, bool, error) {
	var c command
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return nil, false, err
	}
	args, err := c.resolve()
	return args, c.Refresh, err
}

func (s *Server) start(args []string, refresh bool) (*jobs.Job, error) {
	if refresh {
		return s.runner.StartRefresh(args)
	}
	return s.runner.Start(args)
}

type jobInfo struct {
	ID      string    `json:"id"`
	Args    []string  `json:"args"`
	Command string    `json:"command"`
	Started time.Time `json:"started"`
	Done    bool      `json:"done"`
	Code    *int      `json:"code,omitempty"`
	Refresh bool      `json:"refresh,omitempty"`
}

func info(j *jobs.Job) jobInfo {
	st := j.Read(1 << 62)
	ji := jobInfo{ID: j.ID, Args: j.Args, Command: shell.Quote(j.Args), Started: j.Started, Done: st.Done, Refresh: j.Refresh}
	if st.Done {
		ji.Code = &st.Code
	}
	return ji
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request) {
	args, refresh, err := readCommand(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.start(args, refresh)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, info(j))
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	list := []jobInfo{}
	for _, j := range s.runner.List() {
		list = append(list, info(j))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) (*jobs.Job, bool) {
	j, ok := s.runner.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such job")
	}
	return j, ok
}

func (s *Server) input(w http.ResponseWriter, r *http.Request) {
	j, ok := s.job(w, r)
	if !ok {
		return
	}
	var body struct {
		Line string `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := j.Input(body.Line); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	j, ok := s.job(w, r)
	if !ok {
		return
	}
	j.Cancel()
	w.WriteHeader(http.StatusNoContent)
}

// run runs a command to the end, for views that render one command's
// output. A client that goes away cancels it.
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	args, refresh, err := readCommand(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.start(args, refresh)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	code, err := j.Wait(r.Context())
	if err != nil {
		j.Cancel()
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": j.ID, "code": code, "command": shell.Quote(args),
		"out": j.Output(jobs.Stdout), "err": j.Output(jobs.Stderr),
	})
}

func (s *Server) split(w http.ResponseWriter, r *http.Request) {
	args, err := shell.Split(r.URL.Query().Get("line"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if args == nil {
		args = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"args": args, "command": shell.Quote(args)})
}

func (s *Server) grammar(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"grammar": shell.Native.String()})
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"grammar": shell.Native.String(), "viewOnly": s.runner.ViewOnly()})
}

// events streams a job's output as server-sent events until the job ends
// or the client goes away:
//
//	event: chunk    data: {seq, stream, data}
//	event: dropped  data: {next}       output before next was dropped
//	event: state    data: {waiting, secret}  the job started or stopped waiting for
//	                                         input; secret: the input is a passphrase or seed
//	event: exit     data: {code}       the last event
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	j, ok := s.job(w, r)
	if !ok {
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	s.mu.Lock()
	if s.streams >= s.MaxStreams {
		s.mu.Unlock()
		writeErr(w, http.StatusServiceUnavailable, "too many open event streams")
		return
	}
	s.streams++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streams--
		s.mu.Unlock()
	}()
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	beat := time.NewTicker(s.Heartbeat)
	defer beat.Stop()
	waiting, secret := false, false
	first := true
	for {
		st := j.Read(from)
		if st.Dropped {
			sse(w, "dropped", map[string]int64{"next": st.Chunks[0].Seq})
		}
		for _, c := range st.Chunks {
			sse(w, "chunk", c)
		}
		from = st.Next
		if st.Waiting != waiting || st.Secret != secret || first {
			waiting, secret, first = st.Waiting, st.Secret, false
			sse(w, "state", map[string]bool{"waiting": waiting, "secret": secret})
		}
		if st.Done {
			sse(w, "exit", map[string]int{"code": st.Code})
			fl.Flush()
			return
		}
		fl.Flush()
		select {
		case <-st.Changed:
		case <-beat.C:
			fmt.Fprint(w, ": beat\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func sse(w http.ResponseWriter, event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Serve serves on ln until ctx is done.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 2 * time.Minute,
		// Requests share ctx, so open event streams end on shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(shut)
		<-errc
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
