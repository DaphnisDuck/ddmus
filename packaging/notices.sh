#!/bin/sh
# Writes THIRD_PARTY_NOTICES for the binary given as $1, to stdout.
#
# The list comes from the binary itself (`go version -m`), so it names what
# ships, not what go.mod mentions. Runs inside the build image
# (packaging/Dockerfile), which holds the native libraries' license files,
# from the build tree release.sh prepares (ddmus's source with vendor/).
set -eu

binary=$1

cat <<'HEAD'
ddmus: third-party notices

ddmus's own source is MIT-licensed (see LICENSE). The ddmus executable links
go-librespot, which is GPL-3.0, so the executable as a whole is distributed
under GPL-3.0 (see LICENSE-GPL-3.0). The complete corresponding source for
this build is the ddmus-<version>-source.tar.gz published beside it.

This file lists everything compiled or linked into the executable, with each
component's license text.

HEAD

section() {
	printf '\n%s\n%s\n\n' "================================================================================" "$1"
}

section "Native libraries"
cat <<NATIVE
Linked statically:
  mpg123 ${MPG123_VERSION} (LGPL-2.1)
  FLAC $(dpkg-query -W -f '${Version}' libflac-dev) (BSD-3-Clause; Xiph.Org)
  libvorbis $(dpkg-query -W -f '${Version}' libvorbis-dev) (BSD-3-Clause; Xiph.Org)
  libogg $(dpkg-query -W -f '${Version}' libogg-dev) (BSD-3-Clause; Xiph.Org)
Linked dynamically, from the system ddmus runs on:
  alsa-lib (LGPL-2.1), glibc (LGPL-2.1)

mpg123 is LGPL-2.1: you may relink ddmus against a modified mpg123. Its source
and ddmus's build scripts are in the source archive.
NATIVE

native() {
	section "$1"
	cat "$2"
}
native "mpg123" /opt/mpg123/COPYING
native "FLAC" /usr/share/doc/libflac-dev/copyright
native "libvorbis" /usr/share/doc/libvorbis-dev/copyright
native "libogg" /usr/share/doc/libogg-dev/copyright
native "alsa-lib" /usr/share/doc/libasound2-dev/copyright

section "Go standard library and runtime ($(go env GOVERSION))"
cat "$(go env GOROOT)/LICENSE"

# The Go modules. The binary names them (`go version -m`); the vendored build
# tree (the current directory) says which of their packages are compiled in.
# A package's license can sit beside it rather than at the module's root, so
# every directory from each shipped package up to its module root is searched,
# in the module cache: `go mod vendor` leaves some license files out.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go version -m "$binary" > "$tmp/buildinfo"
awk '$1 == "dep" || $1 == "=>" { print $2, $3 }' "$tmp/buildinfo" | sort -u > "$tmp/shipped"
go list -mod=vendor -deps -f '{{with .Module}}{{.Path}} {{.Version}} {{end}}{{.ImportPath}}' . > "$tmp/list"
# Three fields: a dependency's package. Fewer: the standard library or ddmus.
awk 'NF == 3' "$tmp/list" > "$tmp/pkgs"
awk '{ print $1, $2 }' "$tmp/pkgs" | sort -u > "$tmp/mods"
if [ ! -s "$tmp/mods" ] || ! cmp -s "$tmp/shipped" "$tmp/mods"; then
	echo "notices: the binary's modules and the build tree's differ:" >&2
	diff "$tmp/shipped" "$tmp/mods" >&2 || true
	exit 1
fi

modcache=$(go env GOMODCACHE)
# Module paths in the cache escape capitals as "!" plus the lower-case letter.
escape() { printf '%s' "$1" | sed 's/[A-Z]/!\L&/g'; }

while read -r mod ver; do
	root="$modcache/$(escape "$mod")@$ver"
	if [ ! -d "$root" ]; then
		echo "notices: $mod@$ver is not in the module cache" >&2
		exit 1
	fi
	section "$mod $ver"
	awk -v m="$mod" '$1 == m {
		d = $3
		while (index(d, m) == 1) {
			print d
			if (d == m) break
			sub(/\/[^\/]*$/, "", d)
		}
	}' "$tmp/pkgs" | sort -u > "$tmp/dirs"
	found=0
	while read -r dir; do
		find "$root${dir#"$mod"}" -maxdepth 1 -type f ! -name '*.go' \( \
			-iname 'licen[sc]e*' -o -iname 'copying*' -o -iname 'notice*' -o \
			-iname 'unlicense*' -o -iname 'copyright*' -o -iname 'patents*' \) | sort > "$tmp/files"
		while read -r f; do
			found=1
			printf -- '--- %s ---\n' "$mod${f#"$root"}"
			cat "$f"
			echo
		done < "$tmp/files"
	done < "$tmp/dirs"
	if [ "$found" = 0 ]; then
		case "$mod" in
		github.com/xlab/vorbis-go)
			cat <<'NOTE'
This module ships no license file at the pinned revision. It is generated Go
bindings (c-for-go) to libvorbis, whose BSD-3-Clause license is reproduced
above. go-librespot's Vorbis decoder pulls it in. ddmus ships it as inherited,
as cliamp does, and names the gap here.
NOTE
			;;
		*)
			echo "notices: $mod@$ver has no license file; add a note for it in packaging/notices.sh" >&2
			exit 1
			;;
		esac
	fi
done < "$tmp/mods"
