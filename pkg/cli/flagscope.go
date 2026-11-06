package cli

import (
	"flag"
	"fmt"
	"slices"
	"strings"
)

// flagsFor refuses a flag given to a subcommand it does nothing for, such
// as poon peer unblock -range or poon sim load -share, which were silently
// ignored and so looked as if they had taken effect (GREEN-015). owners
// maps a flag to the subcommands that read it; flags it does not name
// apply to every subcommand.
func flagsFor(fs *flag.FlagSet, sub string, owners map[string][]string) error {
	var err error
	fs.Visit(func(f *flag.Flag) {
		subs, ok := owners[f.Name]
		if err != nil || !ok || slices.Contains(subs, sub) {
			return
		}
		err = fmt.Errorf("-%s does nothing for %s; it is for %s", f.Name, sub, strings.Join(subs, " and "))
	})
	return err
}
