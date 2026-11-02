#!/bin/sh
# Builds release archives of poo, and of poo-gui, for every supported platform into dist/,
# with a SHA256SUMS file. Usage: scripts/release.sh VERSION (e.g. 0.1.0).
#
# Builds are reproducible: the same source, toolchain (go.mod's toolchain
# line) and VERSION give the same bytes, so anyone can check a published
# archive against SHA256SUMS by rebuilding it. See docs/releasing.md.
set -eu
v=${1:?usage: scripts/release.sh VERSION}
case $v in v*) v=${v#v} ;; esac
out=${DIST:-dist}
rm -rf "$out"
mkdir -p "$out"
platforms="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64"
# Archive members get a fixed time so the archives are reproducible too.
stamp=$(git log -1 --format=%ct 2>/dev/null || echo 0)
out=$(cd "$out" && pwd)
# The commit goes in by -X, since -buildvcs=false (for reproducible
# builds) leaves it out of the build info; poo version -v prints it.
commit=$(git rev-parse HEAD 2>/dev/null || echo "")
# archive PROGRAM MODULE_DIR PACKAGE OS ARCH builds one archive.
archive() {
	prog=$1 mod=$2 pkg=$3 os=$4 arch=$5
	name=$prog-$v-$os-$arch
	dir=$out/$name
	mkdir -p "$dir"
	bin=$prog
	[ "$os" = windows ] && bin=$prog.exe
	echo "== $name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go -C "$mod" build -trimpath -buildvcs=false \
		-ldflags "-s -w -buildid= -X github.com/JogginTheNoggin/crypto-mint-net/pkg/cli.Version=$v -X github.com/JogginTheNoggin/crypto-mint-net/pkg/cli.Commit=$commit" -o "$dir/$bin" "$pkg"
	cp README.md "$dir/"
	[ -f LICENSE ] && cp LICENSE "$dir/"
	find "$dir" -exec touch -d "@$stamp" {} +
	if [ "$os" = windows ]; then
		(cd "$out" && zip -qX -r "$name.zip" "$name")
	else
		tar -C "$out" --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$stamp" \
			-cf - "$name" | gzip -n >"$out/$name.tar.gz"
	fi
	rm -rf "$dir"
}
# poo is the headless tool and never contains GUI code; poo-gui (the gui
# module) is the same tool plus the GUI, shipped as its own archive so the
# GUI stays optional (docs/gui.md, D88).
for p in $platforms; do
	archive poo . ./cmd/poo "${p%/*}" "${p#*/}"
	archive poo-gui gui ./cmd/poo-gui "${p%/*}" "${p#*/}"
done
(cd "$out" && sha256sum -- *.tar.gz *.zip >SHA256SUMS)
cat "$out/SHA256SUMS"
