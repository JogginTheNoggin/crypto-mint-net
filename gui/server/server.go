// Package server serves the GUI: the embedded page and an API that runs
// poo command lines as jobs. It listens only on a loopback address, and
// every API call needs the secret the GUI was started with (docs/gui.md,
// Security).
//
// API (JSON; changes are POSTs carrying the X-Poo-GUI header):
//
//	POST /api/jobs              {args} or {line}  start a job: {id, args, command}
//	GET  /api/jobs                                the kept jobs
//	GET  /api/jobs/{id}/events?from=N             the job's output as server-sent
//	                                              events: chunk, dropped, state, exit
//	POST /api/jobs/{id}/input   {line}            send a line to the job's input
//	POST /api/jobs/{id}/cancel                    stop the job
//	POST /api/run               {args} or {line}  run to the end: {code, out, err, command}
//	GET  /api/split?line=                         the args a typed line is: {args, command}
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/shell"
)

// CookieName is the session cookie set from the launch secret.
const CookieName = "poo_gui"

// Header must be sent with every POST; a cross-site page cannot send it
// without a preflight, which the server never answers.
const Header = "X-Poo-GUI"

// MaxBody bounds a request body.
const MaxBody = 1 << 20

// Server is the GUI's HTTP server.
type Server struct {
	runner *jobs.Runner
	static fs.FS
	secret string
	host   string // the Host requests must carry, such as 127.0.0.1:4321
	mux    *http.ServeMux
	// Heartbeat is how often an idle event stream sends a comment.
	Heartbeat time.Duration
}

// NewSecret returns a fresh launch secret.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
func New(runner *jobs.Runner, static fs.FS, secret, host string) *Server {
	s := &Server{runner: runner, static: static, secret: secret, host: host, mux: http.NewServeMux(), Heartbeat: 15 * time.Second}
	s.mux.HandleFunc("POST /api/jobs", s.post(s.startJob))
	s.mux.HandleFunc("GET /api/jobs", s.listJobs)
	s.mux.HandleFunc("GET /api/jobs/{id}/events", s.events)
	s.mux.HandleFunc("POST /api/jobs/{id}/input", s.post(s.input))
	s.mux.HandleFunc("POST /api/jobs/{id}/cancel", s.post(s.cancel))
	s.mux.HandleFunc("POST /api/run", s.post(s.run))
	s.mux.HandleFunc("GET /api/split", s.split)
	s.mux.Handle("GET /", http.FileServerFS(static))
	return s
}

// URL is the address to open: it carries the secret once, which the first
// visit swaps for a cookie.
func (s *Server) URL() string {
	return "http://" + s.host + "/?t=" + url.QueryEscape(s.secret)
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
		if !s.secretOK(t) {
			http.Error(w, "wrong secret: open the address poo-gui printed", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: CookieName, Value: s.secret, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.authed(r) {
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			writeErr(w, http.StatusUnauthorized, "not signed in: open the address poo-gui printed")
			return
		}
		http.Error(w, "Open the address poo-gui printed when it started; it signs this browser in.", http.StatusUnauthorized)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) secretOK(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.secret)) == 1
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	return err == nil && s.secretOK(c.Value)
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

func readCommand(r *http.Request) ([]string, error) {
	var c command
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return nil, err
	}
	return c.resolve()
}

type jobInfo struct {
	ID      string    `json:"id"`
	Args    []string  `json:"args"`
	Command string    `json:"command"`
	Started time.Time `json:"started"`
	Done    bool      `json:"done"`
	Code    *int      `json:"code,omitempty"`
}

func info(j *jobs.Job) jobInfo {
	st := j.Read(1 << 62)
	ji := jobInfo{ID: j.ID, Args: j.Args, Command: shell.Quote(j.Args), Started: j.Started, Done: st.Done}
	if st.Done {
		ji.Code = &st.Code
	}
	return ji
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request) {
	args, err := readCommand(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.runner.Start(args)
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
	args, err := readCommand(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.runner.Start(args)
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

// events streams a job's output as server-sent events until the job ends
// or the client goes away:
//
//	event: chunk    data: {seq, stream, data}
//	event: dropped  data: {next}       output before next was dropped
//	event: state    data: {waiting}    the job started or stopped waiting for input
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
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	beat := time.NewTicker(s.Heartbeat)
	defer beat.Stop()
	waiting := false
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
		if st.Waiting != waiting || first {
			waiting, first = st.Waiting, false
			sse(w, "state", map[string]bool{"waiting": waiting})
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
