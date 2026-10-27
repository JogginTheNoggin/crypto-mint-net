// Command poo-gui is the visual way to use the Proof of Ownership full
// node. It is the headless poo tool plus a page that runs poo commands:
// every view, form and the terminal run command lines through cli.Main,
// the function the poo binary runs (docs/gui.md).
//
// Usage:
//
//	poo-gui [-listen 127.0.0.1:PORT] [-no-open] [-app]
//	poo-gui poo ARGS...     run a poo command exactly as poo ARGS... would
//
// A browser signs in by opening a sign-in address once. poo-gui opens one
// itself, or with -no-open prints one; press Enter in its window for
// another (docs/gui.md, Security).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/jobs"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/server"
	"github.com/JogginTheNoggin/crypto-mint-net/gui/web"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, openURL)
	stop()
	os.Exit(code)
}

// run is the program; open shows a URL to the user.
func run(ctx context.Context, args []string, s cli.Stdio, open func(url string, app bool) error) int {
	if len(args) > 0 && args[0] == "poo" {
		return cli.Main(ctx, args[1:], s)
	}
	fs := flag.NewFlagSet("poo-gui", flag.ContinueOnError)
	fs.SetOutput(s.Err)
	listen := fs.String("listen", "127.0.0.1:0", "loopback address to serve the GUI on")
	noOpen := fs.Bool("no-open", false, "print the address instead of opening a browser")
	app := fs.Bool("app", false, "open a browser app window (Chrome, Chromium or Edge) instead of a tab")
	viewOnly := fs.Bool("view-only", false, "run every command in poo's view-only mode: read with the node's view token, refuse changes")
	fs.Usage = func() {
		fmt.Fprint(s.Err, "Usage:\n  poo-gui [-listen ADDR] [-no-open] [-app] [-view-only]\n  poo-gui poo ARGS...   run a poo command exactly as poo ARGS... would\n\n")
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
		fmt.Fprintln(s.Err, "poo-gui:", err)
		return 1
	}
	return 0
}

// How long a sign-in address works. The one poo-gui opens itself is used
// by the browser at once, so it lives only long enough for a browser to
// start; a printed one leaves time to copy it.
const (
	openTTL  = time.Minute
	printTTL = 5 * time.Minute
)

// serve runs the GUI until ctx is done, then stops every job, giving a
// running node time to close its stores. open, when not nil, opens a
// sign-in address in a browser; otherwise one is printed. Each line read
// from in prints a new sign-in address. viewOnly runs every job in poo's
// view-only mode (decision D101).
func serve(ctx context.Context, listen string, viewOnly bool, in io.Reader, out io.Writer, open func(url string) error) error {
	ln, err := server.Listen(listen)
	if err != nil {
		return err
	}
	runner := jobs.New(ctx, jobs.Options{ViewOnly: viewOnly})
	srv := server.New(runner, web.FS(), ln.Addr().String())
	printURL := func(prefix string) {
		u, err := srv.SignInURL(printTTL)
		if err != nil {
			fmt.Fprintln(out, "poo-gui:", err)
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
			fmt.Fprintf(out, "poo-gui %s: opened %s in your browser; press Enter here for a sign-in address to open it elsewhere\n", cli.Version, srv.Home())
		} else {
			fmt.Fprintf(out, "poo-gui %s: could not open a browser (%v)\n", cli.Version, err)
		}
	}
	if !opened {
		printURL(fmt.Sprintf("poo-gui %s: ", cli.Version))
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
