#!/bin/sh
# Builds release archives of poo for every supported platform into dist/,
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
for p in $platforms; do
	os=${p%/*}
	arch=${p#*/}
	name=poo-$v-$os-$arch
	dir=$out/$name
	mkdir -p "$dir"
	bin=poo
	[ "$os" = windows ] && bin=poo.exe
	echo "== $name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false \
		-ldflags "-s -w -buildid= -X github.com/JogginTheNoggin/crypto-mint-net/pkg/cli.Version=$v" -o "$dir/$bin" ./cmd/poo
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
done
(cd "$out" && sha256sum -- *.tar.gz *.zip >SHA256SUMS)
cat "$out/SHA256SUMS"
