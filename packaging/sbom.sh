#!/bin/sh
# Writes a minimal CycloneDX 1.5 SBOM for the binary $1 (version $2) to stdout:
# the Go modules the binary reports, the Go toolchain, and the native
# libraries. Runs inside the build image.
set -eu

binary=$1
version=$2

native() { # name version license linkage
	jq -n --arg n "$1" --arg v "$2" --arg l "$3" --arg k "$4" \
		'{type: "library", name: $n, version: $v, licenses: [{license: {id: $l}}], properties: [{name: "ddsonic:linkage", value: $k}]}'
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go version -m "$binary" > "$tmp/buildinfo"
awk '$1 == "dep" || $1 == "=>" { print $2, $3, $4 }' "$tmp/buildinfo" | sort -u > "$tmp/mods"
if [ ! -s "$tmp/mods" ]; then
	echo "sbom: $binary names no Go modules" >&2
	exit 1
fi

{
	while read -r mod ver sum; do
		jq -n --arg n "$mod" --arg v "$ver" --arg s "$sum" \
			'{type: "library", name: $n, version: $v, purl: ("pkg:golang/" + $n + "@" + $v)}
			 + (if $s == "" then {} else {properties: [{name: "go:sum", value: $s}]} end)'
	done < "$tmp/mods"
	jq -n --arg v "$(go env GOVERSION)" '{type: "library", name: "go (standard library and runtime)", version: $v, licenses: [{license: {id: "BSD-3-Clause"}}]}'
	native mpg123 "$MPG123_VERSION" LGPL-2.1-only static
	native flac "$(dpkg-query -W -f '${Version}' libflac-dev)" BSD-3-Clause static
	native libvorbis "$(dpkg-query -W -f '${Version}' libvorbis-dev)" BSD-3-Clause static
	native libogg "$(dpkg-query -W -f '${Version}' libogg-dev)" BSD-3-Clause static
	native alsa-lib "system" LGPL-2.1-or-later dynamic
	native glibc "system (built against $(dpkg-query -W -f '${Version}' libc6))" LGPL-2.1-or-later dynamic
} > "$tmp/components"

jq -s --arg v "$version" '{
	bomFormat: "CycloneDX",
	specVersion: "1.5",
	version: 1,
	metadata: {component: {type: "application", name: "ddsonic", version: $v, licenses: [{license: {id: "GPL-3.0-only"}}]}},
	components: .
}' "$tmp/components"
