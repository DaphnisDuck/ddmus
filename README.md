# ddmus

**DaphnisDuck's Music Player** is a terminal music player built around your library. Your Spotify library, your YouTube Music playlists, the music on your disk and internet radio sit in one hierarchy that browses instantly, searches as you type and keeps working offline.

ddmus is a fork of [cliamp](https://github.com/bjarneo/cliamp), the Winamp-inspired terminal player by Bjarne Øverli. It keeps cliamp's audio engine, equalizer and visualizers, and changes how you find your music.

```
Music
├── All Music      Albums · Artists (every source together)
├── Spotify        Albums · Artists · Playlists · Liked Songs
├── YouTube Music  Albums · Artists · Playlists · Liked Music
├── Local          Albums · Artists · Genres · Folders · Playlists
├── Radio          Favorites · Browse Stations
└── Search
```

## Why ddmus

- **One library, not a list of services.** You browse albums and artists; where a track comes from is a label on the row.
- **Instant and offline.** Your libraries are kept in a local catalog and synced in the background, so lists open at once and browsing and search work without a network. ([catalog](docs/ddmus/catalog.md))
- **Search everything as you type.** `/` searches every source at once, with operators such as `artist:`, `album:` and `source:local`. ([search](docs/ddmus/search.md))
- **Keyboard all the way.** Vim-style movement, consistent navigation keys, and a bar at the bottom that lists exactly the keys that work where you are. ([navigation](docs/ddmus/navigation.md))
- **A real player underneath.** Gapless playback, a 10-band equalizer with presets, spectrum visualizers, themes, lyrics, playback speed, shuffle and repeat, all inherited from cliamp.
- **Fits your desktop.** Media keys and desktop widgets work through MPRIS, and `ddmus pause`, `ddmus next`, `ddmus status` and friends control the running player from a script or a keybinding.
- **Album artwork** beside a track's details, in Kitty and Ghostty. ([artwork](docs/ddmus/artwork.md))

## What it plays

| Source | What you get | What it needs |
| --- | --- | --- |
| **Spotify** | Saved albums, followed artists, playlists, Liked Songs, and Spotify's own search | A Spotify Premium account |
| **YouTube Music** | Your playlists and Liked Music, with albums and artists read from their tracks | [yt-dlp](https://github.com/yt-dlp/yt-dlp), and a browser you are signed in to or your own Google OAuth client |
| **Local files** | Albums, artists and genres from the tags in your music folder, a folder browser, and saved playlists | Nothing; [ffmpeg](https://ffmpeg.org/) for AAC, ALAC, Opus and WMA files |
| **Internet radio** | Your favorite stations, the [Radio Browser](https://www.radio-browser.info/) directory by country or tag, cliamp's radio channels, and stations you add yourself | Nothing |

Local files and radio need no account at all. cliamp's other sources (Navidrome, Plex, Jellyfin, SoundCloud, podcasts and more) and its Lua plugins are not part of ddmus 1.0.

## Requirements

- **Linux on x86-64.** ddmus is developed and used on Arch, and built and tested on Ubuntu in CI. macOS builds and passes its tests there but is untested in use; Windows is not supported.
- **Sound** through ALSA. On PipeWire or PulseAudio, install `pipewire-alsa` or `pulseaudio-alsa` (`libasound2-plugins` on Debian).
- **Optional:** `ffmpeg` and `yt-dlp`, as in the table above.
- **Terminal:** any modern terminal. Artwork needs Kitty or Ghostty; everywhere else, and inside tmux, ddmus works the same without it.

## Install

**From source**

You need [Go](https://go.dev/dl/) 1.26.6 or later (ddmus is built and tested with 1.27.1), a C compiler, `pkg-config`, and the ALSA and codec headers:

```sh
# Debian/Ubuntu
sudo apt install build-essential pkg-config libasound2-dev libflac-dev libvorbis-dev libogg-dev libmpg123-dev
# Arch
sudo pacman -S base-devel alsa-lib flac libvorbis libogg mpg123
```

```sh
git clone https://github.com/DaphnisDuck/ddmus.git
cd ddmus
make install            # builds ./ddmus and installs it to ~/.local/bin/ddmus
```

To update, `git pull && make install`. ddmus does not update itself.

**Release builds**

Published releases are on the [Releases](https://github.com/DaphnisDuck/ddmus/releases) page; release candidates are marked "Pre-release" there. If the page lists none, none has been published yet, and source is the way to install. Each release has a build for Linux x86-64 (glibc 2.36 or newer; it needs only ALSA and glibc from your system) and a `SHA256SUMS` file:

```sh
sha256sum -c --ignore-missing SHA256SUMS
tar -xzf ddmus-*-linux-amd64.tar.gz
install -Dm755 ddmus-*-linux-amd64/ddmus ~/.local/bin/ddmus
```

An AUR package, `ddmus-bin`, is planned for the stable 1.0 release; it is not in the AUR yet.

## Quick start

```sh
ddmus setup             # Spotify, YouTube Music and your music folder; each is optional
ddmus                   # open the library
```

With nothing set up, ddmus still opens with Local (your `~/Music` folder) and Radio. Opening Spotify for the first time asks you to press `Enter` and sign in in your browser.

In the library:

| Key | Action |
| --- | --- |
| `j` `k` | Move down, up |
| `l` `Enter` | Open, or play |
| `h` `Esc` | Back (never quits) |
| `g` `G` | Top, bottom |
| `/` | Search everything |
| `Space` | Play or pause |
| `p` `n` | Previous, next track |
| `Tab` | Switch between the library and the queue |
| `r` | Sync the source you're browsing |
| `q` | Quit, from any screen |

Choosing a track plays its album or playlist from that track. `Tab` shows the queue, where you can shuffle, repeat, reorder, open track info (`i`) and lyrics (`y`), and change the equalizer, visualizer and speed. In the queue, Enter plays the selected track, `/` filters, and `r` cycles repeat. Where you are typing (search, the queue filter), letters such as `q` are text. Every screen lists its keys at the bottom; the complete tables are in [docs/ddmus/navigation.md](docs/ddmus/navigation.md).

You can also start with files: `ddmus ~/Music/some-album` loads a folder into the queue.

## Configuration

ddmus reads `~/.config/ddmus/config.toml`. `ddmus setup` writes the parts it asks about and leaves the rest of the file alone.

- **Spotify:** `ddmus setup` offers a choice between your own Spotify Developer app, which is recommended, and a shared client that is rate-limited more often. [docs/spotify.md](docs/spotify.md) shows how to create the app. `ddmus spotify reset` signs you out.
- **YouTube Music:** browser cookies or your own Google OAuth client; see [docs/ddmus/youtube.md](docs/ddmus/youtube.md).
- **Local music:** one folder is indexed: `initial_directory` in `config.toml`, else `$XDG_MUSIC_DIR`, else `~/Music`. Only changed files are reread at each start.
- **Radio:** add your own stations in `~/.config/ddmus/radios.toml`; see [Custom Radio Stations](docs/configuration.md#custom-radio-stations).
- **How often sources sync, artwork and the border:**

  ```toml
  [ddmus]
  # how old the last sync may get before the next start syncs again; "0s" is every start
  spotify_refresh = "2h"
  youtube_refresh = "2h"
  artwork = true
  border = true
  ```

  A comment needs a line of its own: ddmus does not read one that follows a value.

## Documentation

| Topic | Where |
| --- | --- |
| Navigation and every key | [docs/ddmus/navigation.md](docs/ddmus/navigation.md) |
| Search and its query language | [docs/ddmus/search.md](docs/ddmus/search.md) |
| The catalog: what is stored, when it syncs, Spotify rate limits | [docs/ddmus/catalog.md](docs/ddmus/catalog.md) |
| YouTube Music setup | [docs/ddmus/youtube.md](docs/ddmus/youtube.md) |
| Album artwork and terminals | [docs/ddmus/artwork.md](docs/ddmus/artwork.md) |
| How ddmus uses the window | [docs/ddmus/layout.md](docs/ddmus/layout.md) |
| Files and folders, running beside cliamp, moving from omatunes | [docs/ddmus/files.md](docs/ddmus/files.md) |
| Release history | [CHANGELOG.md](CHANGELOG.md) |
| Reporting a security problem | [SECURITY.md](SECURITY.md) |

`ddmus --help` lists every command and option.

The other files in [docs/](docs/) are cliamp's own documentation, not yet adapted to ddmus. Read `ddmus` and `~/.config/ddmus` where they say `cliamp` and `~/.config/cliamp`. Some still apply in part:

- [Audio settings](docs/audio-quality.md).
- [Themes](docs/themes.md): the theme files and settings apply. Choose a theme with `--start-theme` or `ddmus theme`; the `t` picker described there is not in ddmus.
- [Local playlists](docs/playlists.md): the file format and the `ddmus playlist` commands apply. The playlist-manager keys described there do not.
- [Remote control](docs/remote-control.md): applies, except the `mono` operation and the plugin operations, which ddmus does not have.

Their pages on other providers, plugins, headless mode and keybindings describe cliamp, not ddmus.

## Troubleshooting

- **"audio output unavailable":** ALSA cannot reach your sound server. Install `pipewire-alsa` or `pulseaudio-alsa`.
- **Some local files won't play:** AAC, ALAC, Opus and WMA need `ffmpeg`.
- **YouTube Music is empty or won't play:** it needs `yt-dlp` and a working sign-in; [docs/ddmus/youtube.md](docs/ddmus/youtube.md) covers the usual causes.
- **Spotify says "rate limited until …":** Spotify has blocked the client for a while; ddmus waits it out and retries by itself. See [docs/ddmus/catalog.md](docs/ddmus/catalog.md).
- **The log** is `~/.config/ddmus/ddmus.log`; `--log-level debug` says more.

## Project status

ddmus is feature-complete for 1.0, its first stable release: no features are being added before it. Release candidates and releases are listed on the [Releases](https://github.com/DaphnisDuck/ddmus/releases) page, and [CHANGELOG.md](CHANGELOG.md) says what each changes. From 1.0 on, a newer 1.x opens the config and catalog of an older one; going back to an older version is not supported. The catalog (`~/.local/share/ddmus/library.db`) can always be deleted and rebuilt by a sync; your sign-ins and settings live in `~/.config/ddmus`.

ddmus is maintained by one person. Bug reports are welcome in [Issues](https://github.com/DaphnisDuck/ddmus/issues).

## Lineage and thanks

ddmus would not exist without **[Bjarne Øverli](https://github.com/bjarneo)**, who created [cliamp](https://github.com/bjarneo/cliamp), and everyone who has [contributed to it](https://github.com/bjarneo/cliamp/graphs/contributors). Nearly everything that makes ddmus sound good is their work: the audio engine, the equalizer, the visualizers and the provider integrations. If you enjoy ddmus, please star, use and support cliamp, and visit [cliamp.stream](https://cliamp.stream).

ddmus adds the library, the catalog, search and the layout on top, and follows cliamp's development: see [docs/ddmus/upstream.md](docs/ddmus/upstream.md). It keeps its own config, data and media-key name, so both can be installed side by side.

Both build on [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), [Beep](https://github.com/gopxl/beep) and [go-librespot](https://github.com/devgianlu/go-librespot); the catalog uses [modernc.org/sqlite](https://gitlab.com/cznic/sqlite).

## License

ddmus's source code is MIT-licensed, the same as cliamp: see [LICENSE](LICENSE). The original copyright belongs to Bjarne Øverli.

The `ddmus` executable is a different matter. It links [go-librespot](https://github.com/devgianlu/go-librespot), which is GPL-3.0, so the executable as a whole is distributed under **GPL-3.0** ([LICENSE-GPL-3.0](LICENSE-GPL-3.0)). The release build ([docs/ddmus/releasing.md](docs/ddmus/releasing.md)) produces the complete corresponding source for each binary (`ddmus-<version>-source.tar.gz`) and a `THIRD_PARTY_NOTICES` file listing everything linked in, with its license.

The only official binaries are those published on this repository's [Releases](https://github.com/DaphnisDuck/ddmus/releases) page, and the AUR package `ddmus-bin` built from them once it exists. If you distribute a modified ddmus, please give it another name.

ddmus comes with no warranty; use it at your own risk.
