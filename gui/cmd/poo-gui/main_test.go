package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// TestPooPassthrough checks "poo-gui poo ARGS" is "poo ARGS".
func TestPooPassthrough(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"crypto"}, {"nope"}} {
		var a, ae, b, be bytes.Buffer
		ca := run(context.Background(), append([]string{"poo"}, args...), cli.Stdio{Out: &a, Err: &ae}, nil)
		cb := cli.Main(context.Background(), args, cli.Stdio{Out: &b, Err: &be})
		if ca != cb || a.String() != b.String() || ae.String() != be.String() {
			t.Errorf("%v: poo-gui poo differs from poo", args)
		}
	}
}

func TestServeOpensAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	opened := make(chan string, 1)
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, nil, cli.Stdio{Out: pw, Err: io.Discard}, func(url string, app bool) error {
			opened <- url
			return nil
		})
		pw.Close()
	}()
	line, err := bufio.NewReader(pr).ReadString('\n')
	if err != nil || !strings.Contains(line, "opened http://127.0.0.1:") {
		t.Fatalf("banner %q %v", line, err)
	}
	go io.Copy(io.Discard, pr)
	url := <-opened
	// GUI-001: the token the browser got is not printed.
	if i := strings.Index(url, "?t="); i < 0 || strings.Contains(line, url[i:]) {
		t.Fatalf("opened %s, printed %s", url, line)
	}
	jar, _ := cookiejar.New(nil)
	resp, err := (&http.Client{Jar: jar}).Get(url) // signs in, then the page
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(page), `src="app.js"`) {
		t.Fatalf("page: %d %.80s", resp.StatusCode, page)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("did not stop")
	}
}

// TestNoOpenPrintsOneUseAddresses is GUI-001: with -no-open poo-gui
// prints a sign-in address, Enter prints a new one, and each works once.
func TestNoOpenPrintsOneUseAddresses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-no-open"}, cli.Stdio{In: inR, Out: outW, Err: io.Discard}, nil)
		outW.Close()
	}()
	urls := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			if f := strings.Fields(sc.Text()); len(f) > 0 && strings.Contains(f[len(f)-1], "?t=") {
				urls <- f[len(f)-1]
			}
		}
	}()
	next := func() string {
		select {
		case u := <-urls:
			return u
		case <-time.After(10 * time.Second):
			t.Fatal("no sign-in address printed")
			return ""
		}
	}
	first := next()
	io.WriteString(inW, "\n")
	second := next()
	if first == second {
		t.Fatal("Enter printed the same address")
	}
	get := func(u string) int {
		jar, _ := cookiejar.New(nil)
		resp, err := (&http.Client{Jar: jar}).Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, u := range []string{first, second} {
		if c := get(u); c != http.StatusOK {
			t.Fatalf("first use of %s: %d", u, c)
		}
		if c := get(u); c != http.StatusForbidden {
			t.Fatalf("second use of %s: %d", u, c)
		}
	}
	cancel()
	inW.Close()
	<-done
}

func TestRefusesPublicListen(t *testing.T) {
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"-listen", "0.0.0.0:0", "-no-open"}, cli.Stdio{Out: io.Discard, Err: &errOut}, nil); code != 1 || !strings.Contains(errOut.String(), "loopback") {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
}
