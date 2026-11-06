// Command poon-gui is the visual way to use the Proof of Ownership full
// node. It is the headless poon tool plus a page that runs poon commands:
// every view, form and the terminal run command lines through cli.Main,
// the function the poon binary runs (docs/gui.md).
//
// Usage:
//
//	poon-gui [-listen 127.0.0.1:PORT] [-no-open] [-app]
//	poon-gui poon ARGS...     run a poon command exactly as poon ARGS... would
//
// A browser signs in by opening a sign-in address once. poon-gui opens one
// itself, or with -no-open prints one; press Enter in its window for
// another (docs/gui.md, Security).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/server"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/web"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

func main() {
	stopOn, absorb := hangup(signal.Ignored(syscall.SIGHUP), isTerminal(os.Stdin))
	if absorb {
		signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), stopOn...)
	code := run(ctx, os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, openURL)
	stop()
	os.Exit(code)
}

// hangup says which signals end poon-gui through its cancel path, so its
// jobs are interrupted (a followed grind stops the node's job) before it
// exits, given whether SIGHUP was ignored at start and whether standard
// input is a terminal (docs/gui.md, Signals).
//
// Interrupt and SIGTERM always stop it. SIGHUP stops it only when it was
// started from a terminal, where SIGHUP means the terminal closed
// (GUI-010). Started with SIGHUP ignored, as nohup does, poon-gui leaves it
// ignored, so it outlives the session it was started from; catching it
// would un-ignore it (GUI-011). Without a terminal, SIGHUP is a daemon's
// reload: it does not stop poon-gui (absorb is true, so Go's default exit
// does not run either), and a node the GUI hosts reloads its node policy
// on it, as poon node does. Windows never sends SIGHUP.
func hangup(ignored, tty bool) (stopOn []os.Signal, absorb bool) {
	stopOn = []os.Signal{os.Interrupt, syscall.SIGTERM}
	switch {
	case ignored:
	case tty:
		stopOn = append(stopOn, syscall.SIGHUP)
	default:
		absorb = true
	}
	return stopOn, absorb
}

// run is the program; open shows a URL to the user.
func run(ctx context.Context, args []string, s cli.Stdio, open func(url string, app bool) error) int {
	if len(args) > 0 && args[0] == "poon" {
		return cli.Main(ctx, args[1:], s)
	}
	fs := flag.NewFlagSet("poon-gui", flag.ContinueOnError)
	fs.SetOutput(s.Err)
	listen := fs.String("listen", "", "loopback address to serve the GUI on (default: 127.0.0.1 on the port poon-gui used last, else any free port)")
	noOpen := fs.Bool("no-open", false, "print the address instead of opening a browser")
	app := fs.Bool("app", false, "open a browser app window (Chrome, Chromium or Edge) instead of a tab")
	viewOnly := fs.Bool("view-only", false, "run every command in poon's view-only mode: read with the node's view token, refuse changes")
	fs.Usage = func() {
		fmt.Fprint(s.Err, "Usage:\n  poon-gui [-listen ADDR] [-no-open] [-app] [-view-only]\n  poon-gui poon ARGS...   run a poon command exactly as poon ARGS... would\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	var opener func(string) error
	if !*noOpen {
		opener = func(url string) error { return open(url, *app) }
	}
	if err := serve(ctx, *listen, *viewOnly, s.In, s.Out, opener); err != nil {
		fmt.Fprintln(s.Err, "poon-gui:", err)
		return 1
	}
	return 0
}

// How long a sign-in address works. The one poon-gui opens itself is used
// by the browser at once, so it lives only long enough for a browser to
// start; a printed one leaves time to copy it.
const (
	openTTL  = time.Minute
	printTTL = 5 * time.Minute
)

// serve runs the GUI until ctx is done, then stops every job, giving a
// running node time to close its stores. open, when not nil, opens a
// sign-in address in a browser; otherwise one is printed. Each line read
// from in prints a new sign-in address. viewOnly runs every job in poon's
// view-only mode (decision D101).
func serve(ctx context.Context, listen string, viewOnly bool, in io.Reader, out io.Writer, open func(url string) error) error {
	ln, err := listenGUI(listen)
	if err != nil {
		return err
	}
	runner := jobs.New(ctx, jobs.Options{ViewOnly: viewOnly, KeepNew: 10 * time.Second})
	srv := server.New(runner, web.FS(), ln.Addr().String())
	srv.Version = cli.Version
	printURL := func(prefix string) {
		u, err := srv.SignInURL(printTTL)
		if err != nil {
			fmt.Fprintln(out, "poon-gui:", err)
			return
		}
		fmt.Fprintf(out, "%sopen %s\n  (works once, within %v; press Enter here for another)\n", prefix, u, printTTL)
	}
	opened := false
	if open != nil {
		// The token in this address is in the browser's arguments only
		// until the browser spends it.
		u, err := srv.SignInURL(openTTL)
		if err == nil {
			err = open(u)
		}
		if err == nil {
			opened = true
			fmt.Fprintf(out, "poon-gui %s: opened %s in your browser; press Enter here for a sign-in address to open it elsewhere\n", cli.Version, srv.Home())
		} else {
			fmt.Fprintf(out, "poon-gui %s: could not open a browser (%v)\n", cli.Version, err)
		}
	}
	if !opened {
		printURL(fmt.Sprintf("poon-gui %s: ", cli.Version))
	}
	if in != nil {
		go func() {
			sc := bufio.NewScanner(in)
			for sc.Scan() && ctx.Err() == nil {
				printURL("")
			}
		}()
	}
	serr := srv.Serve(ctx, ln)
	if err := runner.Close(15 * time.Second); err != nil {
		return err
	}
	return serr
}

// openURL opens url in the default browser, or in an app window.
func openURL(url string, app bool) error {
	if app {
		for _, b := range appBrowsers() {
			if p, err := exec.LookPath(b); err == nil {
				return exec.Command(p, "--app="+url, "--new-window").Start()
			}
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func appBrowsers() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"msedge", "chrome", `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, `C:\Program Files\Google\Chrome\Application\chrome.exe`}
	case "darwin":
		return []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"}
	default:
		return []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"}
	}
}

// listenGUI listens on addr, or, when none is given, on 127.0.0.1 at the
// port poon-gui used last, falling back to any free port (GUI White G1).
// The browser keeps the GUI's saved settings (node policy, wallet file,
// theme, terminal history) per origin, port included, so a new port each
// start lost them all. Only the port number is kept, in poon-gui's folder
// under the user's config directory; nothing else is written there.
func listenGUI(addr string) (net.Listener, error) {
	if addr != "" {
		return server.Listen(addr)
	}
	file := lastPortFile()
	if file != "" {
		if b, err := os.ReadFile(file); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 0 && p < 65536 {
				if ln, err := server.Listen(net.JoinHostPort("127.0.0.1", strconv.Itoa(p))); err == nil {
					return ln, nil
				}
			}
		}
	}
	ln, err := server.Listen("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	if file != "" {
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		if os.MkdirAll(filepath.Dir(file), 0o700) == nil {
			_ = os.WriteFile(file, []byte(port+"\n"), 0o600)
		}
	}
	return ln, nil
}

// lastPortFile is where poon-gui keeps the port it used last, or "" when
// the system names no config directory.
func lastPortFile() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "poon-gui", "last-port")
}
