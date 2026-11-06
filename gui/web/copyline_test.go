package web

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// TestCopyLineWindows checks Copy's line on Windows (api.js copyLine,
// GUI-024): a pasted line goes to PowerShell or cmd.exe, so a word is
// copied only when neither shell would run or change any of it, and any
// other word turns Copy off (null). It needs node and is skipped without.
func TestCopyLineWindows(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	type tc struct {
		args []string
		want *string
	}
	s := func(v string) *string { return &v }
	cases := []tc{
		{[]string{"status"}, s("poon status")},
		{[]string{"address", "-root", `C:\Users\me\.poon`, "abc123"}, s(`poon address -root C:\Users\me\.poon abc123`)},
		{[]string{"policy", "export", "-out", `C:\My Files\p.json`}, s(`poon policy export -out "C:\My Files\p.json"`)},
		{[]string{"tx", "-in-serial", "5", "-pool=1"}, s("poon tx -in-serial 5 -pool=1")},
		// Red's link: PowerShell runs $(...) in a bare word.
		{[]string{"address", "x$(calc)"}, nil},
		// cmd.exe ends a quoted word at \" and runs what follows &.
		{[]string{"address", `a\"&calc`}, nil},
		{[]string{"address", "a;calc"}, nil},
		{[]string{"address", "a&calc"}, nil},
		{[]string{"address", "a|calc"}, nil},
		{[]string{"address", "%PATH%"}, nil},
		{[]string{"address", "!x!"}, nil},
		{[]string{"address", "`calc`"}, nil},
		{[]string{"address", "@args"}, nil},
		{[]string{"address", "a,b"}, nil},
		{[]string{"address", "it's"}, nil},
		{[]string{"address", ""}, nil},
		{[]string{"-out", `C:\My Dir\`}, nil},
		{[]string{"address", " x"}, nil},
		{[]string{"address", "é"}, nil},
		{[]string{"address", "a\nb"}, nil},
	}
	var in [][]string
	for _, c := range cases {
		in = append(in, c.args)
	}
	b, _ := json.Marshal(in)
	out, err := exec.Command(node, "testdata/copyline.mjs", "static/api.js", string(b)).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var got []*string
	if err := json.Unmarshal(out, &got); err != nil || len(got) != len(cases) {
		t.Fatalf("node printed %s: %v", out, err)
	}
	for i, c := range cases {
		switch {
		case c.want == nil && got[i] != nil:
			t.Errorf("%q: copied as %s, want Copy off", c.args, *got[i])
		case c.want != nil && (got[i] == nil || *got[i] != *c.want):
			t.Errorf("%q: copied as %v, want %s", c.args, got[i], *c.want)
		}
	}
}
