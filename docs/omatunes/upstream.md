# Syncing with upstream cliamp

omatunes tracks [bjarneo/cliamp](https://github.com/bjarneo/cliamp) so we keep getting its improvements.

## Remotes

```sh
git remote -v
# origin    git@github.com:DaphnisDuck/omatunes.git
# upstream  https://github.com/bjarneo/cliamp.git
```

If `upstream` is missing, add it:

```sh
git remote add upstream https://github.com/bjarneo/cliamp.git
```

## Sync workflow

```sh
git fetch upstream
git switch -c sync/upstream-$(date +%Y%m%d) main
git merge upstream/main          # merge, don't rebase: main is public
make check                       # gofmt + vet + tests must pass
git push -u origin HEAD          # open a PR into main
```

When a merge conflicts, start by searching for `// omatunes:` markers. Every omatunes edit to an upstream file carries one.

## Rules that keep merges cheap

- Keep the Go module path `github.com/bjarneo/cliamp` and the config dir `~/.config/cliamp`.
- Put new behavior in new files or packages: `library/`, `ui/model/library_*.go`, `external/spotify/library_browse.go`.
- Keep each necessary edit to an upstream file small and tag it with `// omatunes:` (or `# omatunes:` in Makefiles and TOML).
- omatunes docs go in `docs/omatunes/`. Leave upstream `docs/` and `site/` alone.
- Don't reformat or reorganize upstream code opportunistically.
