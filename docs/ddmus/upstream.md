# Syncing with upstream cliamp

ddmus tracks [bjarneo/cliamp](https://github.com/bjarneo/cliamp) so we keep getting its improvements.

## Remotes

```sh
git remote -v
# origin    git@github.com:DaphnisDuck/ddmus.git
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

When a merge conflicts, start by searching for `// ddmus:` markers. Every ddmus edit to an upstream file carries one.

## Rules that keep merges cheap

- Keep the Go module path `github.com/bjarneo/cliamp`. ddmus keeps its own files; see [files.md](files.md).
- Put new behavior in new files or packages: `library/`, `ui/model/library_*.go`, `external/spotify/library_browse.go`.
- Keep each necessary edit to an upstream file small and tag it with `// ddmus:` (or `# ddmus:` in Makefiles and TOML).
- ddmus docs go in `docs/ddmus/`. Leave upstream `docs/` and `site/` alone.
- Don't reformat or reorganize upstream code opportunistically.
