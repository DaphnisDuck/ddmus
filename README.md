# DaphnisDuck's Music Player

**Version 0.5**

DaphnisDuck's Music Player (`ddmus`) is a retro terminal music player built around your library rather than around providers. It is a fork of [cliamp](https://github.com/bjarneo/cliamp) by Bjarne Øverli, made with love and a lot of respect for the original.

cliamp is a wonderful Winamp-inspired player: spectrum visualizer, parametric EQ, Lua plugins, and support for an impressive list of streaming services and media servers. ddmus keeps all of that playback machinery and changes how you *find* your music.

## What we're building

You shouldn't have to remember where your music lives. ddmus organises everything as one hierarchy:

```
Music
├── All Music      Albums · Artists (every source together)
├── Spotify        Albums · Artists · Playlists · Liked Songs
├── YouTube Music  Albums · Artists · Playlists · Liked Music
├── Local          Albums · Artists · Genres · Folders · Playlists
├── Radio          Favorites · Browse Stations
└── Search
```

The application owns navigation; providers own content and playback. The roadmap:

1. **Library navigation** (v0.1): the hierarchy above for Spotify, local files and internet radio, with vim-style keys, sitting above cliamp's existing now-playing, queue, EQ and visualizer.
2. **Persistent catalog** (v0.2): a local SQLite catalog of your Spotify library and music folder that syncs in the background, so browsing a large library is instant and works offline.
3. **Unified search** (v0.3): search-as-you-type across Spotify, local files and your radio stations at once, offline, plus an All Music menu that lists every source's albums and artists together.
4. **More providers**, one per release: **YouTube Music** (v0.4) syncs your playlists and Liked Music, with albums and artists read in the background; Plex, Jellyfin, Navidrome and cliamp's other providers can follow.

v0.5 is a cleanup release: faster, quieter syncs, sturdier YouTube enrichment, and a keymap that shows only the keys that work.

The full plan lives in [plan.md](plan.md).

## Status

v0.5 is an early release for people comfortable building from source.

- **In the Music hierarchy:** Spotify, YouTube Music, local files and internet radio.
- **YouTube Music:** your music playlists and Liked Music sync into the catalog (browser cookies or your own Google OAuth client, as in cliamp), and each track's artist, album and year is read in the background, giving YouTube Albums and Artists too. See [docs/ddmus/youtube.md](docs/ddmus/youtube.md).
- **Catalog:** Spotify and Local browsing read from a local catalog, so lists open instantly and work offline. Spotify syncs in the background, and album track lists are cached as you open albums and, gradually, for the rest of your saved albums. A sync writes only what changed, and skips rereading your saved albums and liked songs when their count and newest item are unchanged. The local music folder is re-indexed at every start, rereading only changed files. See [docs/ddmus/catalog.md](docs/ddmus/catalog.md).
- **Search:** `/` from anywhere searches the whole catalog as you type, with operators like `artist:`, `album:` and `source:local`. Enter on a track plays its album from that track, and rows at the end run a source's own live search or a radio-directory search. See [docs/ddmus/search.md](docs/ddmus/search.md).
- **All Music:** Music → All Music lists albums and artists from Spotify, YouTube Music and Local together, each labelled with its source. Albums sort by title; `o` sorts them by artist.
- **Hidden for now:** cliamp's other providers (podcasts, non-music YouTube, SoundCloud, Mixcloud, Navidrome, Plex, Jellyfin, Emby, Qobuz, Tidal, and more) are still in the code but have no entry in the menu yet. They come back one per release.
- **Disabled keys:** most of cliamp's jump keys (provider switching, theme and visualizer pickers, file browser and similar) are turned off while the new navigation settles. `?` or `Ctrl+K` lists the keys that work where you are. See [docs/ddmus/navigation.md](docs/ddmus/navigation.md).
- **Runs alongside cliamp:** ddmus keeps its own config, data and media-key (MPRIS) name, so you can install both. See [docs/ddmus/files.md](docs/ddmus/files.md).

## Build and install

**Prerequisites**

- [Go](https://go.dev/dl/) 1.26.6 or later
- On Linux, ALSA and codec development headers:

```sh
# Debian/Ubuntu
sudo apt install libasound2-dev libflac-dev libvorbis-dev libogg-dev libmpg123-dev
# Fedora
sudo dnf install alsa-lib-devel flac-devel libvorbis-devel libogg-devel mpg123-devel
# Arch
sudo pacman -S alsa-lib flac libvorbis libogg mpg123
```

- On macOS: `brew install flac libvorbis libogg mpg123 pkg-config`

**Build**

```sh
git clone https://github.com/DaphnisDuck/ddmus.git
cd ddmus
make && make install   # builds ./ddmus and installs it to ~/.local/bin/ddmus
```

Without Make: `go build -o ddmus .`

**Optional runtime dependencies**

- [ffmpeg](https://ffmpeg.org/) for AAC, ALAC, Opus and WMA playback
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) for YouTube Music

Windows builds follow cliamp's instructions, which need MSYS2 and CGO for Spotify; see cliamp's README. They are untested for ddmus.

## Quick start

```sh
ddmus                          # open the Music library
ddmus ~/Music/some-album       # load a directory into the queue (Tab shows it)
```

| Key | Action |
|---|---|
| `j` `k` | Move down / up |
| `l` `Enter` | Open, or play |
| `h` `Esc` | Back |
| `g` `G` | Top / bottom |
| `/` | Search everything (see [docs/ddmus/search.md](docs/ddmus/search.md)) |
| `o` | In an Albums list, sort by title or by artist |
| `r` | Sync the source you're browsing now |
| `Space` | Play / pause |
| `Tab` | Switch between the library and the queue |
| `q` | Back; quits at Music |
| `Ctrl+K` | All keybindings |

Selecting a track replaces the queue with its album or playlist and starts playing there. The full key list is in [docs/ddmus/navigation.md](docs/ddmus/navigation.md).

## Configuration

ddmus reads `~/.config/ddmus/config.toml`. Set `DDMUS_CONFIG_DIR` to use another directory. Coming from omatunes, this player's name before v0.6? Move your folders once as shown in [docs/ddmus/files.md](docs/ddmus/files.md#moving-from-omatunes-before-v06). It starts empty, so to carry over your cliamp settings, copy them once:

```sh
rsync -a --exclude='*.log' --exclude='*.sock' ~/.config/cliamp/ ~/.config/ddmus/
```

- **Spotify:** run `ddmus setup` and choose Spotify, or follow [docs/spotify.md](docs/spotify.md), reading `~/.config/ddmus` wherever it says `~/.config/cliamp`. A Spotify Premium account is required. The first time you open Spotify in the library, press `Enter` to sign in. Your library then syncs in the background: at startup when the last sync is older than 30 minutes, and whenever you press `r`. To change the interval:

  ```toml
  [ddmus]
  spotify_refresh = "2h"   # "0s" syncs at every start
  ```

- **Local music:** Local → Albums, Artists and Genres come from an index of a single directory, updated at startup (only changed files are reread). It is `initial_directory` in `config.toml`, else `$XDG_MUSIC_DIR`, else `~/Music`:

  ```toml
  initial_directory = "~/Music"
  ```

- **Catalog:** the catalog lives in `~/.local/share/ddmus/library.db` and holds no passwords or tokens. Delete it (with its `-wal` and `-shm` files) to rebuild it from scratch at the next start.
- **Radio:** Radio → Browse Stations covers the [Radio Browser](https://www.radio-browser.info/) directory (about 58,000 stations) by country or tag, plus the cliamp radio channels. Add your own stations in `~/.config/ddmus/radios.toml`; see [docs/configuration.md](docs/configuration.md#custom-radio-stations).

cliamp's other documentation in [docs/](docs/) still describes the engine, EQ, themes, plugins and configuration keys accurately. Its keybinding and provider-pane sections describe cliamp's interface rather than ddmus'.

> **Updating:** `ddmus upgrade` is disabled, because cliamp's self-updater would install cliamp over ddmus. Update by pulling and rebuilding: `git pull && make install`.

## Troubleshooting

**No audio output**

"audio output unavailable" means the ALSA backend cannot reach your sound server. Install the bridge package:

- **PipeWire:** `pipewire-alsa`
- **PulseAudio:** `pulseaudio-alsa` (`libasound2-plugins` on Debian/Ubuntu, including WSL2)

On WSL2, also see [WSL2 setup](docs/configuration.md#wsl2-windows-subsystem-for-linux).

## Staying close to upstream

ddmus tracks cliamp and regularly merges its improvements. Fork changes live in new files where possible, and every edit to an upstream file is marked `// ddmus:`. See [docs/ddmus/upstream.md](docs/ddmus/upstream.md).

## Thanks

ddmus would not exist without **[Bjarne Øverli](https://github.com/bjarneo)** ([x.com/iamdothash](https://x.com/iamdothash)), who created cliamp, and everyone who has [contributed to it](https://github.com/bjarneo/cliamp/graphs/contributors). Nearly everything that makes ddmus sound good is their work: the audio engine, the EQ, the visualizers, the provider integrations and the plugin system. If you enjoy ddmus, please go and star, use and support [cliamp](https://github.com/bjarneo/cliamp) and visit [cliamp.stream](https://cliamp.stream).

cliamp in turn builds on [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), [Beep](https://github.com/gopxl/beep) and [go-librespot](https://github.com/devgianlu/go-librespot). ddmus' catalog uses [modernc.org/sqlite](https://gitlab.com/cznic/sqlite).

## License

MIT, the same as cliamp. See [LICENSE](LICENSE). The original copyright belongs to Bjarne Øverli.

## Disclaimer

Use this software at your own risk. The authors are not responsible for damage or issues that result from its use.
