// Command poo-gui is the visual way to use the Proof of Ownership full
// node. It is the headless poo tool plus a page that runs poo commands:
// every view, form and the terminal run command lines through cli.Main,
// the function the poo binary runs (docs/gui.md).
//
// Usage:
//
//	poo-gui [-listen 127.0.0.1:PORT] [-no-open] [-app]
//	poo-gui poo ARGS...     run a poo command exactly as poo ARGS... would
package main

import (
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
	fs.Usage = func() {
		fmt.Fprint(s.Err, "Usage:\n  poo-gui [-listen ADDR] [-no-open] [-app]\n  poo-gui poo ARGS...   run a poo command exactly as poo ARGS... would\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if err := serve(ctx, *listen, s.Out, func(url string) error {
		if *noOpen {
			return nil
		}
		return open(url, *app)
	}); err != nil {
		fmt.Fprintln(s.Err, "poo-gui:", err)
		return 1
	}
	return 0
}

// serve runs the GUI until ctx is done, then stops every job, giving a
// running node time to close its stores.
func serve(ctx context.Context, listen string, out io.Writer, ready func(url string) error) error {
	ln, err := server.Listen(listen)
	if err != nil {
		return err
	}
	secret, err := server.NewSecret()
	if err != nil {
		ln.Close()
		return err
	}
	runner := jobs.New(ctx, jobs.Options{})
	srv := server.New(runner, web.FS(), secret, ln.Addr().String())
	fmt.Fprintf(out, "poo-gui %s: open %s\n", cli.Version, srv.URL())
	if err := ready(srv.URL()); err != nil {
		fmt.Fprintf(out, "could not open a browser (%v); open the address above\n", err)
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
