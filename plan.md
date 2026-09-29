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

## M2 implementation plan

Goal: the UI browses a local SQLite catalog instantly. Background sync keeps it current from Spotify and the local music folder. A failed sync never damages the cache. Playback still goes through the providers.

### Packages
```
catalog/                 Catalog interface + domain types (no SQL, no Bubbletea)
catalog/sqlite/          modernc.org/sqlite implementation; migrations/*.sql via embed.FS
catalog/sqlite/migrations/001_initial.sql, 002_…
catalogsync/             sync engine (named to avoid clashing with stdlib sync)
catalogsync/spotifysrc/  Spotify source: pages raw data → catalog records
catalogsync/localsrc/    local-folder indexer (replaces library/localscan.go)
```
- UI and `library/` import only `catalog`, never `catalog/sqlite` (invariant 1).
- `main_omatunes.go` opens the store, starts the syncer and passes the `Catalog` to `library.Root`.

### Storage
- **File:** `~/.local/share/omatunes/library.db` (`appdir.LibraryDBPath()`, in the same `DataDir` as the album-art cache).
- **Driver:** `modernc.org/sqlite`. Pin v1.59.0; v1.60.0 is one day old, so only take it if a test needs it. Driver name `"sqlite"`.
- **DSN:** `_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate`. Immediate transactions take the write lock up front, so a sync never fails upgrading a read lock.
- **Writes:** one connection is enough, because only the syncer writes (`SetMaxOpenConns` on a dedicated write handle). WAL lets reads run while a sync writes.
- **Migrations:**
  - A `schema_migrations(version, applied_at)` table.
  - Each migration file runs in its own transaction at open, in order, and is never edited once merged.
  - A newer DB than the binary refuses to open read-write and says so.
- **FTS5:** spike test on day one: `CREATE VIRTUAL TABLE t USING fts5(x)` must succeed with the pinned driver. M3 depends on it.

### Schema v1 (001_initial.sql)
The schema lives in `catalog/sqlite/migrations/001_initial.sql`, which is authoritative. In summary:
- **Identity:** every catalog object has an internal integer ID and a unique `(provider, provider_id)` (invariant 3).
- **Entities:** `artists`, `albums`, `tracks` and `playlists`.
  - `albums` and `tracks` store a denormalized `artist_credit`, and albums also store `sort_artist`, all written by the sync with the `album_artists`/`track_artists` junction rows. List queries then need no per-row joins.
  - Sort columns always come from `catalog.SortKey`.
  - `albums.tracks_cached_at` marks the lazy album-track cache.
  - `playlists.own` records ownership, and `playlists.snapshot` is the provider's change marker.
- **Membership:** `library_items(provider, collection, kind, item_id, added_at, last_seen_gen)`, where `kind` is one of album, artist, track or playlist. Each row belongs to the sync collection that saw it, so reconciling a collection deletes only its own stale rows (`DELETE … WHERE provider=? AND collection=? AND last_seen_gen<?`), never entities. `item_id` is polymorphic with no foreign key. Unreferenced entities are removed by `Writer.Sweep`, which runs once per provider sync with explicit keep-alive rules (see `catalog/sqlite/write.go`).
- **Sync bookkeeping:** `sync_state(provider, collection, generation, last_attempt_at, last_success_at, last_error)`, and `local_files(path, size, mtime_ns, track_id)` for the indexer.
- **Storage details:** junction and membership tables are `WITHOUT ROWID`. Every foreign-key child column is indexed, so track deletes don't scan whole tables. Times are Unix milliseconds.

### Catalog API (sketch; it will evolve)
```go
type Catalog interface {
    Albums(ctx, provider string, opt ListOpts) ([]Album, error)       // library albums, sorted
    AlbumTracks(ctx, albumID int64) ([]Track, bool /*cached*/, error)
    Artists(ctx, provider string) ([]Artist, error)
    ArtistAlbums(ctx, artistID int64) ([]Album, error)
    Playlists(ctx, provider string) ([]Playlist, error)
    PlaylistTracks(ctx, playlistID int64) ([]Track, error)
    LikedTracks(ctx, provider string) ([]Track, error)
    SyncStatus(ctx, provider string) (SyncStatus, error)
    // write side, used only by catalogsync:
    BeginSync(ctx, provider, collection string) (SyncTx, error)       // opens a tx, bumps generation
}
type SyncTx interface { UpsertAlbums(...); UpsertTracks(...); Seen(kind, ids...); Commit() error; Rollback() }
```

### Sync engine
- **Sources:** each source implements `Collections()` and `Fetch(ctx, collection) iter/pages`. The engine owns transactions and reconciliation, so sources stay simple and testable with fakes.
- **One collection run** (saved albums, followed artists, playlists, liked tracks, local files):
  1. Fetch every page into memory. Spotify's scale (thousands of rows) is fine.
  2. If any page fails, record `last_error`, leave the catalog untouched and stop.
  3. Otherwise, in one transaction: bump the generation, upsert entities, mark membership seen with the new generation, delete this provider's membership rows with an older generation (reconciliation happens only here), commit, and record `last_success_at`.
- **Playlists:** the playlist list reconciles like any collection. Each playlist's tracks re-sync only when its `snapshot` changes, in a per-playlist transaction that replaces its `playlist_tracks` rows.
- **Album tracks, lazy plus background fill (confirmed):**
  - Opening an uncached album loads it from the provider, shows it, and writes the tracks to the catalog.
  - A low-priority filler caches uncached saved albums one at a time, with a delay and exponential backoff on HTTP 429.
  - It pauses while a foreground load is running and stops on shutdown.
- **Scheduling:**
  - At startup, if `now - last_success_at > spotify_refresh` (default 30m), start a background sync. Local folder indexing always runs at startup, incrementally, skipping files whose size and mtime are unchanged.
  - `r` in the library forces a sync of the current source.
  - One sync per provider at a time; a second request while one is running is dropped.
- **Lifecycle:** the syncer runs on a context cancelled when omatunes quits, and `main` waits for it to stop (with a short deadline) before closing the DB.
- **UI messages:** `prog.Send(catalog.UpdatedMsg{Provider, Collection})` after each commit, and `SyncStatusMsg` on start, fail and success. These message types live in `internal/playback`-style shared code so the UI doesn't import the syncer.

### Spotify source data
`playlist.Track` has names but no album or artist IDs, so the source needs richer data. Add fork-owned methods in `external/spotify/library_browse.go` that return plain structs defined in a new fork-owned `provider/catalog.go` (for example `CatalogAlbum`, `CatalogTrack` with provider IDs for album and artists). Candidates:
- `SavedAlbumsPage(ctx, offset)`
- `FollowedArtistsPage(ctx, after)`
- `SavedTracksPage(ctx, offset)`
- `PlaylistItemsPage(ctx, id, offset)`
- `AlbumTracksCatalog(ctx, id)`

These take a context, which also fixes the M1 "Back can't cancel provider calls" deferral for synced paths.

### UI changes
- **Library adapters** (`library/sources.go`): Spotify and Local levels read from `catalog.Catalog`, so every level loads instantly from SQLite. Radio stays live; stations aren't catalogued in M2.
- **Playback** is unchanged. Entries carry `playlist.Track{Path: playable_uri, …}` built from catalog rows, so the player routes to the provider exactly as before.
- **Refresh on update:** on `UpdatedMsg`, reload the visible frame if it shows that provider/collection, keeping the cursor on the same item ID.
- **Status:** a right-aligned header indicator shows `↻` while syncing and `✓ 2m` after a success. The footer shows "sync failed · cached library available" when the last attempt failed.
- **Empty cache:** before the first sync, levels show "Syncing your Spotify library…" instead of blocking.

### Config (confirmed: `[omatunes]` in config.toml)
```toml
[omatunes]
spotify_refresh = "30m"   # background sync if the last success is older than this
```
- **Parser:** one small tagged hook in `config/config.go`'s section switch that hands `[omatunes]` keys to a fork-owned `config/omatunes.go`.
- **Future keys:** `library_db` (path override) and `music_dir` (to split Local's scan folder from `initial_directory`).

### Tests (the M2 bar is automated, not manual)
- **catalog/sqlite:** migrations apply to an empty DB and are idempotent on reopen; round trips per table; sort order; constraint violations; a newer DB than the binary refuses to open. Every test uses a temp DB.
- **catalogsync:** table-driven over a fake source:
  - A: empty DB, and the source returns A B C, gives A B C.
  - B: a DB holding A B C, synced against a source returning A C D, gives A C D. B's membership is removed; the B entity may remain if something else references it.
  - C: a DB holding A B C, where page 1 returns A and page 2 errors, leaves A B C intact, records `last_error`, and leaves `last_success_at` unchanged.
  - Also covered: playlist snapshot unchanged means no track rewrite; the local indexer skips unchanged files and removes deleted files after a complete scan; a cancelled context rolls back.
- **UI:** `UpdatedMsg` reloads the visible level and keeps the cursor; the status indicator renders; the uncached-album path fetches and then caches.
- **Race:** `go test -race` covering the syncer running alongside reads.

### Delivery (one PR per slice, each shippable)
- M2.1 Foundation: dependency, `catalog` types and interface, sqlite store, migration runner, `001_initial.sql`, FTS5 spike, DB path helper. No UI change.
- M2.2 Sync engine: generations, reconciliation and failure safety with a fake source, plus scenarios A–C.
- M2.3 Spotify source: fork-owned richer page methods and the source for saved albums, followed artists, liked tracks, and playlists with snapshots.
- M2.4 UI on the catalog: adapters read the catalog, startup opens the DB and starts the syncer, `UpdatedMsg` refresh, status indicator, `r` refresh, and graceful shutdown.
- M2.5 Lazy album tracks and the background filler with 429 backoff.
- M2.6 Local indexer into the catalog, replacing `library/localscan.go` (incremental by mtime and size).
- M2.7 `[omatunes]` config, docs (`docs/omatunes/catalog.md`), the offline test (browse with the network off), and a `v0.2.0` tag.

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
- [x] M2.1 Foundation: modernc.org/sqlite v1.59.0, `catalog` types/interface + `SortKey`, `catalog/sqlite` store (WAL, immediate transactions, 0700 dir, race-safe migrations), `001_initial.sql`, read queries, FTS5 verified, `appdir.LibraryDBPath`.
- [x] M2.2 Sync engine (`catalogsync`): per-collection snapshots applied in one transaction (generation bump, upsert that never blanks known data, reconcile) or a recorded failure that leaves the cache untouched; `Writer.Sweep` once per provider sync with explicit keep-alive rules (membership, playlists, local files, cached albums kept whole); single-connection writer; scenarios A–C plus cancellation, collection isolation and concurrency tests.
- Deferred from M2.2 review: prepare statements once per transaction; skip rewriting unchanged rows (WAL churn); per-playlist transactions so a large first playlist sync doesn't hold the writer for seconds (M2.3); a source-side guard against a suspiciously empty "complete" snapshot, e.g. Spotify returning 0 saved albums when the catalog holds 2,000 (M2.3).
- [x] M2.3 Spotify source: `external/spotify/catalog_sync.go` (context-aware whole-collection fetchers returning catalog records with Spotify IDs; every paged read must match Spotify's reported total or fails with ErrIncomplete; 403/404 playlists map to catalog.ErrForbidden and keep their stored tracks) and `catalogsync/spotifysrc` (albums, artists, liked, playlists; playlist tracks fetched only when the snapshot changed). The Spotify session is now read under its lock (tagged upstream edit), since sync calls it from a background goroutine.
- Deferred from M2.3 review: ensureSession ignores the sync context (upstream code); a typed HTTP status error instead of matching "http status 403/404" text (pinned by a test through the real request path); skipping unchanged albums/liked with a one-request total+newest check, as savedTracksUnchanged does; detecting a same-total edit mid-read (rare; the next sync corrects it).
- [x] M2.4 UI on the catalog: `library.SpotifyCatalog` (catalog-backed levels with live fallbacks for uncached albums, unsyncable playlists and discographies), `main_catalog.go` (open, sync when stale >30m, ordered events, `r`, cancel+wait on quit), in-place refresh keeping the cursor by ID with stale parents reloaded on Back, header badge. Verified live: 2,141 albums synced in ~20s and open instantly; restart within 30m skips the sync.
- [ ] M2.5 Lazy album tracks plus background filler with 429 backoff.
- [ ] M2.6 Local indexer into the catalog (replaces library/localscan.go).
- [ ] M2.7 `[omatunes]` config, docs, offline check, tag v0.2.0.

## Decisions log
- 2026-09-29: Spotify Artists means followed artists through a new `ArtistBrowser` implementation in `external/spotify/library_browse.go`.
- 2026-09-29: Rename only the binary and UI; the module path stays cliamp. (Config dir superseded below.)
- 2026-09-29: Providers other than Spotify, Local and Radio are still constructed but hidden from the root until M4.
- 2026-09-29: Local Albums, Artists and Genres come from an in-memory tag scan in M1 (confirmed). The M2 indexer replaces it.
- 2026-09-29: Spotify saved albums come through `provider.AlbumBrowser` (`external/spotify/library_browse.go`), not by splitting the provider-pane "Artist - Album" labels. Radio favorites come through `FavoriteTracks()` (`external/radio/library_favorites.go`), so an active catalog search cannot empty them. Adapters only use advertised capabilities.
- 2026-09-29: omatunes gets its own files so it coexists with cliamp: `internal/appdir.Name = "omatunes"` drives ~/.config/omatunes, ~/.local/share/omatunes, ~/Music/omatunes and the plugin write allowlist; `appmeta` names drive the MPRIS bus name. `CLIAMP_CONFIG_DIR` is checked first (upstream tests set it to isolate themselves), then `OMATUNES_CONFIG_DIR`. User data was copied from ~/.config/cliamp once.
- 2026-09-29: Milestone 1 ships as v0.1 (tag v0.1.0). README rewritten for Omatunes; upstream packaging, sponsorship and video removed; credit to cliamp kept prominent.
- 2026-09-29: Deferred to M2: cancelling provider calls on Back (the provider interfaces take no context), and caching followed artists. The catalog replaces both.

- 2026-09-29: M2 uses `modernc.org/sqlite` (pure Go, FTS5), pinned to v1.59.0 unless a newer one is needed. It is the fork's first new dependency.
- 2026-09-29: Album tracks are cached lazily on first open, and a background filler completes the rest with rate-limit backoff.
- 2026-09-29: M2 settings live in an `[omatunes]` section of config.toml, parsed by a fork-owned file through one tagged hook.
- 2026-09-29: Library membership (`library_items`) is kept separate from catalog entities; reconciliation deletes membership, never shared entities.
- 2026-09-29: Spotify maps its API responses to catalog records inside the fork-owned `external/spotify/catalog_sync.go` (the provider knows its data best); `spotifysrc` only chooses what to fetch. This supersedes "plain structs in provider/catalog.go".
- 2026-09-29: The "suspiciously empty snapshot" guard is the completeness check: a read must return exactly the total Spotify reports. An API that reports total 0 is trusted (you really emptied the collection).

## Open questions
- Whether `music_dir` should split from `initial_directory` (the Local scan folder vs the file browser's start folder). Default: keep reusing `initial_directory` until someone needs them apart.
- Artwork: cache album art images locally for offline display, or store URLs only (M2 stores URLs only).
- Whether radio favorites move into the catalog (M3 needs station search).
