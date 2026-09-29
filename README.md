# Omatunes

**Version 0.2**

Omatunes is a retro terminal music player built around your library rather than around providers. It is a fork of [cliamp](https://github.com/bjarneo/cliamp) by Bjarne Øverli, made with love and a lot of respect for the original.

cliamp is a wonderful Winamp-inspired player: spectrum visualizer, parametric EQ, Lua plugins, and support for an impressive list of streaming services and media servers. Omatunes keeps all of that playback machinery and changes how you *find* your music.

## What we're building

You shouldn't have to remember where your music lives. Omatunes organises everything as one hierarchy:

```
Music
├── Spotify   Albums · Artists · Playlists · Liked Songs
├── Local     Albums · Artists · Genres · Folders · Playlists
├── Radio     Favorites · Browse Stations
└── Search
```

The application owns navigation; providers own content and playback. The roadmap:

1. **Library navigation** (v0.1): the hierarchy above for Spotify, local files and internet radio, with vim-style keys, sitting above cliamp's existing now-playing, queue, EQ and visualizer.
2. **Persistent catalog** (v0.2, this release): a local SQLite catalog of your Spotify library and music folder that syncs in the background, so browsing a large library is instant and works offline.
3. **Unified search**: fast full-text search across every source at once.
4. **More providers**: bring cliamp's other providers (YouTube Music, Plex, Jellyfin, Navidrome, …) into the catalog.

The full plan lives in [plan.md](plan.md).

## Status

v0.2 is an early release for people comfortable building from source.

- **In the Music hierarchy:** Spotify, local files and internet radio.
- **Catalog:** Spotify and Local browsing read from a local catalog, so lists open instantly and work offline. Spotify syncs in the background, and album track lists are cached as you open albums and, gradually, for the rest of your saved albums. The local music folder is re-indexed at every start, rereading only changed files. See [docs/omatunes/catalog.md](docs/omatunes/catalog.md).
- **Search** still opens cliamp's provider search; unified search across the catalog is milestone 3.
- **Hidden for now:** cliamp's other providers (podcasts, YouTube, SoundCloud, Mixcloud, Navidrome, Plex, Jellyfin, Emby, Qobuz, Tidal, and more) are still in the code but have no entry in the menu yet. They come back in milestone 4.
- **Disabled keys:** most of cliamp's jump keys (provider switching, theme and visualizer pickers, file browser and similar) are turned off while the new navigation settles. See [docs/omatunes/navigation.md](docs/omatunes/navigation.md).
- **Runs alongside cliamp:** Omatunes keeps its own config, data and media-key (MPRIS) name, so you can install both. See [docs/omatunes/files.md](docs/omatunes/files.md).

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
git clone https://github.com/DaphnisDuck/omatunes.git
cd omatunes
make && make install   # builds ./omatunes and installs it to ~/.local/bin/omatunes
```

Without Make: `go build -o omatunes .`

**Optional runtime dependencies**

- [ffmpeg](https://ffmpeg.org/) for AAC, ALAC, Opus and WMA playback
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) for the YouTube-family providers (not yet in the Omatunes menu)

Windows builds follow cliamp's instructions, which need MSYS2 and CGO for Spotify; see cliamp's README. They are untested for Omatunes.

## Quick start

```sh
omatunes                          # open the Music library
omatunes ~/Music/some-album       # load a directory into the queue (Tab shows it)
```

| Key | Action |
|---|---|
| `j` `k` | Move down / up |
| `l` `Enter` | Open, or play |
| `h` `Esc` | Back |
| `g` `G` | Top / bottom |
| `/` | Search |
| `r` | Sync the source you're browsing now |
| `Space` | Play / pause |
| `Tab` | Switch between the library and the queue |
| `q` | Back; quits at Music |
| `Ctrl+K` | All keybindings |

Selecting a track replaces the queue with its album or playlist and starts playing there. The full key list is in [docs/omatunes/navigation.md](docs/omatunes/navigation.md).

## Configuration

Omatunes reads `~/.config/omatunes/config.toml`. Set `OMATUNES_CONFIG_DIR` to use another directory. It starts empty, so to carry over your cliamp settings, copy them once:

```sh
rsync -a --exclude='*.log' --exclude='*.sock' ~/.config/cliamp/ ~/.config/omatunes/
```

- **Spotify:** run `omatunes setup` and choose Spotify, or follow [docs/spotify.md](docs/spotify.md), reading `~/.config/omatunes` wherever it says `~/.config/cliamp`. A Spotify Premium account is required. The first time you open Spotify in the library, press `Enter` to sign in. Your library then syncs in the background: at startup when the last sync is older than 30 minutes, and whenever you press `r`. To change the interval:

  ```toml
  [omatunes]
  spotify_refresh = "2h"   # "0s" syncs at every start
  ```

- **Local music:** Local → Albums, Artists and Genres come from an index of a single directory, updated at startup (only changed files are reread). It is `initial_directory` in `config.toml`, else `$XDG_MUSIC_DIR`, else `~/Music`:

  ```toml
  initial_directory = "~/Music"
  ```

- **Catalog:** the catalog lives in `~/.local/share/omatunes/library.db` and holds no passwords or tokens. Delete it (with its `-wal` and `-shm` files) to rebuild it from scratch at the next start.
- **Radio:** Radio → Browse Stations covers the [Radio Browser](https://www.radio-browser.info/) directory (about 58,000 stations) by country or tag, plus the cliamp radio channels. Add your own stations in `~/.config/omatunes/radios.toml`; see [docs/configuration.md](docs/configuration.md#custom-radio-stations).

cliamp's other documentation in [docs/](docs/) still describes the engine, EQ, themes, plugins and configuration keys accurately. Its keybinding and provider-pane sections describe cliamp's interface rather than Omatunes'.

> **Updating:** `omatunes upgrade` is disabled, because cliamp's self-updater would install cliamp over Omatunes. Update by pulling and rebuilding: `git pull && make install`.

## Troubleshooting

**No audio output**

"audio output unavailable" means the ALSA backend cannot reach your sound server. Install the bridge package:

- **PipeWire:** `pipewire-alsa`
- **PulseAudio:** `pulseaudio-alsa` (`libasound2-plugins` on Debian/Ubuntu, including WSL2)

On WSL2, also see [WSL2 setup](docs/configuration.md#wsl2-windows-subsystem-for-linux).

## Staying close to upstream

Omatunes tracks cliamp and regularly merges its improvements. Fork changes live in new files where possible, and every edit to an upstream file is marked `// omatunes:`. See [docs/omatunes/upstream.md](docs/omatunes/upstream.md).

## Thanks

Omatunes would not exist without **[Bjarne Øverli](https://github.com/bjarneo)** ([x.com/iamdothash](https://x.com/iamdothash)), who created cliamp, and everyone who has [contributed to it](https://github.com/bjarneo/cliamp/graphs/contributors). Nearly everything that makes Omatunes sound good is their work: the audio engine, the EQ, the visualizers, the provider integrations and the plugin system. If you enjoy Omatunes, please go and star, use and support [cliamp](https://github.com/bjarneo/cliamp) and visit [cliamp.stream](https://cliamp.stream).

cliamp in turn builds on [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), [Beep](https://github.com/gopxl/beep) and [go-librespot](https://github.com/devgianlu/go-librespot). Omatunes' catalog uses [modernc.org/sqlite](https://gitlab.com/cznic/sqlite).

## License

MIT, the same as cliamp. See [LICENSE](LICENSE). The original copyright belongs to Bjarne Øverli.

## Disclaimer

Use this software at your own risk. The authors are not responsible for damage or issues that result from its use.
