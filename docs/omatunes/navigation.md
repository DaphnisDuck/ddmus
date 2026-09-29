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
  - **Albums**, **Artists** and **Genres** come from a tag scan of your music directory: `initial_directory` from config, else `$XDG_MUSIC_DIR`, else `~/Music`. The scan runs the first time you open one of them and is cached for the session.
  - **Folders** opens the file browser.
  - **Playlists** are your saved local playlists.
- **Radio:**
  - **Favorites** are your starred stations. Enter plays one.
  - **Browse Stations** lists the cliamp radio channels and the station directory by country or tag.
- **Search:** until unified search (Milestone 3) lands, this opens the existing provider search (Spotify if configured, else Local).

Enter on a track replaces the queue with the list it's in and starts at that track, the way an album plays. The library stays on screen.

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
| `Space` | Play / pause |
| `Tab` | Show the queue; `Tab`, `Esc` or `b` returns to the library |
| `s` `<` `>` `,` `.` `+` `-` `Shift+←` `Shift+→` | Stop, previous/next, volume, seek |
| `?` `Ctrl+K` | Keymap |

On an error screen, Enter retries. When Spotify needs you to sign in, Enter starts sign-in in your browser and reloads once it finishes.

In the queue view, list navigation, Enter, `/` (filter the queue) and the transport keys work as in cliamp.

## Disabled cliamp keys

Keys that jump to other parts of cliamp are disabled, including provider switching (`S`, `R`, `L`, `N`, …), `o`, `u`, `p`, `t`, `v`, `e`, `y` and `w`. They are swallowed by an allowlist in `ui/model/library_nav.go` (`libraryPassthroughKeys`, `queuePassthroughKeys`). To bring one back, add it there.
