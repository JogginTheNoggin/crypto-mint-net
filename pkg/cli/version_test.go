package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestVersionVerbose(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"version"}, &b); err != nil || strings.TrimSpace(b.String()) != Version {
		t.Fatalf("version: %v %q", err, b.String())
	}
	b.Reset()
	if err := run([]string{"version", "-v"}, &b); err != nil {
		t.Fatal(err)
	}
	var bi BuildInfo
	if err := json.Unmarshal(b.Bytes(), &bi); err != nil {
		t.Fatal(err)
	}
	if bi.Version != Version || bi.Go != runtime.Version() || bi.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("%+v", bi)
	}
}
