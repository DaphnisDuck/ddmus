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

### M5: Deferred cleanup
- Pay down what M1–M4 deferred: the keymap overlay, enrichment and YouTube sync robustness, catalog writer performance, sync correctness, and the live offline check. No new providers.

### M6: Rename to ddmus (DaphnisDuck's Music Player)
- Omatunes is already another music player's name. The fork becomes **DaphnisDuck's Music Player**, `ddmus` for short: the binary and CLI, code references, config and data folders, MPRIS and IPC names, docs, and the GitHub repository. Existing omatunes data moves over on first start.

### M7: The queue view
- Make the queue (Now Playing) view work under the library: shuffle and repeat, play next, track info, queue editing and sound keys come back. The settings panel's SRC shows the queue's source. Every view lists all of its live keys at the bottom, replacing the `?`/`Ctrl+K` overlay.

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

## M3 implementation plan

Goal: `/` from anywhere opens one search over everything the catalog holds (Spotify, Local and your radio stations). Results appear as you type, in well under 50 ms, offline. Selecting a result opens or plays it through its own provider. A top-level Library node lists albums and artists from every source together.

### Decisions (confirmed 2026-09-29)
- **Scope:** as-you-type search covers the catalog only. It's instant, offline and never rate-limited. Explicit rows at the end reach outside it:
  - "Search Spotify for “q”…" opens the existing live Spotify search with the query.
  - "Search the radio directory for “q”…" opens the existing Radio Browser search.
- **Radio:** your favorite stations and your `radios.toml` stations go into the catalog as provider `radio`. Favorites always rank above other stations.
- **Duplicates:** the same album or artist from two sources stays two rows, labelled with the source (Spotify / Local) and sorted next to each other. Merging is deferred.
- **Enter on a track** plays the track's album, starting at that track. An uncached Spotify album is fetched and cached first (M2.5 path). Offline, or with no album, it plays just the track.

### Index (migration `002_search.sql`)
- **Tables:** one FTS5 table per kind, keyed by the entity's catalog ID (`rowid = id`). Sections then fall out naturally, and each kind gets its own limits.

  | Table | Columns |
  |---|---|
  | `artists_fts` | name |
  | `albums_fts` | title, artist |
  | `tracks_fts` | title, artist, album, genre |
  | `playlists_fts` | name |

- **Tokenizer:** `unicode61 remove_diacritics 2` ("dvorak" finds Dvořák), with `prefix='1 2 3'` so prefix queries stay fast.
- **Kept in sync by triggers** on the entity tables: after insert, delete, and update of searchable columns. Updates are guarded by `WHEN OLD.x IS NOT NEW.x …`, so re-syncing unchanged rows doesn't churn the index. An album retitle also updates its tracks' `album` column.
- **Backfill:** the migration fills the tables from existing rows, so an upgraded catalog is searchable without a resync.
- **Budget:** measure on the real catalog, about 80k rows (2k Spotify albums plus a 37k-file local library).
  - A query must run in under 15 ms at p95.
  - The first local index may slow by at most 50% (7.1 s today).
  - Unchanged startups must not slow down.

### Query language (`catalog.ParseQuery`, pure Go, no SQL)
- **Plain words** match as prefixes in any field (`bee sym` finds "Beethoven Symphony No. 5"). `"quoted text"` is a phrase.
- **Operators:**
  - `artist:`, `album:`, `title:` and `genre:` limit a word or phrase to one field.
  - `source:` (or `provider:`) takes `spotify`, `local` or `radio`.
  - `type:` takes `artist`, `album`, `track`, `playlist` or `station`.
  - An unknown `foo:` is plain text.
- **Safety:** `catalog/sqlite` turns the parsed query into an FTS5 `MATCH` string. Every token is quoted there, so no user input reaches FTS syntax unescaped.

### Catalog API
```go
type Query struct { Terms []Term; Providers []string; Kinds []SearchKind }
type Term  struct { Field string /* "" = any */; Text string; Phrase bool }

type SearchKind string // artist, album, track, playlist, station
type SearchResult struct {
    Kind     SearchKind
    Provider string
    Score    float64 // lower is better (bm25 plus boosts)
    // Exactly one is set; the library builds entries from it.
    Artist *Artist; Album *Album; Track *Track; Playlist *Playlist
}
Search(ctx, q Query, limit int) (map[SearchKind][]SearchResult, error) // per-kind top `limit`
```
- **Ranking:** bm25 with field weights (tracks: title 10, artist 5, album 3, genre 1; albums: title 10, artist 5). Then:
  - a boost for library members (saved albums, followed artists, liked and local tracks)
  - a boost for exact title matches
  - for stations, favorites first
- **Stations** are `radio` tracks, reported with Kind `station`.

### Radio in the catalog (`catalogsync/radiosrc`)
- **Records:** stations are track records under provider `radio`: title is the station name, genre its tags, and playable URI its stream URL.
- **Collections:** `favorites` and `custom`, both read from local files through the radio provider. It's instant, needs no network, and reuses the engine's generation and reconcile logic.
- **When it syncs:** at startup, on `r` in Radio, and after a favorite is toggled in the library.
- **Radio → Favorites** stays read from the radio provider; only search reads these rows.

### UI
- **Opening search:** `/` from anywhere in the library, or the root's Search entry, opens the search screen: an input line above the results.
- **Typing:** each keystroke re-runs the query, superseding the one in flight (generation plus context). A short debounce of about 60 ms keeps key repeat smooth.
- **Results:**
  - Sections in this order: Artists, Albums, Tracks, Playlists, Stations.
  - Limits: 5 artists, 8 albums, 20 tracks, 5 playlists, 5 stations.
  - A section that hits its limit ends in "More…", which opens the full list (up to 200).
  - The two explicit rows for outside search come last.
- **Keys:** `↓`/`Tab` moves from the input into the results. `/` returns to the input. `Esc` in the input closes search, and in the results it goes back to the input. `Enter` in the input jumps to the first result.
- **Routing:** the library builds result entries with the same builders the browse levels use (Spotify catalog, Local catalog, radio). Opening a result is identical to opening it while browsing. The UI still never runs SQL.
- **Library node:** Music → Library → Albums and Artists, across every source.
  - Albums are sorted by artist then title, with the source in the detail line ("Ozawa · 1990 · Local").
  - Opening a row goes to that source's own album or artist level.
  - Source-specific browsing (Spotify, Local, Radio) stays as it is.

### Tests (automated, as in M2)
- **Parser:** table-driven: words, prefixes, phrases, each operator, unknown operators, empty and odd input (unbalanced quotes, a lone `:`), and injection attempts (`"`, `*`, `NEAR`, `-`).
- **Index:**
  - Triggers keep the FTS tables in step through ApplySnapshot, Sweep, CacheAlbumTracks, a local re-index and an album retitle.
  - The migration backfills an existing catalog.
  - Accents and prefixes both match.
- **Ranking:**
  - A title match beats an artist-only match.
  - A library member beats a non-member.
  - A favorite station beats a custom one.
  - Operators filter correctly.
- **Performance:** a benchmark over a synthetic 100k-track catalog, plus a timed run against a copy of the real library.
- **UI:**
  - Typing supersedes stale results.
  - Section limits and "More…" work.
  - Enter on a track plays its album from that track, and the offline fallback plays just the track.
  - Esc and `/` focus behave as specified.
  - The explicit rows open the provider searches with the query.

### Delivery
- M3.1 Search index and API: `002_search.sql` (FTS5 tables, triggers, backfill), `catalog.ParseQuery`, `Store.Search` with ranking, parser/index/ranking tests and the benchmark. No UI.
- M3.2 Radio in the catalog: `catalogsync/radiosrc` (favorites, custom), wiring, resync on favorite toggle.
- M3.3 Search screen: input, as-you-type, sectioned results through the library builders, track→album play, explicit Spotify and radio-directory rows, `/` everywhere.
- M3.4 Library node: unified Albums and Artists with source labels.
- M3.5 `docs/omatunes/search.md`, README, review agents, live check, tag `v0.3.0`.

## M4 implementation plan (YouTube Music, v0.4)

Goal: YouTube Music joins the catalog like Spotify. Your music playlists and liked music are synced in the background, browsable and searchable offline, and played through the existing yt-dlp path. Provider menus become capability-driven, so later providers (M4.x releases) show only the lists they actually have.

### Decisions (confirmed 2026-09-29)
- **Sign-in:** keep cliamp's two modes exactly: `cookies_from` (a browser session through yt-dlp, no Google setup) and `client_id` + `client_secret` (your own Google OAuth client, through the YouTube Data API). OAuth wins when both are set, and `cookies_from` alongside it still lets yt-dlp play private uploads. The config keys and files are unchanged (`ytmusic_credentials.json`).
- **Lists:** YouTube Music shows Playlists and Liked Music (`LM`). Watch later (`WL`) and Liked videos (`LL`) are never listed. Neither mode can list library albums or artists (M4.0), so YouTube's Albums and Artists come from enrichment (below).
- **Non-music (confirmed after M4.0):** cookie mode classifies playlists like OAuth mode: sample one video per playlist and keep the playlist if its YouTube category is Music. yt-dlp's full read gives `categories`. That costs about 4 s per new playlist, once, and is cached.
- **Enrichment (confirmed after M4.0):** a background enricher reads each YouTube track's real track title, artists, album and year through a full yt-dlp read (about 4.4 s per track), gently, newest first, pausing for foreground work like the album filler. Enriched tracks get proper artist credits and an album. YouTube then offers Artists and Albums lists and joins Library → Albums/Artists; an album holds only the tracks you have, since album track lists are not reachable.
- **Refresh:** every 2 hours by default (`[omatunes] youtube_refresh`), plus `r`. Only playlists whose marker changed are refetched.
- **Scope:** YouTube Music only. The non-music "YouTube" and "YouTube (All)" providers stay hidden.

### What the two modes give (from the code; M4.0 confirms live)
| | OAuth (Data API) | Cookies (yt-dlp) |
|---|---|---|
| Playlists | `playlists.list mine`, with item counts | the `youtube.com/feed/playlists` page |
| Music vs other | classified by sampling one video per playlist (cached) | not classified: every playlist counts |
| Tracks | `playlistItems.list`, 50 per call, then `videos.list` for durations | yt-dlp, 100 per run |
| Liked Music | spike: `LM` (Liked Music) or `LL` (liked videos) through `playlistItems` | spike: the `LM` playlist URL |
| Change marker | item count plus the playlist etag | item count |
| Cost | quota: about 1 unit per 50 items, 10,000 a day | time: one yt-dlp run per 100 items |

Track metadata is thin in both: the title is the video title, often "Artist - Song (Official Video)", and the artist is the uploading channel ("X - Topic" becomes X). There is no album.

### Catalog mapping
- **Provider:** `catalog.YouTube = "youtube"`.
- **Track:** `Ref{youtube, videoID}`. The title is the video title and the artist is the cleaned channel; its artist record is keyed by channel ID where the API gives one, else by name. Tracks have no album. PlayableURI is `https://music.youtube.com/watch?v=ID`, the path playback already uses. Duration and AddedAt (when it was added to the playlist) come along where known.
- **Playlists:** a `playlists` collection with snapshot = the change marker; the tracks of an unchanged playlist are not refetched (M2.3's mechanism).
- **Liked:** a `liked` collection of track members, if M4.0 finds it readable in the mode in use; otherwise the list is not offered.
- **Completeness:** a read must see as many raw items as the playlist reports, counting private and deleted videos before they are skipped, or it fails with ErrIncomplete, as Spotify reads do.

### Capability-driven menus (M4.1)
- A source's menu lists come from what it syncs, not from a hard-coded Spotify menu. `SpotifyCatalog` becomes a generic catalog source menu built from the source's lists (Albums, Artists, Playlists, Liked). Spotify keeps all four; YouTube gets Playlists and Liked. Local keeps its own menu (Genres, Folders).
- The UI sees no change: invariant 2 holds, since capability decisions stay in `library`.
- Search: playlist results of any catalogued provider open (today only Spotify's), labelled with their source. A "Search YouTube Music for …" row appears only when the provider can search live (cookie mode's `provider.Searcher`).

### Packages
```
external/ytmusic/catalog_sync.go   fork-owned, context-aware fetchers returning catalog records (OAuth and cookie paths)
catalogsync/youtubesrc/            source: playlists (marker-driven), liked
library/                           generic catalog source menu; search rows for any provider's playlists
main_catalog.go                    YouTube source (refresh 2h), Music → YouTube Music
config/omatunes.go                 youtube_refresh
```

### Tests
- **Fetchers:** paging and completeness per mode against faked API and yt-dlp output; skipped private/deleted videos still counted; classification honoured; markers.
- **Source:** only changed playlists refetched; liked present or absent by capability; failures leave the cache intact (scenarios A–C for YouTube).
- **Library:** the generic menu shows exactly the source's lists, and Spotify's menu is unchanged; YouTube playlists and tracks in search, labelled; Enter on a YouTube track plays just the track (no album).
- **Runtime:** youtube_refresh, `r`, retries, as for Spotify.

### Delivery
- M4.0 Spike (needs one sign-in mode set up): Liked Music readability per mode, item counts versus readable items, quota cost of a full OAuth sync, and yt-dlp time for a cookie sync of your library. Also, with cookies: can yt-dlp list the YouTube Music library's albums and artists (e.g. the library albums/artists pages), and expand a library album (an `OLAK5uy_` album playlist) into its tracks? If so, YouTube gets Albums (and perhaps Artists) through the path cookie mode already uses, without an InnerTube client; the plan's lists are revisited with the results.
- M4.1 Capability-driven source menus, and search rows for any provider's playlists. No visible change for Spotify.
- M4.2 Cookie path first (it is the mode set up and verified): yt-dlp fetchers for the playlists feed, playlists and Liked Music; classification by sampled category; `youtubesrc` with markers.
- M4.3 OAuth path: Data API fetchers behind the same source, `LM` readability checked, quota measured (needs a Google OAuth client to verify live).
- M4.4 Wiring: Music → YouTube Music, `youtube_refresh` (2h), runtime source, search label, the live-search row where available.
- M4.5 Enrichment: background per-track metadata; a sync must not overwrite an enriched track's title and credits (an `enriched_at` marker, migration 003); YouTube Albums and Artists lists and Library membership from enriched tracks.
- M4.6 `docs/omatunes/youtube.md` (both sign-in modes, the `+gnomekeyring` note, quota, what syncs, enrichment), README, review agents, live check, tag `v0.4.0`.

## M5 implementation plan (deferred cleanup, v0.5)

Goal: pay down what M1–M4 deferred before adding another provider. No new sources, no new menus. Every slice either fixes something a user can hit or recovers a missed budget, and each ends green (`make check`, `go test -race ./...`). Branch `m5-cleanup`.

### Scope (checked against the code 2026-09-29)
- **Keys:** the `?`/`Ctrl+K` keymap overlay lists upstream bindings the library gate swallows (M1.6).
- **Enrichment:** an unrecognised permanent yt-dlp error ends every enrichment run at the same track, so nothing after it is ever enriched (M4.6).
- **YouTube sync:** a playlist whose five sampled videos all refuse is left out, and reconciled away if it was already synced (M4.6). You can't switch Google accounts without deleting `ytmusic_credentials.json` (M4.6).
- **Writer performance:** statements are prepared per call, unchanged rows are rewritten on every sync (WAL churn), and a collection is one transaction, so a large first playlist sync holds the writer for seconds (M2.2). The first full local index missed its budget by 55% (6.4 s → 10.0 s, M3.1).
- **Sync correctness:** Spotify 403/404 is still recognised by matching `"http status 403"` text (`external/spotify/catalog_sync.go`, M2.3). Saved albums and liked songs are reread in full on every sync, although one request (total + newest) could show that nothing changed (M2.3). A change to local grouping rules regroups only changed files, because there is no indexer version (M2.7).
- **Verification:** the live offline check (network off, browse and search the cache) has never been run (M2.7, M3.5).

Not taken: `ensureSession` ignoring the sync context (upstream code); a same-total edit during a read (rare, and the next sync corrects it); checking the cookie feed's count (it reports none); telling own from saved playlists in the cookie feed (no data to do it with); `ArtistAlbums` listing out-of-library albums (intended as the offline discography); the open questions below.

### Delivery
- M5.1 Keymap overlay: when the library is enabled, the overlay lists what the library actually does: the allowlisted keys plus omatunes' own (`/`, `r`, `o`, `Tab`). Disabled keys are hidden, not greyed. Upstream's list is unchanged when the library is off. Test: every listed key is one `handleLibraryKey` handles, and every allowlisted key is listed. Update `docs/omatunes/navigation.md`.
- M5.2 Enrichment and YouTube robustness:
  - Per-track failure count (migration 004, `tracks.enrich_failures`). A track that exhausts its retries is skipped for the rest of the run and counted. At 3 failed runs it is marked read, like ErrForbidden.
  - A run still ends after `MaxFailures` consecutive failures on *different* tracks, because that pattern means an outage, not one bad track.
  - A known playlist whose samples all refuse keeps its stored classification and tracks instead of being reconciled away.
  - `omatunes youtube signin --force` signs in anew, to switch accounts.
- M5.3 Writer performance: prepare statements once per snapshot transaction; skip `UPDATE`s whose values are unchanged (compare before writing, or `WHERE … IS NOT` guards); write each playlist's tracks in its own transaction, with the collection's reconcile still running only after every playlist succeeded (the M2 invariant). Measure on a copy of the real catalog before and after: first local index (target ≤ 7 s), an unchanged Spotify sync (WAL bytes written), the first 305-track playlist.
- M5.4 Sync correctness:
  - A typed HTTP status error through the Spotify request path, replacing the text match (the existing test pins behaviour).
  - Albums and liked songs skip the full read when total and newest `added_at` match the catalog, as `savedTracksUnchanged` already does for one case.
  - `localsrc` stores an indexer version, and a bump regroups every file once, from stored tags without rereading them.
- M5.5 Live offline check (scratch HOME, network off: start, browse Spotify/YouTube/Local, search, play Local, `r` fails gracefully with the cached badge), docs (`docs/omatunes/` for anything user-visible), review agents, tag `v0.5.0`.

## M6 implementation plan (rename to ddmus, v0.6)

Goal: nothing a user sees, types or finds on disk says omatunes. The full name is **DaphnisDuck's Music Player**; everything else says `ddmus`. Your library, settings and sign-ins carry over untouched. Branch `m6-rename`.

### What carries the name today (surveyed 2026-09-29)
- **Identity:** `internal/appdir.Name` ("omatunes") drives most of it:
  - the config, data and download folders;
  - the MPRIS bus name and Identity;
  - the IPC socket (`$TMPDIR/omatunes.sock`);
  - the client name sent to Plex, Jellyfin/Emby and Navidrome, and the podcast User-Agent;
  - the PipeWire stream match.
- **Separate from it:** `OMATUNES_CONFIG_DIR`, `appdir/omatunes.go`, the Makefile's `BINARY`, the UI header "O M A T U N E S" and the terminal title, and the `[omatunes]` config section (`config/omatunes.go`, `OmatunesConfig`).
- **Fork-owned files and messages:** `commands_omatunes.go`, `ytmusic/signin_omatunes.go`, the disabled-upgrade message, and `ErrSchemaTooNew`'s text.
- **Tags and docs:** 73 `// omatunes:` tags on upstream edits; `docs/omatunes/` (6 files), README, CLAUDE.md, plan.md.
- **Your data:** `~/.config/omatunes` (config.toml with an `[omatunes]` section, Spotify and YouTube credentials, radio favorites, classification cache, plugins, themes) and `~/.local/share/omatunes` (library.db). No `~/Music/omatunes`, and no installed binary.
- **Unchanged:** the Go module path (`github.com/bjarneo/cliamp`, for cheap upstream merges), the catalog schema, past git tags and history, and past entries in this plan's Status and Decisions sections (they record what was true then).

### Decisions (2026-09-29; Go names and display text chosen during planning, open to change)
- **Names:**
  - `appdir.Name = "ddmus"`: `~/.config/ddmus`, `~/.local/share/ddmus`, `~/Music/ddmus`, `org.mpris.MediaPlayer2.ddmus`, `ddmus.sock`, binary `ddmus`.
  - `DDMUS_CONFIG_DIR` replaces `OMATUNES_CONFIG_DIR`; `CLIAMP_CONFIG_DIR` still comes first, for upstream tests.
- **Display:**
  - `appmeta.DisplayName()` = "DaphnisDuck's Music Player", used for the MPRIS Identity (media widgets show it).
  - The UI header reads "DaphnisDuck's Music Player", and the terminal title is `ddmus`.
- **Config section:** `[ddmus]` (`config/ddmus.go`, `DdmusConfig`, `cfg.Ddmus`). `[omatunes]` is still read, as a deprecated alias with a log note; `[ddmus]` wins when both are present.
- **Moving your data:** on first start, when a ddmus folder is missing and its omatunes folder exists, it is renamed into place (config, data, downloads). If that fails, the app says so and does not start on empty folders. A running omatunes is not detected: quit it first.
- **Code:** tags become `// ddmus:`, and fork files `*_omatunes.go` become `*_ddmus.go`. `docs/omatunes/` becomes `docs/ddmus/`.
- **Local folder and memory:** `~/Documents/projects/omatunes` is renamed last, by you, after M6. Claude's project memory is tied to that path and is copied over at that point.

### Delivery
- M6.1 Identity: `appdir.Name`, `DDMUS_CONFIG_DIR`, `appmeta.DisplayName`, MPRIS Identity, UI header and terminal title, Makefile `BINARY ?= ddmus`, user-facing strings (upgrade message, `ErrSchemaTooNew`, CLI usage). Tests follow.
- M6.2 Data move: `appdir.MigrateLegacy` run first thing at startup (config, data, downloads; rename only when the target is missing), with tests for each case (fresh install, moved, both present, failure). The `[ddmus]` section plus the `[omatunes]` alias. Verified by moving a copy of your real folders under a scratch HOME.
- M6.3 Code and docs:
  - `// omatunes:` → `// ddmus:`; fork files and identifiers renamed.
  - `docs/omatunes/` → `docs/ddmus/`.
  - README as "DaphnisDuck's Music Player (ddmus)", keeping the credit to cliamp.
  - CLAUDE.md and the live, forward-looking parts of plan.md.
  - A grep gate: no "omatunes" outside git history, plan.md's history, and prompt.txt.
- M6.4 GitHub and release:
  - `gh repo rename ddmus` (confirmed with you at that step), then update the origin remote and README links.
  - Live check with your real data moved (after a backup).
  - Review, then tag `v0.6.0`, merge, push.
  - Afterwards you rename the local folder, and the Claude memory is carried over.

## M7 implementation plan (the queue view, v0.7)

Goal: the queue view is fully usable without cliamp's provider screens. Every key a view lists works, and every key that works is listed. Branch `m7-queue`.

### Findings (2026-09-29, from the code and a live look)
- `n`, `a`, `i`, `Ctrl+I`, `z`, `r` (and `x`, `e`, `m`, `[ ]`, `y`, …) do nothing in the queue view because the M1 gate (`queuePassthroughKeys`) swallows them. Their cliamp handlers are intact.
- The bottom bar is cliamp's main-screen `commandHelp`, unaware of the gate. It advertises swallowed keys and "Esc Back to provider", though Esc returns to the Library.
- The settings panel (VOL, EQ, SRC, SHF, RPT, SPD) is reached in cliamp by Tab/Shift+Tab focus, but Tab is the library/queue toggle.
- `SRC [cliamp radio] 1/8` is cliamp's provider pill (the provider index), which the library replaced.
- `?`/`Ctrl+K` opens the keymap in the queue's place (visualizer, queue and settings hidden).
- Terminals send `Ctrl+I` as Tab, so it cannot be a separate key.

### Decisions (confirmed 2026-09-29)
- **Key help:** each view's bottom bar lists every key that works there, wrapping to more lines when needed. The `?`/`Ctrl+K` overlay goes away while the library is enabled, and `Ctrl+G` still hides the bar. It is upstream's overlay, so it stays for cliamp's own screens.
- **Favorite (`n`):** removed. Liking on the source, or omatunes' own playlists, may bring it back later.
- **Queue keys back:**
  - `z` shuffle and `r` repeat (cycle off/all/one);
  - `a` play next and `A` queued list;
  - `i` track info; `Ctrl+I` is dropped;
  - `x` remove, `Shift+↑/↓` move, `Ctrl+Z` undo;
  - `e` EQ preset, `m` mono, `[ ]` speed, `y` lyrics, `Ctrl+J` jump to time.
- **SRC:** shows the queue's source ([Local], [Spotify], [YouTube], [Radio]), display only.
- **Settings panel:** display-only, changed through the keys above. Per-band EQ editing needs panel focus and is out of M6 (presets through `e`).

### Delivery
- M7.1 Queue keys: widen `queuePassthroughKeys` to the confirmed set.
  - Check each works from the library's queue: overlays it opens (`A`, `i`, `y`, `Ctrl+J`) open over the queue and close back to it; `x` and `Ctrl+Z` on the playing track; `a` on a library-played album; `r` means repeat in the queue while it syncs in the library.
  - `n` and `Ctrl+I` stay swallowed.
  - Tests per key: passes the gate, and has its effect.
- M7.2 SRC: the library records the source of what it plays (the level's catalog provider, or Radio). SRC shows it, and the provider-pill index goes. A queue built by cliamp's own paths (a URL or file argument) shows what cliamp knows, or nothing. Tagged edit in the settings render only.
- M7.3 Key bars:
  - One table per view describes its keys: the library's own rows plus the cliamp commands it passes (M5.1's `libraryKeymapSections` grows into this). The gate, the bottom bar and the tests all read it, so none can drift. The M5 review asked for this.
  - The bottom bar renders it for Library, Search Results, Library Search, and Queue, wrapping within the width. The layout budget accounts for the extra lines.
  - `?`/`Ctrl+K` come out of the passthrough and the bar while the library is enabled.
  - Tests: the bar lists exactly the live keys, and it fits at 80 and 120 columns.
- M7.4 Docs (`docs/omatunes/navigation.md` key tables, README), review agents, live check, tag `v0.7.0`.

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
- [x] M2.5 Lazy album tracks plus background filler: `catalogsync.Filler` (on-demand `FetchAlbumTracks`, which the library uses through the optional `catalog.AlbumTrackFetcher` capability, plus a background `Run` over uncached saved albums, newest saved first, re-listing until nothing new is left); `Store.CacheAlbumTracks`/`UncachedAlbums`; Spotify `AlbumTrackRecords` (retrying, foreground) and `AlbumTrackRecordsOnce` (fork-owned single-attempt `webAPIOnce` that returns `catalog.RateLimitError` with Retry-After, so background 429s neither retry inside webAPI nor warn the user). Pacing: 1s between albums, backoff 5s doubling to 10m (at least Retry-After), rate limits never give up, 3 consecutive other failures end the run, refused/404 albums are skipped. The fill pauses while an album open or a sync holds it, runs after each sync and at startup when no sync is due, and stops on quit. Not yet verified live against Spotify.
- M2.5 follow-ups: a failed sync retries on its own (1m doubling to 30m, reset on success, stopped on quit), so an outage clears without `r` or a restart. A silent Spotify sign-in reports `ErrNeedsAuth` only when signing in again would help (no stored credentials, a revoked refresh token, or login5/accesspoint rejecting the credential); other failures, such as login5 answering 503 "no healthy upstream" (seen 2026-09-29), keep their cause (`external/spotify/session_errors.go`, one tagged line each in `ensureSession` and `NewSessionSilent`).
- [x] M2.6 Local indexer: `catalogsync/localsrc` walks the music folder, rereads tags only for files whose size or mtime changed (unchanged files are rebuilt from their catalog rows), and groups them as the v0.1 scan did (album tag within an album folder, disc folders, Various Artists credited to each track artist). Identity: track = file path, album = folder + lowercased title, artist = lowercased name. Nothing changed returns `catalogsync.ErrUnchanged`, recorded as a success with no writes. A missing folder, or an empty one over a non-empty index, fails and keeps the cache; files under unreadable subfolders are kept. Catalog additions: `Genres`/`GenreAlbums`, `AlbumRecord.Credit`, `TrackRecord.File` with `Snapshot.Files` reconciling `local_files`, `IndexedFiles`, `RecordSyncSuccess`. Sweep no longer keeps a track alive just because its album is a library member (only a cached album does), so a deleted file leaves a surviving album. `library.LocalCatalog` replaces `library/localscan.go`; Local's Albums/Artists/Genres need the catalog. The runtime syncs each provider separately (Spotify when stale, Local at every startup; separate retry backoff), `r` syncs the browsed source (all at the root), and the badge names each provider when there are several. Measured on the real library (37,157 files): first index 7.1s, unchanged re-index 0.75s.
- [x] M2.7 `[omatunes]` config, docs, offline check, tag v0.2.0 (tagged 2026-09-29). Done: `config/omatunes.go` (`[omatunes] spotify_refresh`, default 30m, `0s` = every startup, invalid keeps the default) through tagged hooks in `config/config.go` (struct field, default, section case); `openCatalog(sp, cfg)`; `docs/omatunes/catalog.md` (navigation.md links to it); automated offline test (`TestSpotifyCatalogBrowsesOffline`); `go test -race ./...` clean. Review of M2.5–M2.7 done and applied: Spotify sign-in refusals are classified by code (only INVALID_CREDENTIALS/UNKNOWN_IDENTIFIER, BadCredentials/CouldNotValidateCredentials/ExtraVerificationRequired need sign-in); `ensureWebAPI` restores a Web API token lost to a passing refresh failure for the catalog fetchers; a local index writes exactly (cleared tags clear); albums leaving the library drop their cached tracks in Sweep; dangling symlinks and files deleted mid-walk count as deleted; unchanged startups skip converting stored tracks; shared `catalogsync.NextBackoff`; per-source policy (`source{refresh, fill}`) instead of provider-name checks. The live offline check (browsing with the network off) was not run before tagging; the automated offline test stands in for it.
- Deferred from the M2.5–M2.7 review: an indexer version so a change to the grouping rules regroups unchanged files (today only changed files are regrouped); ArtistAlbums lists every catalog album of the artist, including ones outside the library (intended as the offline discography fallback).
- [x] M3.1 Search index and API: `002_search.sql` (FTS5 per kind with an unindexed provider column, bm25 weights as the tables' rank, `prefix='2 3'`, change-guarded triggers incl. album retitle → tracks, backfill, `library_items (kind, item_id)` index); `catalog.ParseQuery` (words as prefixes, phrases, `artist:`/`album:`/`title:`/`genre:`, `source:`/`provider:`, `type:`; unknown operators are text); `Store.Search` in two phases (FTS picks its best 200 per kind by rank, then membership and exact-title boosts re-rank them), kinds searched concurrently; one-character words match whole words only. Measured on a copy of the real catalog (26k local tracks, 4.8k albums), every prefix typed on the way to eight queries: p50 2.1 ms, p95 13.5 ms, worst 27 ms ("th"). Backfilling the real catalog on upgrade: 0.3 s. Budget missed: the first full local index is about 55% slower (6.4 s → 10.0 s, warm cache), the cost of writing the FTS rows in the pure-Go driver (prefix indexes are not the cause: 9.4 s without them); unchanged startups are unaffected (0.7 s).
- [x] M3.2 Radio stations in the catalog: `catalogsync/radiosrc` (collections `favorites` and `custom`: starred stations, and the built-in plus radios.toml stations; a station's ID is its stream URL), synced at every startup and after every favorite toggle through a fork-owned `radio.Provider.OnFavoritesToggled` hook (one tagged `defer` in `ToggleFavorite`); a `quiet` source, so it reports no status badge. For M3.3: catalog tracks carry no Stream/Realtime flags, so the station result builder must set them.
- [x] M3.3 Search screen: `library/search.go` (a `SearchLevel` per query; sections Artists 5, Albums 8, Tracks 20, Playlists 5, Stations 5 with "More…" up to 200; rows built by each source's own builders, now shared with browsing, labelled Spotify/Local; "No matches in your library"; "Search Spotify for …" reopens cliamp's live Spotify search already running, "Search the radio directory for …" lists `radio.SearchStationTracks` without touching the provider's own search state); `Entry.PlayFrom` plays a searched track's album from it (fetching an uncached Spotify album; just the track offline or album-less); `Catalog.Album`. UI (`ui/model/library_search.go`): `/` from anywhere opens or returns to search with the input focused and the last query kept; typing reloads after 60 ms through the usual superseding load; Enter/↓/Tab to results, `/`/Esc/h back to the query, Esc in the query closes; the cursor is hidden while typing; tracks number within their section. Without a catalog, Search stays the provider search. Verified live on a copy of the real catalog.
- [x] Sorting pass (inserted before M3.4, 2026-09-29): sort keys are literal apart from case, accents and leading punctuation (`catalog.SortKeyVersion` 2; stored keys recomputed once at open, tracked by `PRAGMA user_version`); Albums lists sort by title by default, `o` switches to artist order (`library.OrderedLevel`, header shows the order); Radio → Favorites sorts A–Z, directory lists keep most-voted-first; station rows carry no number (live streams have no track number or duration).
- [x] M3.4 Library node: Music → Library (first in Music) → Albums and Artists across sources (`Catalog.Albums`/`Artists` with an empty provider mean every provider), built with the search's row builders (`searcher.albumRow`/`artistRow`, source label first so a long credit cannot hide it); duplicates stay two rows side by side; rows open their source's own levels; Albums reorders with `o`; the lists are catalog levels of every source (CatalogProvider ""), reloaded after any sync, and `r` there syncs all. Verified live: 4,808 albums (2,667 Local, 2,141 Spotify) and 2,526 artists.
- [x] M3.5 Docs, review, tag v0.3.0 (tagged 2026-09-29). Done: `docs/omatunes/search.md`; README, navigation.md and catalog.md updated for v0.3. Review of M3 applied: a searched track of an uncached album that cannot be fetched plays alone, never the album's known fragment; a radio sync requested mid-run runs once more (request counter), so a favorite toggled during a sync is not lost; leaving the search input runs a pending query at once, so no late tick reloads under the cursor; returning to search keeps the refresh command it popped past; one-letter words count only letters and digits ("a!" is whole-word). Cleanups: `searcher` → `catalogView`; one `library.SourceLabel`; `catalogView.rows`/`trackRow`; fetch size derived from the section limits; exact-title text moved into sqlite; search scans reuse `scanAlbumWith`/`scanTrackWith`, tracks and stations split; sort-key rewrite with prepared statements. Not taken: partial results when one kind errors (not triggerable; would hide failures), paging the 4,800-row Library list (opens instantly), shrinking the opt-in benchmark. The live offline check was not run before tagging.
- [x] M4.0 Spike (cookie mode, 2026-09-29; OAuth untested, no client configured). Brave on Linux needs `cookies_from = "brave+gnomekeyring"` and the system `python-secretstorage` package (plain "brave": "cannot decrypt v11 cookies"). Results with yt-dlp 2026.08.19:
  - Playlists feed (`youtube.com/feed/playlists`): 1.2 s, 9 playlists including Watch later (WL) and Liked videos (LL); mostly non-music (game shows, lessons), which cookie mode does not filter.
  - A playlist (31 tracks): 1.4 s flat; entries match `playlist_count`; each has ID, title, channel, duration.
  - Liked Music (`list=LM`): readable, 1.2 s (4 tracks). Liked videos (`list=LL`): 2.2 s (154).
  - Library albums/artists/songs/playlists (`music.youtube.com/library/...`, `browse/FEmusic_*`): not readable. yt-dlp: "YouTube Music is not directly supported", redirects to youtube.com and gets 404.
  - Album playlists (`OLAK5uy_`): untested; no album ID reachable (the Topic channel tried has no releases or playlists tab).
  - Per-video full extraction (not flat) returns track, artist(s), album and release year, even for an official non-Topic upload (Libertango: Astor Piazzolla, "The Soul of Tango, Greatest Hits", 2000), but costs about 4.4 s per video.
- [x] M4.1 Capability-driven source menus: `library.SyncedSource` (title, player, the collections its sync covers, liked title) and `SyncedMenu`, which offers Albums/Artists/Playlists/liked only for synced collections (`catalog.Collection*`, now also spotifysrc's names); `library/spotify_catalog.go` → `synced_catalog.go`; Root lists each synced source, Spotify browses live only when it is not synced; the runtime reports each source's collections and `librarySources` builds Spotify's `SyncedSource` from them. Search routes albums, artists and playlists to any synced source's browser; playlist rows now carry their source label; `SourceLabel` spells YouTube. No visible change for Spotify (verified live).
- [x] M4.2 YouTube cookie path: `external/ytmusic/catalog_sync.go` (`CookieCatalog`: yt-dlp with `--cookies-from-browser`, through a replaceable runner; playlists feed minus Watch later, Liked videos and Liked Music; classification by the category of the first readable video of five, cached in cliamp's `ytmusic_classification.json` under scope `cookies:<browser>`, a playlist that cannot be classified yet is left out uncached; playlist reads count every entry for completeness (ErrIncomplete) and skip private/deleted videos; channel as artist keyed `channel:<id>`, "X - Topic" → X; missing, private and region-blocked content maps to ErrForbidden) and `catalogsync/youtubesrc` (liked, playlists; a refused playlist keeps its tracks). `catalog.YouTube`; `source:youtube` in queries. The feed gives no per-playlist counts, so cookie playlists carry no marker and are re-read each sync (about 1.4 s each). Live on the real account (into a scratch catalog): first sync 49 s (classifying 7 playlists; a region-blocked sample fell through to the next video), then 4 s; 1 music playlist (31 tracks) and 4 Liked Music tracks.
- [x] M4.3 YouTube OAuth path. Built: `external/ytmusic/catalog_sync_oauth.go` (`OAuthCatalog`: silent session only, else ErrNeedsAuth; `playlists.list mine` minus WL/LL/LM, classified with cliamp's classifier, marker = item count + etag; `playlistItems` paged with a total check, private/deleted skipped, owner channel as artist, AddedAt; durations via cliamp's `fetchDurations`; 404/403 → ErrForbidden), tested against a fake Data API. Live on the real account (2026-09-29, after adding the account as a test user): sign-in works; Liked Music (`LM`) readable, 4 tracks, cleaner artists than cookies; Liked videos (`LL`) readable (147). But `playlists.list mine` reports 18 playlists and returns none (no further pages), and the API has no endpoint for playlists saved from other channels (the cookie feed lists them). A sync costs about 3 quota units plus 2 per newly classified playlist. With both modes configured, `youtubesrc.Mixed` reads Liked Music through OAuth (falling back to cookies when OAuth needs signing in, as a Testing-status Google app does about weekly) and playlists through cookies; with one mode, that mode does everything. Verified live with both: 1 music playlist (31 tracks) through cookies, 4 Liked Music tracks through OAuth.
- [x] M4.4 Wiring: `[omatunes] youtube_refresh` (default 2h); `youtubeClient` picks the sync client from the sign-in (cookies, OAuth, or both → `Mixed`) when YouTube is enabled and yt-dlp is installed; Music → YouTube Music (Playlists, Liked Music) with cliamp's `ytmusic` provider as player; search offers "Search <source> for …" for every synced source whose provider searches live (so none for OAuth-mode YouTube). Verified live (2026-09-29): 4 music playlists including a 305-track one (four yt-dlp pages), a saved one, and "Bubba's Essentials" alongside Spotify's playlist of the same name (two rows labelled Spotify · 34 and YouTube · 3, each with its own tracks); 14 Liked Music tracks newest first; a YouTube track played; YouTube tracks in search beside Spotify and Local; a forced sync with 3 new playlists to classify took about 21 s. Saved playlists: a playlist saved from someone else in YouTube Music (library or bookmark) appears in no list a sync can read (not the youtube.com feed, not the API), though it reads fine by ID. So `[omatunes] youtube_playlists = ["<link or ID>", …]` (one line) lists them: they sync as followed playlists without classification, deduplicated against the feed; a gone one is skipped, other failures keep the cache (`Client.PlaylistRecord`, `youtubesrc.New(client, extra...)`, `youtubesrc.PlaylistID`). Verified live: "Good soup" (14 tracks) synced beside the account's four. Automatic discovery through YouTube Music's private API (InnerTube) is deferred. Cosmetic: the cookie feed does not reliably tell own playlists from saved ones, so saved ones in the feed show under "Your playlists".
- [x] M4.5 Enrichment: migration 003 (`tracks.enriched_at`); a sync keeps an enriched track's title and credits; `catalog.TrackMetadata`; store `UnenrichedTracks` (library tracks: liked or in library playlists, newest first), `EnrichTrack` (writes title, artists, album, year, or only marks a read that found nothing), `RefreshDerived` (the `derived` collection: albums and credited artists of enriched library tracks). `catalogsync.Enricher` (shares the Filler's pause control, now `pauser`; batches of 10 with a derived refresh and a UI reload after each; rate limits back off from 1 min to 30 min; unreadable tracks are marked read; 3 consecutive other failures end a run). `CookieCatalog.TrackMetadata` (full yt-dlp read; artists keyed `artist:<name>`, albums `album:<artist>/<title>`; bot checks and 429 → RateLimitError; cookies optional, so OAuth-only setups enrich anonymously). The runtime's per-source background `worker` (Spotify's filler, YouTube's enricher) is held during that source's sync and run after it; a source's `lists` add menu lists (YouTube: Albums, Artists). `SyncedSource.PartialAlbums`: YouTube albums open and play as the catalog has them, never fetched; the Spotify album fetch refuses other providers' albums. Verified live: within 90 s YouTube Music had Albums (10), Artists and search results, each album holding your tracks of it. A full first enrichment of about 360 tracks takes about 30 min in the background, resuming where it stopped.
- [x] M4.6 Docs, review, tag v0.4.0 (tagged 2026-09-29, merged to main and pushed): `docs/omatunes/youtube.md`, README v0.4, `omatunes youtube signin` (OAuth sign-in outside the TUI). Review fixes: playlist classification never caches a failure or an empty playlist as non-music, and a failed classification (bot check, network, quota) fails the playlists collection so the sync keeps what it has, in both modes (OAuth sync now uses its own `classifyForSync`; cliamp's `classifyPlaylists` is untouched); more permanent yt-dlp refusals (age-gated, members-only, not in your country, private video) map to ErrForbidden so the Enricher marks them read; `Mixed` also falls back to cookies for Liked Music when OAuth refuses (quota); a batch that found nothing skips the derived refresh. Verified live (2026-09-29): a forced sync kept all five playlists, Albums intact, a Liked Music track played, `youtube signin` succeeded silently with the stored token. Known, deferred: an unrecognised permanent yt-dlp error still stops each enrichment run at the same track; the cookie playlists feed has no count to check a short read against; a playlist whose five sampled videos all refuse is left out (and reconciled away if it was synced); a forced sign-in to switch Google accounts. M4 (YouTube Music) complete.
- [x] M5 (deferred cleanup, v0.5): planned and completed 2026-09-29, see the M5 implementation plan. Tagged v0.5.0, merged to main and pushed.
- [x] M5.1 Keymap overlay: with the library enabled, `?`/`Ctrl+K` (main screen only; other overlays keep their own context) lists the current context's own keys (Library, Search Results, Library Search, Queue) and then the cliamp commands whose every key the gate passes through, so `Nj` (digits swallowed), `i`, `a`, `Ctrl+F`, provider Esc and the like are gone (`ui/model/library_keymap.go`, one tagged line in `keymap.go`). Plugin key bindings are not listed, since the gate swallows them too. Tests pin the passthrough half both ways (no listed cliamp key is swallowed; every passthrough key is listed); the library's own rows are a hand-kept table beside `handleLibraryKey`. Verified live in all four contexts.
- [x] M5.2 Enrichment and YouTube robustness:
  - Migration 004 adds `tracks.enrich_failures`, with `Store.RecordEnrichFailure(track, giveUpAfter)`.
  - The Enricher tries a track twice (`enrichAttempts`), then skips it for the rest of the run. A skipped track's failure is counted when a success follows it, or at the run's end if the run read something (M5.5 review: a run that read nothing may be an outage). At 3 counted runs (`enrichGiveUpRuns`) it is marked read; a lone bad track at the end of the queue is only ever skipped. A store error ends the run uncounted.
  - `MaxFailures` (3) tracks skipped in a row end the run as an outage, and that streak is not counted. A run that skipped tracks returns an error naming the last failure, which the runtime logs.
  - `PlaylistRecords(ctx, synced)` in both YouTube modes lists a synced playlist that cannot be classified right now (empty, or every sample refused), uncached, so it keeps its tracks instead of being reconciled away.
  - `omatunes youtube signin --force` (`ytmusic.NewSessionForced`, `prompt=select_account consent` through tagged variadic options on upstream's `doOAuth`/`newInteractiveSession`) switches Google accounts; the stored token is replaced only on success.
  - `docs/omatunes/youtube.md` updated. Live: migration 004 applied to a copy of the real catalog. The forced sign-in was not run live (it needs the browser).
- [x] M5.3 Writer performance:
  - The snapshot writer prepares each statement once per transaction (`snapWriter.prepare`). Parsing, including the FTS triggers compiled into every upsert, was about 65% of write time.
  - Upserts rewrite a row, and bump its `updated_at` (never read; it now means last changed), only when a value differs (`doUpdate` renders the `DO UPDATE … WHERE … IS NOT …` guard). An unchanged upsert reads its id back.
  - Credits and playlist track lists are rewritten only when they differ, and local file rows only when size, mtime or track differ.
  - `BenchmarkSyncWrites` (Spotify-shaped: 2,000 albums, 1,000 liked, 20×300 playlist tracks): first sync 2.35 s → 0.63 s; unchanged resync 2.19 s → 0.41 s and 387 → 70 WAL pages (what is left is the generation marks the reconcile needs).
  - Real first local index (opt-in `TestIndexRealLibrary`, `OMATUNES_BENCH_MUSIC`, 26,406 files, warm cache): write 8.2 s → 4.7 s, total 10.9 s → 7.4 s, just over the 7 s target. What is left is inserts and FTS index building; a 64 MB page cache gained only 0.1 s and was not kept.
  - Per-playlist transactions were not built: the 6,000-track playlists collection now holds the writer 0.39 s (was 1.67 s), and UI reads never wait on it.
  - Test: `TestUnchangedSyncRewritesNothing` (temp triggers record every update and delete; mutation-checked). Verified live on a scratch catalog: a forced sync of YouTube, Local and Radio kept every count, with nothing in the log.
- [x] M5.4 Sync correctness:
  - `spotify.StatusError` (fork file `status_error.go`) replaces upstream's `"http status …"` errors in `webAPIWithBody` (one tagged line, same message) and in `webAPIOnce`. `unreadable` matches the code through `errors.As`.
  - Migration 005: `sync_state.last_applied_at` and `source_version`. `catalog.CollectionState` (count, newest `added_at` and its IDs, last full read, version) comes from `Store.CollectionStates` into `Known.Collections`. `catalog.CollectionHead` and `CollectionState.Matches` implement the total-plus-newest rule.
  - `spotifysrc` asks `SavedAlbumsHead`/`LikedTracksHead` (one `limit=1` request) and returns ErrUnchanged when the head matches and the last full read is under a day old (`fullReadEvery`). A failed head read falls through to the full read.
  - `localsrc.indexerVersion` (1): a stored version that differs regroups every file once from stored tags (a rule needing unstored tags must also force a reread). `Snapshot.Version` is stored on apply.
  - `TestSearchMigrationBackfills` now seeds its v1 catalog with v1 SQL, since today's writer needs v5 columns.
  - Docs: catalog.md explains the head check.
  - Live (scratch catalog): the upgrade regrouped Local once from stored tags with identical counts, and the next start was unchanged. The Spotify head check was not verified live (the account was still rate-limited); it is covered by `TestSavedCollectionsSkipUnchanged` and the real request path shared with `webAPI`.
- [x] M5.5 Offline check, docs, review, tag v0.5.0:
  - Live offline check (2026-09-29, `unshare -rn`, scratch catalog), passed: startup marks Spotify "sync failed · cached" at once. Spotify Albums, Liked Songs and Playlists, YouTube Albums, and Local Albums browse from the catalog. Search works, and a local track plays. `r` at the root fails both online sources with backoff retries. An uncached Spotify album shows the provider error with "Press Enter to retry"; its wording ("sign-in unavailable…") reads like an account problem, and is left as is.
  - Fixed from the check: the YouTube OAuth sync wrapped every silent-refresh failure in ErrNeedsAuth, so offline read as "sign in again". Now only a missing token or Google refusing the grant or client does (`signInError`).
  - README v0.5.
  - Review agents (Go correctness, quality) run; applied:
    - an outage backlog shorter than the streak no longer counts against its tracks;
    - store errors end an enrichment run;
    - `errors.Is` in `RecordEnrichFailure`;
    - an unfetched playlist listing keeps its stored track count;
    - `CollectionState.Newest` capped at 20;
    - the same-second miss of `Matches` documented;
    - `StatusError` now `Body []byte` plus `ReadErr`;
    - tests for the Spotify head request path, the forced sign-in URL, and enricher cancellation, no-success and store-failure runs;
    - comment and precedence cleanups.
  - Not taken: per-context press tests for the keymap's own rows (plan wording corrected instead).
- [ ] M6 (rename to ddmus, v0.6): planned 2026-09-29, see the M6 implementation plan. Next: M6.1.
- [ ] M7 (the queue view, v0.7): planned 2026-09-29, see the M7 implementation plan. Later candidates: InnerTube discovery of saved YouTube Music playlists; the next provider (none in use yet); omatunes-owned cross-source playlists; liking on the source (`n`).

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
- 2026-09-29: Cached album track lists follow the library: Sweep uncaches albums that left it (supersedes "cached albums kept whole" regardless of membership), so the M2.5 fill cannot grow the catalog forever.
- 2026-09-29: A local index is written exactly (blanks overwrite); provider records keep the "zero means unknown" merge.
- 2026-09-29: The "suspiciously empty snapshot" guard is the completeness check: a read must return exactly the total Spotify reports. An API that reports total 0 is trusted (you really emptied the collection).

- 2026-09-29: M3 search is catalog-only as you type; explicit rows reach live Spotify search and the radio directory.
- 2026-09-29: Radio favorites and radios.toml stations move into the catalog (provider `radio`) for search; favorites rank first. This answers the open question below.
- 2026-09-29: Cross-source duplicates stay separate, labelled by source; merging is deferred.
- 2026-09-29: Enter on a searched track plays its album from that track (just the track when offline or album-less).

- 2026-09-29: Albums sort by title by default, with `o` for artist order; names sort literally ("The Planets" under T). Radio favorites A–Z; directory stations stay ranked by votes; station rows are unnumbered.

- 2026-09-29: M4 is YouTube Music. Sign-in keeps cliamp's two modes (cookies_from, or an own OAuth client, which wins when both are set). The menu shows Playlists and Liked Music; YouTube syncs every 2 hours by default (youtube_refresh). Provider menus become capability-driven.

- 2026-09-29: YouTube's album/artist gap is the official Data API's (it models YouTube videos and playlists, not the YouTube Music library). The private InnerTube API has the library but is unofficial and cookie-authenticated; M4.0 first checks whether yt-dlp reaches library albums/artists with cookies before any InnerTube client is considered.

- 2026-09-29: After the M4.0 spike: cookie mode classifies playlists by sampled video category (like OAuth); Watch later and Liked videos are never listed; YouTube tracks are enriched in the background with artist/album/year, giving YouTube Albums and Artists. The cookie path is built first.

- 2026-09-29: With both YouTube sign-ins, the sync reads Liked Music through OAuth and playlists through cookies (the API lists none of this account's playlists and never saved ones); this departs from cliamp's "OAuth wins" for the sync only. cliamp keeps one classification scope in its cache file, so switching modes reclassifies once.

- 2026-09-29: Playlists saved from others in YouTube Music are configured by link (`youtube_playlists`), since no readable list includes them; InnerTube discovery may come later as its own slice.

- 2026-09-29: The unified Library node is renamed "All Music", so it does not read as a second Local.

- 2026-09-29: The YouTube sync classifies playlists more strictly than cliamp: a failed read fails the playlists collection (the sync keeps what it has) and is never cached as non-music, and an empty playlist stays unknown. The OAuth sync uses its own `classifyForSync`; cliamp's `classifyPlaylists` is unchanged. With both sign-ins, Liked Music falls back to cookies when OAuth needs signing in or refuses (quota).

- 2026-09-29: OAuth sign-in happens outside the TUI with `omatunes youtube signin`; the sync itself never signs in interactively.

- 2026-09-29: M5 is deferred cleanup (v0.5), not a new provider: no Plex/Jellyfin/Navidrome server is in use to test against.

- 2026-09-29: The fork is renamed: DaphnisDuck's Music Player, `ddmus` for short (omatunes is another player's name). M6 does the rename before the queue work; the Go module path stays cliamp's.

- 2026-09-29: M7 is the queue view (planned as M6, renumbered when the rename came first): every view lists all its live keys at the bottom instead of the `?`/`Ctrl+K` overlay; `n` Favorite is removed; SRC shows the queue's source; the settings panel is display-only, driven by direct keys (Tab stays the library/queue toggle).

- 2026-09-29: A snapshot stays one transaction per collection. After M5.3 the largest measured one (6,000 playlist tracks) takes 0.39 s, so per-playlist transactions (M2.2 deferral) are dropped.

## Open questions
- Whether `music_dir` should split from `initial_directory` (the Local scan folder vs the file browser's start folder). Default: keep reusing `initial_directory` until someone needs them apart.
- Artwork: cache album art images locally for offline display, or store URLs only (M2 stores URLs only).
