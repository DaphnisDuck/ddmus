# Library navigation

omatunes opens on **Music**, a library hierarchy that sits above cliamp's playback chrome. Now playing, progress, volume, EQ and the visualizer stay where they were.

```
Music
├── All Music      Albums · Artists (every source together)
├── Spotify        Albums · Artists · Playlists · Liked Songs
├── YouTube Music  Albums · Artists · Playlists · Liked Music
├── Local          Albums · Artists · Genres · Folders · Playlists
├── Radio          Favorites · Browse Stations (cliamp radio · Countries · Genres & Tags)
└── Search
```

- **All Music:** Albums and Artists from Spotify, YouTube Music and Local in one list each, every row labelled with its source. The same album or artist in both stays two rows, side by side. Opening a row goes to that source's own album or artist.
- **Spotify:**
  - **Albums** are your saved albums.
  - **Artists** are the artists you follow; each one opens their albums and singles.
  - **Playlists** are the ones you own or follow.
  - **Liked Songs** is your saved tracks.
- **YouTube Music:** your music playlists, Liked Music, and the albums and artists read from those tracks in the background. See [youtube.md](youtube.md).
- **Local:**
  - **Albums**, **Artists** and **Genres** come from an index of your music directory: `initial_directory` from config, else `$XDG_MUSIC_DIR`, else `~/Music`. Files are grouped into albums by album tag within a folder (per-disc folders like `CD1` stay one album); an album with several track artists is credited to Various Artists and listed under each of them.
  - **Folders** opens the file browser.
  - **Playlists** are your saved local playlists.
- **Radio:**
  - **Favorites** are your starred stations. Enter plays one.
  - **Browse Stations** lists the cliamp radio channels and the station directory by country or tag.
- **Search:** searches everything the catalog holds as you type; `/` opens it from anywhere. See [search.md](search.md). Without a catalog it is the provider's own search (Spotify if configured, else Local).

Lists sort A–Z as names are written ("The Planets" under T), ignoring case, accents and leading punctuation. Albums sort by title; press `o` to sort by artist instead. Radio favorites sort by name; stations under a country or tag keep the directory's most-voted-first order.

Enter on a track replaces the queue with the list it's in and starts at that track, the way an album plays. The library stays on screen.

## The catalog and sync

Spotify and Local browsing read from a local catalog (`~/.local/share/omatunes/library.db`), so lists open instantly and work offline. Spotify syncs at startup when its last sync is older than 30 minutes, Local re-indexes changed files at every startup, and `r` syncs the source you're browsing. The header shows each source's status (`↻ syncing`, `✓ synced 2m ago`, `sync failed · cached`). See [catalog.md](catalog.md) for what's cached, failure safety, and the `[omatunes]` settings.

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
| `o` | In an Albums list, switch between sorting by title and by artist |
| `Space` | Play / pause |
| `Tab` | Show the queue; `Tab`, `Esc` or `b` returns to the library |
| `s` `<` `>` `,` `.` `+` `-` `Shift+←` `Shift+→` | Stop, previous/next, volume, seek |
| `?` `Ctrl+K` | Keymap |

On an error screen, Enter retries. When Spotify needs you to sign in, Enter starts sign-in in your browser and reloads once it finishes.

In the queue view, list navigation, Enter, `/` (filter the queue) and the transport keys work as in cliamp.

## Disabled cliamp keys

Keys that jump to other parts of cliamp are disabled, including provider switching (`S`, `R`, `L`, `N`, …), `o`, `u`, `p`, `t`, `v`, `e`, `y` and `w`. They are swallowed by an allowlist in `ui/model/library_nav.go` (`libraryPassthroughKeys`, `queuePassthroughKeys`). To bring one back, add it there.

The keymap (`?` or `Ctrl+K`) follows the same allowlist: in the library, the queue and search it lists only the keys that work there (the library's own keys, then the player keys passed through to cliamp), built in `ui/model/library_keymap.go`. A key brought back through the allowlist shows up in the keymap by itself when cliamp's command registry describes it.
