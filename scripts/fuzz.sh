#!/bin/sh
# Runs every fuzz target for a while each (default 10s; FUZZTIME=1m for
# longer). A failing input is saved under the package's testdata/fuzz,
# where it becomes a regression test: commit it with the fix.
set -eu
t=${FUZZTIME:-10s}
for pkg in $(go list ./...); do
	for f in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz' || true); do
		echo "== $pkg $f"
		go test "$pkg" -run '^$' -fuzz "^$f\$" -fuzztime "$t"
	done
done
