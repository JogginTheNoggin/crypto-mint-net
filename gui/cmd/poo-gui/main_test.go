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
	if err != nil || !strings.Contains(line, "open http://127.0.0.1:") {
		t.Fatalf("banner %q %v", line, err)
	}
	go io.Copy(io.Discard, pr)
	url := <-opened
	if !strings.Contains(line, url) {
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

func TestRefusesPublicListen(t *testing.T) {
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"-listen", "0.0.0.0:0", "-no-open"}, cli.Stdio{Out: io.Discard, Err: &errOut}, nil); code != 1 || !strings.Contains(errOut.String(), "loopback") {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
}
