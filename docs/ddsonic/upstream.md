# Syncing with upstream cliamp

ddsonic tracks [bjarneo/cliamp](https://github.com/bjarneo/cliamp) so we keep getting its improvements.

## Remotes

```sh
git remote -v
# origin    git@github.com:DaphnisDuck/ddsonic.git
# upstream  https://github.com/bjarneo/cliamp.git
```

If `upstream` is missing, add it, and keep cliamp's tags out of your tag list:

```sh
git remote add upstream https://github.com/bjarneo/cliamp.git
git config remote.upstream.tagOpt --no-tags
git config --add remote.upstream.fetch '+refs/tags/*:refs/upstream-tags/*'
```

cliamp's release tags (`v1.0.0` to `v2.3.0` and later) share names with ddsonic's own, and their release workflow has no repository check: pushed to ddsonic's GitHub, each would publish a cliamp release there. With the two settings above, a fetch files them under `refs/upstream-tags/` (`git log refs/upstream-tags/v2.3.0`), where `git tag` doesn't list them and `git push --tags` doesn't send them. A clone that already has them as tags can drop them with `git tag -d` after the next fetch. GitHub also refuses new tags unless the tag ruleset is switched off for a release; see [releasing.md](releasing.md).

## Sync workflow

```sh
git fetch upstream
git switch -c sync/upstream-$(date +%Y%m%d) main
git merge upstream/main          # merge, don't rebase: main is public
make check                       # gofmt + vet + tests must pass
git push -u origin HEAD          # open a PR into main
```

When a merge conflicts, start by searching for `// ddsonic:` markers. Every ddsonic edit to an upstream file carries one.

## Rules that keep merges cheap

- Keep the Go module path `github.com/bjarneo/cliamp`. ddsonic keeps its own files; see [files.md](files.md).
- Put new behavior in new files or packages: `library/`, `ui/model/library_*.go`, `external/spotify/library_browse.go`.
- Keep each necessary edit to an upstream file small and tag it with `// ddsonic:` (or `# ddsonic:` in Makefiles and TOML).
- ddsonic docs go in `docs/ddsonic/`. Leave upstream `docs/` alone.
- Don't reformat or reorganize upstream code opportunistically.

## Upstream files ddsonic removed

These build, install or present cliamp, so ddsonic deleted them rather than carry them (2026-10-04):

- `site/`: cliamp's website and its `install.sh`, which downloads cliamp.
- `flake.nix`, `flake.lock`, `nix/`: the Nix package, which builds this checkout as `cliamp`. ddsonic has no Nix package.
- `cliamp.desktop`, `Cliamp.png`, `Cliamp.svg`, `Cliamp.ico`, `cliamp_windows.rc`, `logo.txt`: cliamp's launcher, logo and Windows icon resource.
- `CNAME`.

When upstream changes one of them, the merge reports a modify/delete conflict. Keep it deleted (`git rm <path>`); a file upstream adds under these paths is deleted the same way. ddsonic's own website, icon or launcher, when they exist, are new files under ddsonic's names.

The `protocol` and `open` commands' code (`cmd/protocol*.go`, with its `cliamp-url-handler.desktop` template) stays: it is upstream Go code that ddsonic's command line doesn't reach (`cli_ddsonic.go`).
