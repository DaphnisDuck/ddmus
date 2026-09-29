# Library navigation

omatunes opens on **Music**, a library hierarchy that sits above cliamp's playback chrome. Now playing, progress, volume, EQ and the visualizer stay where they were.

```
Music
├── Spotify   Albums · Artists · Playlists · Liked Songs
├── Local     Albums · Artists · Genres · Folders · Playlists
├── Radio     Favorites · Browse Stations (cliamp radio · Countries · Genres & Tags)
└── Search
```

- **Spotify:**
  - **Albums** are your saved albums.
  - **Artists** are the artists you follow; each one opens their albums and singles.
  - **Playlists** are the ones you own or follow.
  - **Liked Songs** is your saved tracks.
- **Local:**
  - **Albums**, **Artists** and **Genres** come from an index of your music directory: `initial_directory` from config, else `$XDG_MUSIC_DIR`, else `~/Music`. Files are grouped into albums by album tag within a folder (per-disc folders like `CD1` stay one album); an album with several track artists is credited to Various Artists and listed under each of them.
  - **Folders** opens the file browser.
  - **Playlists** are your saved local playlists.
- **Radio:**
  - **Favorites** are your starred stations. Enter plays one.
  - **Browse Stations** lists the cliamp radio channels and the station directory by country or tag.
- **Search:** until unified search (Milestone 3) lands, this opens the existing provider search (Spotify if configured, else Local).

Enter on a track replaces the queue with the list it's in and starts at that track, the way an album plays. The library stays on screen.

## The catalog and sync

Spotify and Local browsing read from a local catalog (`~/.local/share/omatunes/library.db`), so lists open instantly and work offline.

- **When Spotify syncs:** at startup, if the last successful sync is more than 30 minutes old, and whenever you press `r`. The sync runs in the background while you browse. A failed sync retries on its own after 1 minute, then 2, 4, and so on up to every 30 minutes.
- **Album tracks:** opening a Spotify album whose tracks aren't cached fetches and caches them, so it opens offline from then on. In the background, omatunes caches the rest of your saved albums one at a time, newest saved first. It pauses while you open an album or a sync runs, and backs off when Spotify rate-limits it.
- **When Local indexes:** at every startup and whenever you press `r` in Local. Only files whose size or modification time changed have their tags read again, so an index with nothing new takes about a second. If the music directory is missing, or is empty while the index holds files (an unmounted drive), the index is kept as it is and the header reports the failure. Files under a folder omatunes can't read are kept too.
- **`r`:** syncs the source you're browsing; at the Music root it syncs every source.
- **Status:** the header shows `↻ syncing`, `✓ synced 2m ago`, or `sync failed · cached` when the last attempt failed and you're seeing the cached library. With both Spotify and Local, each status is prefixed with its source.
- **Updates:** screens refresh in place when a sync lands, keeping the cursor on the same item.
- **What stays live:**
  - a playlist Spotify won't let the sync read
  - an artist's full discography; offline, you get the albums the catalog knows instead

## Keys

| Key | Action |
|---|---|
| `j` `k` / `↓` `↑` | Move (wraps) |
| `g` `G` / `Home` `End` | Top / bottom |
| `Ctrl+D` `Ctrl+U` / `PgDn` `PgUp` | Page down / up |
| `l` `Enter` `→` | Open, or play |
| `h` `Esc` `Backspace` `←` | Back |
| `q` | Back; quits at Music |
| `/` | Search |
| `r` | Sync the source you're browsing now (everything, at Music) |
| `Space` | Play / pause |
| `Tab` | Show the queue; `Tab`, `Esc` or `b` returns to the library |
| `s` `<` `>` `,` `.` `+` `-` `Shift+←` `Shift+→` | Stop, previous/next, volume, seek |
| `?` `Ctrl+K` | Keymap |

On an error screen, Enter retries. When Spotify needs you to sign in, Enter starts sign-in in your browser and reloads once it finishes.

In the queue view, list navigation, Enter, `/` (filter the queue) and the transport keys work as in cliamp.

## Disabled cliamp keys

Keys that jump to other parts of cliamp are disabled, including provider switching (`S`, `R`, `L`, `N`, …), `o`, `u`, `p`, `t`, `v`, `e`, `y` and `w`. They are swallowed by an allowlist in `ui/model/library_nav.go` (`libraryPassthroughKeys`, `queuePassthroughKeys`). To bring one back, add it there.
