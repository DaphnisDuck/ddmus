# Releasing ddmus

ddmus releases one build: Linux x86-64, from the pinned environment in `packaging/Dockerfile`. The executable is GPL-3.0 (it links go-librespot); ddmus's source is MIT.

## What a release holds

| File | What it is |
| --- | --- |
| `ddmus-<version>-linux-amd64.tar.gz` | The `ddmus` binary, `LICENSE`, `LICENSE-GPL-3.0`, `THIRD_PARTY_NOTICES`, `README.md`, `CHANGELOG.md` |
| `ddmus-<version>-source.tar.gz` | The complete corresponding source: ddmus at the tag, every Go dependency (`vendor/`), the codec libraries' sources (`native/`) and the build scripts (`packaging/`) |
| `ddmus-<version>-THIRD_PARTY_NOTICES.txt` | The notices, also inside both archives |
| `ddmus-<version>-sbom.cdx.json` | A CycloneDX SBOM: Go modules, the Go toolchain, native libraries |
| `SHA256SUMS` | Checksums of the four files above; the build attestation covers the same files |

## How the binary is built

- **Toolchain:** Go 1.27.1, from an image pinned by digest.
- **Codecs:** FLAC, Vorbis and Ogg from one Debian 12 snapshot and mpg123 from a checksummed source archive, all linked statically, so the binary doesn't depend on the codec versions of the machine it runs on.
- **Dynamic:** ALSA and glibc. Debian 12 sets the baseline, glibc 2.36. Tested on Arch and Debian 13.
- **Reproducible:** two builds of the same commit in this image give the same bytes, and the source archive rebuilds to the same binary with no network.

`THIRD_PARTY_NOTICES` and the SBOM are generated from the built binary (`go version -m`), so they list what ships. A dependency with no license file stops the build until `packaging/notices.sh` has a note for it; today that is `xlab/vorbis-go` only.

## Cutting a release

1. `CHANGELOG.md` has a section for the version (`## [1.0.0]`). A candidate (`v1.0.0-rc.1`) uses the section of the version it leads to.
2. Switch the repository's tag ruleset off (Settings → Rules → Rulesets), tag the commit, push that one tag (`git push origin v1.0.0-rc.1`), and switch the ruleset back on. The ruleset refuses every new, moved or deleted tag, so that cliamp's tags can never reach ddmus's repository by accident: a tag push runs the workflow of the tagged commit, and cliamp's would publish cliamp releases here. Locally, cliamp's tags live under `refs/upstream-tags/` for the same reason; see [upstream.md](upstream.md).
3. The Release workflow (actions pinned by commit) builds the files, installs the archive in a fresh Debian 13 container, attests the build, and creates a **draft** release. A tag with a hyphen is marked as a prerelease.
4. Check the draft, then publish it by hand.
5. Stable releases only: update `packaging/aur/PKGBUILD` (`pkgver`, `updpkgsums`, `.SRCINFO`) and push it to the AUR. A candidate is tested from the recipe locally and never pushed.

## A dry run on your machine

```sh
docker build -t ddmus-build -f packaging/Dockerfile packaging
mkdir -p dist
docker run --rm -v "$PWD":/src:ro -v "$PWD/dist":/out ddmus-build \
    /src/packaging/release.sh v1.0.0-test.1 "$(git log -1 --format=%ct)"
```

This builds the committed tree (`HEAD`); `dist/` is ignored by git. The files are owned by root, as Docker wrote them.

## Moving a pin

Change it in `packaging/Dockerfile` (the Go image digest, `DEBIAN_SNAPSHOT`, the mpg123 version and checksum), keep `mise.toml` and the Go version in `.github/workflows/ci.yml` in step, and cut a new candidate.

## GitHub settings the workflows rely on

- **A tag ruleset** on all tags that restricts creations, updates and deletions, with an empty bypass list. It is switched off only while a release tag is pushed.
- **The "Deploy to GitHub Pages" workflow stays disabled** (Actions → the workflow → Disable workflow). It was cliamp's and is gone from `main`, but the copies in older tags can be started by hand and have no repository check.
- **Private vulnerability reporting** is on (`SECURITY.md` points to it).
