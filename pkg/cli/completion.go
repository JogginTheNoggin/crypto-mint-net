package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Shell completion (GREEN-005). The scripts are tiny: each asks
// poon __complete WORDS... CURRENT for the candidates. The answers come
// from the usage texts and from each command's own flag set, learned by
// running the command with -h under a probe (newFlagSet), so they are
// never stale. A flag set parses before a command does anything, so the
// probe only reads.

const bashCompletion = `# poon bash completion: source this file, or put it in
# /etc/bash_completion.d/ (or ~/.local/share/bash-completion/completions/poon).
_poo() {
	local cur=${COMP_WORDS[COMP_CWORD]}
	local IFS=$'\n'
	COMPREPLY=($(poon __complete "${COMP_WORDS[@]:1:COMP_CWORD-1}" "$cur" 2>/dev/null))
}
complete -o default -F _poo poon
`

const zshCompletion = `#compdef poon
# poon zsh completion: put this file as _poo in a directory on $fpath.
_poo() {
	local -a c
	c=(${(f)"$(poon __complete ${words[2,CURRENT-1]} ${words[CURRENT]} 2>/dev/null)"})
	(( ${#c} )) && compadd -a c || _files
}
compdef _poo poon
`

const fishCompletion = `# poon fish completion: put this file in ~/.config/fish/completions/poon.fish.
complete -c poon -f -a '(poon __complete (commandline -opc)[2..-1] (commandline -ct) 2>/dev/null)'
`

func completionCmd(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: poon completion bash|zsh|fish")
	}
	switch args[0] {
	case "bash":
		fmt.Fprint(out, bashCompletion)
	case "zsh":
		fmt.Fprint(out, zshCompletion)
	case "fish":
		fmt.Fprint(out, fishCompletion)
	default:
		return fmt.Errorf("no completion for %q: bash, zsh or fish", args[0])
	}
	return nil
}

// completeCmd prints the candidates for the last argument, one per line,
// given the words before it.
func completeCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		args = []string{""}
	}
	for _, c := range complete(args[:len(args)-1], args[len(args)-1]) {
		fmt.Fprintln(out, c)
	}
	return nil
}

// complete returns the candidates for cur after the words before it.
func complete(before []string, cur string) []string {
	if len(before) == 0 {
		return withPrefix(commandNames(), cur)
	}
	cmd := before[0]
	subs := subcommands(cmd)
	if len(before) == 1 && len(subs) > 0 && !strings.HasPrefix(cur, "-") {
		return withPrefix(subs, cur)
	}
	if cmd == "help" && len(before) == 1 {
		return withPrefix(commandNames(), cur)
	}
	if cmd == "completion" && len(before) == 1 {
		return withPrefix([]string{"bash", "zsh", "fish"}, cur)
	}
	path := []string{cmd}
	if len(subs) > 0 && len(before) > 1 {
		path = append(path, before[1])
	}
	flags := commandFlags(path)
	// The value of a flag: policy names for -policy, nothing else is
	// known (a file path falls back to the shell's own completion).
	if last := before[len(before)-1]; strings.HasPrefix(last, "-") && !strings.Contains(last, "=") {
		if f, ok := flags[strings.TrimLeft(last, "-")]; ok && !isBoolFlag(f) {
			if f.Name == "policy" {
				return withPrefix(policyNames(), cur)
			}
			return nil
		}
	}
	if strings.HasPrefix(cur, "-") {
		names := make([]string, 0, len(flags))
		for n := range flags {
			names = append(names, "-"+n)
		}
		sort.Strings(names)
		return withPrefix(names, cur)
	}
	return nil
}

func withPrefix(names []string, cur string) []string {
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, cur) {
			out = append(out, n)
		}
	}
	return out
}

var (
	subWord   = regexp.MustCompile(`^[a-z]+(\|[a-z]+)+$`)
	indentSub = regexp.MustCompile(`^  ([a-z]+)\s`)
)

// subcommands returns the subcommands of cmd that the usage texts name.
func subcommands(cmd string) []string {
	var text string
	switch cmd {
	case "wallet":
		text = walletUsage
	case "sim":
		text = simUsage
	case "grind":
		// Its own usage lists every subcommand, so a new one is
		// completed without a second list to keep (GREEN-044).
		text = grindUsage
	}
	var out []string
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			if m := indentSub.FindStringSubmatch(line); m != nil && !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
		return append(out, "help")
	}
	for _, line := range strings.Split(usage, "\n") {
		rest, ok := strings.CutPrefix(line, "  poon "+cmd+" ")
		if !ok {
			continue
		}
		w, _, _ := strings.Cut(rest, " ")
		if subWord.MatchString(w) {
			// A list of subcommands, and maybe more on later lines, as
			// poon policy export and import (D149).
			for _, sub := range strings.Split(w, "|") {
				if !slices.Contains(out, sub) {
					out = append(out, sub)
				}
			}
			continue
		}
		if w != "" && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "[") && strings.ToLower(w) == w && !slices.Contains(out, w) {
			out = append(out, w) // one subcommand per line, as poon series
		}
	}
	return out
}

// commandFlags runs the command at path with -h under a probe and returns
// the flags its flag sets define.
func commandFlags(path []string) map[string]*flag.Flag {
	flags := map[string]*flag.Flag{}
	s := &stdio{Writer: io.Discard, err: io.Discard, probe: func(fs *flag.FlagSet) {
		// The flags are defined after the set is made, so read them when
		// the command parses: -h makes Parse call Usage.
		fs.Usage = func() {
			fs.VisitAll(func(f *flag.Flag) { flags[f.Name] = f })
		}
	}}
	run(append(slices.Clone(path), "-h"), s)
	return flags
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// policyNames lists the saved node policies under the default root.
func policyNames() []string {
	m := (&where{root: ptr(defaultRoot())}).manager()
	es, err := m.List()
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range es {
		if !e.Archived {
			out = append(out, e.Name)
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }
