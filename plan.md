# omatunes plan

This is the working anchor for the omatunes fork of cliamp. Read it before starting any task. Update **Status** and **Decisions** as work lands.

## Vision

You shouldn't have to remember where your music lives. omatunes turns cliamp's provider-oriented TUI into a library-oriented player:

- The application owns navigation.
- Providers own capabilities and content.
- Playback (the player, queue, now playing, EQ and visualizer) stays cliamp's and is left alone.

### Non-goals (until explicitly scheduled)
- No replacement audio engine, and no giant provider rewrites.
- No automatic metadata correction or MusicBrainz enrichment, no recommendations, no smart playlists.
- No requirement that every provider support every library concept.

## Milestones

### M1: Library-oriented navigation (Spotify, Local, Radio)
Target hierarchy:
```
Music
├── Spotify   Albums → Album → Tracks · Artists → Artist → Albums · Playlists → Tracks · Liked Songs
├── Local     Albums · Artists · Genres · Folders · Playlists
├── Radio     Favorites · Browse Stations
└── Search    (placeholder; routes to existing search until M3)
```
Keys:

| Key | Action |
|---|---|
| `j` `k` | Down / Up |
| `g` `G` | Top / Bottom |
| `l` `Enter` | Enter |
| `h` `Esc` `Backspace` | Back |
| `/` | Search |
| `Space` | Play/Pause |
| `Tab` | Switch focus between library and queue |
| `q` | Quit at the root; back everywhere else |

Other cliamp jump keys are disabled by an allowlist table.

Done when:
- [ ] Existing playback works.
- [ ] Spotify auth works.
- [ ] Local playback works.
- [ ] Radio works.
- [ ] Queue and Now Playing work.
- [ ] Back navigation is predictable.
- [ ] Provider code has changed minimally.
- [ ] No database exists.
- [ ] `go test ./...` passes.

### M2: Persistent catalog and background sync
- SQLite at the XDG data dir (`$XDG_DATA_HOME/cliamp/library.db`), created automatically. Schema changes go through migrations.
- A Catalog interface between the UI and SQLite. The UI never runs SQL.
- Spotify sync covers albums, artists as needed, playlists with playlist_tracks, and liked songs. It runs asynchronously and uses generation marking. Deletions are reconciled **only after a complete, successful sync**.
- Local library indexing into the same catalog.
- Starting the app never waits on sync. Browsing works offline from the cache.
- `CatalogUpdatedMsg` requeries the view. A small status line shows sync state (`↻`/`✓`/`sync failed · cached library available`).
- Freshness policy (`[cache] spotify_refresh`) and a manual refresh key (`r`).
- Tests cover the catalog and sync, using temp databases:
  - Empty DB plus A B C gives A B C.
  - A B C synced against A C D gives A C D.
  - A failure on page 2 leaves A B C intact and marks the sync failed.
- Not in M2: dedup across providers, FTS search.

### M3: Unified search
- FTS5 over the catalog, with a global `/` and search-as-you-type.
- Ranking: BM25 plus field weights.
- Results are normalized to `SearchResult{Type, ID, Provider, Title, Subtitle, Score}`. Selecting one routes back to its provider for playback.
- A top-level Library node with unified Albums and Artists. Source-specific browsing stays available.
- Radio stations show up as a separate result type.
- Duplicates stay separate unless the identity match is high-confidence.
- The parser should leave room for operators (`artist:`, `provider:` ...).

### M4: Multi-provider expansion
- One provider per release, starting with YouTube Music.
- Each provider answers three questions: Browse? Cache? Play?
- Providers advertise capabilities. The app never assumes a provider behaves like Spotify.

## Architecture invariants
1. `library/` defines the navigation Node model and has no Bubbletea dependency. `ui/model` only talks to Nodes (and, from M2, the Catalog). The exception is playback, which uses the existing cliamp paths.
2. Capability detection (type assertions on `provider/interfaces.go`) lives in the `library/` adapters, never in the view layer.
3. From M2, the catalog has its own internal IDs. `(provider, provider_item_id)` is unique but is never the primary key.
4. Sync never deletes cached rows after a partial run.
5. Secrets (tokens, credentials) never go into the catalog database.

## Upstream policy
- The `upstream` remote is `https://github.com/bjarneo/cliamp.git`. `origin` is `DaphnisDuck/omatunes`.
- To sync, create a branch `sync/upstream-YYYYMMDD`, run `git fetch upstream && git merge upstream/main`, then `make check`, then open a PR. Merge; don't rebase public history.
- Keep the Go module path `github.com/bjarneo/cliamp`. omatunes has its own config, data, downloads and MPRIS name so it runs beside cliamp (see `docs/omatunes/files.md`).
- Put new code in new files and packages (`library/`, `ui/model/library_*.go`, `external/spotify/library_browse.go`).
- Keep each unavoidable edit to an upstream file small, and tag it with a `// omatunes:` comment.
- omatunes docs live in `docs/omatunes/`. Upstream's `docs/` and `site/` are left untouched to avoid conflicts, so the CLAUDE.md rule "keep site in sync" applies to upstream-style changes only.

## Status
- [x] M0: add the `upstream` remote, create `plan.md`, add the CLAUDE.md fork note, write `docs/omatunes/upstream.md`, make the Makefile build `omatunes`, rebrand the UI title and terminal title.
- [x] M1.1: `library/` Level/Entry model and root menu, with tests.
- [x] M1.2: Spotify adapter (Playlists/Liked Songs over `Playlists()` sections, pinned by `external/spotify/library_contract_test.go`) and `external/spotify/library_browse.go` (AlbumBrowser + ArtistBrowser; `user-follow-read` was already in the scopes).
- [x] M1.3: Radio adapter (Favorites via `f:` IDs; Browse Stations = cliamp radio channels + the provider's GenreBrowseRouter routes).
- [x] M1.4: Local adapter (Folders intent → file browser, Playlists).
- [x] M1.5: Local in-memory scan (`library/localscan.go`: Albums/Artists/Genres via `resolve.AudioFiles` + the existing tag reader).
- [x] M1.6: key allowlist gate (`ui/model/library_nav.go`). Follow-up: the `?`/`Ctrl+K` keymap overlay still lists upstream bindings, including disabled ones.
- [x] M1.7: `main.go` wiring (`main_omatunes.go`), starting on the Library screen.
- [x] M1.8: `docs/omatunes/navigation.md`.
- [x] M1.9: manual test and refinement pass with the user; omatunes given its own files (docs/omatunes/files.md). M1 complete.

## Decisions log
- 2026-09-29: Spotify Artists means followed artists through a new `ArtistBrowser` implementation in `external/spotify/library_browse.go`.
- 2026-09-29: Rename only the binary and UI; the module path stays cliamp. (Config dir superseded below.)
- 2026-09-29: Providers other than Spotify, Local and Radio are still constructed but hidden from the root until M4.
- 2026-09-29: Local Albums, Artists and Genres come from an in-memory tag scan in M1 (confirmed). The M2 indexer replaces it.
- 2026-09-29: Spotify saved albums come through `provider.AlbumBrowser` (`external/spotify/library_browse.go`), not by splitting the provider-pane "Artist - Album" labels. Radio favorites come through `FavoriteTracks()` (`external/radio/library_favorites.go`), so an active catalog search cannot empty them. Adapters only use advertised capabilities.
- 2026-09-29: omatunes gets its own files so it coexists with cliamp: `internal/appdir.Name = "omatunes"` drives ~/.config/omatunes, ~/.local/share/omatunes, ~/Music/omatunes and the plugin write allowlist; `appmeta` names drive the MPRIS bus name. `CLIAMP_CONFIG_DIR` is checked first (upstream tests set it to isolate themselves), then `OMATUNES_CONFIG_DIR`. User data was copied from ~/.config/cliamp once.
- 2026-09-29: Deferred to M2: cancelling provider calls on Back (the provider interfaces take no context), and caching followed artists. The catalog replaces both.

## Open questions
- M2 SQLite driver: the recommendation is `modernc.org/sqlite` (pure Go, no CGO, FTS5).
- The M2 migrations layout: `catalog/migrations/NNN_*.sql` embedded with `embed.FS`.
- Default `spotify_refresh` interval: 15m or 30m?
- Where Local's `music_dir` config lives: a `[omatunes]` block, or reuse `initial_directory`?
