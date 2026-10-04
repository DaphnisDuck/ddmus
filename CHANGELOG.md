# Changelog

Notable changes to ddmus (DaphnisDuck's Music Player). The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). Within 1.x, a newer ddmus opens an older config and catalog; going back to an older ddmus is not supported.

ddmus is a fork of [cliamp](https://github.com/bjarneo/cliamp). Versions before 0.6 were released as omatunes.

## [1.0.0] - Unreleased

The first release for people who don't build from source. 1.0 adds no features to 0.10: it settles the command line, removes what ddmus doesn't support, and ships a tested Linux build.

### Added
- A release build for Linux x86-64 with checksums, third-party notices, an SBOM, a build attestation and the complete corresponding source; an AUR package, `ddmus-bin`.
- `ddmus setup` sets up Spotify, YouTube Music and the Local music folder, and keeps every other line of an existing `config.toml`.
- `--version` answers in every build.
- Spotify syncs every 2 hours by default.

### Changed
- ddmus 1.0 supports Spotify, YouTube Music, local files and internet radio. cliamp's other providers (Navidrome, Plex, Jellyfin, SoundCloud and the rest) are not offered: `--provider` and `setup` no longer list them, and their sections in `config.toml` are left untouched but have no effect.
- The log and the IPC socket are named `ddmus.log` and `ddmus.sock` (they kept cliamp's names before). A script that opens the socket by path needs the new name.
- In "Search Spotify for …" and its YouTube Music twin, Enter replaces the queue, as playing from the library does.
- `shuffle` and `repeat` reject values they don't know.
- The executable is distributed under GPL-3.0, because it links go-librespot; ddmus's own source stays MIT.

### Removed
- Lua plugins: none is loaded, and the `plugins` command and its IPC operations are gone. Plugin files and `[plugins.*]` config stay where they are, unused.
- `--daemon` (headless mode), `--mono`, `--expanded`, `--simplified`, and the `mono`, `open`, `protocol`, `qobuz`, `radio` and `tidal` commands. `cliamp://` links are neither registered nor handled; see [docs/ddmus/files.md](docs/ddmus/files.md) if an earlier ddmus registered them.
- `ddmus upgrade`: update with your package manager or from the releases page.

### Fixed
- Two quick volume changes from a desktop media control could freeze ddmus; a dropped session bus could crash it.
- A stalled YouTube Music download froze every control until yt-dlp gave up.
- Opening a Spotify track on a stalled network froze ddmus for up to 30 seconds.
- A Local playlist that lists a track more than once lost a copy on every save.
- A YouTube Music sign-in made inside ddmus stopped working about an hour later, until a restart.
- A stalled Spotify request could hold a library sync until quit.
- Stop now cancels an album that is still opening from search.
- Hardening from an independent review of playback, catalog identity and sync.

## [0.10.0] - 2026-09-30
- Album artwork in the track info view (`i` in the queue) in Kitty and Ghostty, from Spotify covers and local files.

## [0.9.0] - 2026-09-30
- ddmus fills the terminal: taller windows list more rows, wide ones line rows up as a table, and resizing reflows at once. A border frames the screen (`border = false` under `[ddmus]` turns it off).

## [0.8.0] - 2026-09-30
- One set of keys everywhere: `q` quits from any screen, `Esc` only goes back, `p` and `n` skip tracks, and `/` is only ever search.
- Spotify's long rate-limit blocks no longer hang album opens, and are waited out across restarts.

## [0.7.0] - 2026-09-30
- The queue view works under the library: shuffle, repeat, play next, track info, queue editing, lyrics and the sound keys. Every view lists its keys in a bar at the bottom.

## [0.6.0] - 2026-09-30
- Renamed from omatunes to DaphnisDuck's Music Player, `ddmus`. Settings and library move once, by hand: see [docs/ddmus/files.md](docs/ddmus/files.md).

## [0.5.0] - 2026-09-29
- Faster, quieter syncs, sturdier YouTube enrichment, and a keymap that shows only the keys that work.

## [0.4.0] - 2026-09-29
- YouTube Music: playlists and Liked Music sync into the catalog; albums and artists are read in the background.

## [0.3.0] - 2026-09-29
- Search-as-you-type across Spotify, local files and radio stations, offline; an All Music menu.

## [0.2.0] - 2026-09-29
- A local catalog of the Spotify library and the music folder, synced in the background, so browsing is instant and works offline.

## [0.1.0] - 2026-09-29
- Library navigation (Music → Spotify, Local, Radio) above cliamp's player, queue, EQ and visualizer.
