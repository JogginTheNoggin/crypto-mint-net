package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// BuildInfo is what poo version -v prints: enough for a bug report to say
// exactly which code ran (GREEN-013).
type BuildInfo struct {
	Version  string            `json:"version"`
	Go       string            `json:"go"`
	Platform string            `json:"platform"`
	Commit   string            `json:"commit,omitempty"`
	Modified bool              `json:"modified,omitempty"`
	Time     string            `json:"commit_time,omitempty"`
	Deps     map[string]string `json:"deps,omitempty"`
}

func buildInfo() BuildInfo {
	b := BuildInfo{Version: Version, Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, Commit: Commit}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		case "vcs.time":
			b.Time = s.Value
		}
	}
	for _, d := range bi.Deps {
		if b.Deps == nil {
			b.Deps = map[string]string{}
		}
		v := d.Version
		if d.Replace != nil {
			v += " => " + d.Replace.Path + " " + d.Replace.Version
		}
		b.Deps[d.Path] = v
	}
	return b
}

// releaseCommit is the commit the node reports as its release: Commit
// when a release build sets it with -X, or else the VCS revision a dev
// build records ("+modified" when its tree had changes), so poo status
// and poo doctor name the code a dev build runs too (GREEN-059).
func releaseCommit() string {
	if Commit != "" {
		return Commit
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return vcsCommit(bi.Settings)
}

// vcsCommit reads the revision, and whether the tree was modified, from
// build settings.
func vcsCommit(settings []debug.BuildSetting) string {
	rev, modified := "", false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev != "" && modified {
		rev += "+modified"
	}
	return rev
}

// versionCmd prints the version, or with -v the build's details.
func versionCmd(args []string, out io.Writer) error {
	fs := newFlagSet("version", out)
	verbose := fs.Bool("v", false, "also the Go version, platform, commit and dependencies")
	// Accepted and ignored, as poo version took any arguments before -v:
	// scripts and tests pass the node-location flags to every command.
	addWhere(fs)
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if !*verbose {
		fmt.Fprintln(out, Version)
		return nil
	}
	return o.write(out, buildInfo())
}
