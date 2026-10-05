#!/bin/sh
# Builds the release files for one version, inside the build image:
#
#   docker build -t ddsonic-build -f packaging/Dockerfile packaging
#   docker run --rm -v "$PWD":/src:ro -v "$PWD/dist":/out ddsonic-build \
#       /src/packaging/release.sh v1.0.0 "$(git log -1 --format=%ct)"
#
# /src is a checkout of the tag; /out receives the binary archive, the
# complete corresponding source, the SBOM and SHA256SUMS. The second argument
# is the timestamp every file in the archives carries, so two runs on the same
# commit give the same bytes. An optional third argument names the tree to
# build instead of HEAD (a dry run of uncommitted work).
set -eu

version=$1
epoch=$2
# The version goes into linker flags and file names: accept a release tag only.
if ! printf '%s\n' "$version" | grep -Eqx 'v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?'; then
	echo "release: '$version' is not a version tag (v1.2.3 or v1.2.3-rc.1)" >&2
	exit 1
fi
treeish=${3:-HEAD}
name="ddsonic-${version#v}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Build from a copy of the tracked files only, with dependencies vendored:
# the same tree the source archive ships.
tree="$work/$name-source"
mkdir -p "$tree"
git -c safe.directory=/src -C /src archive --format=tar "$treeish" | tar -x -C "$tree"
cd "$tree"
go mod vendor
mkdir native
cp /native-src/* native/
cat > native/README <<NATIVE
Sources of the codec libraries the ddsonic binary links statically. flac,
libvorbis and libogg are Debian 12 source packages (the .orig archive plus
Debian's changes); mpg123 is the upstream archive. packaging/Dockerfile shows
how each is built, and packaging/release.sh how ddsonic is linked against them.
NATIVE

go build -a -trimpath -mod=vendor \
	-ldflags="-s -w -buildid= -X main.version=${version} -extldflags '-lvorbisenc -lvorbis -lFLAC -logg -lmpg123 -lm'" \
	-o "$work/ddsonic" .
if readelf -d "$work/ddsonic" | grep -E 'Shared library:.*(FLAC|vorbis|ogg|mpg123)'; then
	echo "release: codec libraries must be linked statically" >&2
	exit 1
fi
got=$("$work/ddsonic" --version)
case "$got" in
*"$version"*) ;;
*) echo "release: --version says '$got', want $version" >&2; exit 1 ;;
esac

pack() { # archive directory-name, from $work
	tar -C "$work" --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner \
		-cf - "$2" | gzip -n -9 > "/out/$1"
}

bin="$name-linux-amd64"
mkdir "$work/$bin"
cp "$work/ddsonic" "$work/$bin/ddsonic"
cp LICENSE LICENSE-GPL-3.0 README.md CHANGELOG.md "$work/$bin/"
# The launcher entry and the icons, laid out as they install under a prefix.
sh packaging/desktop.sh "$work/$bin/share"
sh packaging/notices.sh "$work/ddsonic" > "$work/$bin/THIRD_PARTY_NOTICES"
# Licenses that sit beside a package, not at its module's root, must be there.
for nested in github.com/dop251/goja/ftoa/internal/fast/LICENSE_V8 \
	google.golang.org/api/internal/third_party/uritemplates/LICENSE; do
	if [ -d "vendor/$(dirname "$nested")" ] && ! grep -qF -- "--- $nested ---" "$work/$bin/THIRD_PARTY_NOTICES"; then
		echo "release: THIRD_PARTY_NOTICES lacks $nested" >&2
		exit 1
	fi
done
cp "$work/$bin/THIRD_PARTY_NOTICES" "$tree/THIRD_PARTY_NOTICES"

pack "$bin.tar.gz" "$bin"
pack "$name-source.tar.gz" "$name-source"
sh packaging/sbom.sh "$work/ddsonic" "$version" > "/out/$name-sbom.cdx.json"
cp "$work/$bin/THIRD_PARTY_NOTICES" "/out/$name-THIRD_PARTY_NOTICES.txt"

cd /out
sha256sum "$bin.tar.gz" "$name-source.tar.gz" "$name-sbom.cdx.json" "$name-THIRD_PARTY_NOTICES.txt" > SHA256SUMS
cat SHA256SUMS
