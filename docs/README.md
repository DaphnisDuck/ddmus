# Documentation

ddmus's own documentation is in [ddmus/](ddmus/); start with the [README](../README.md).

Every other page in this folder is **cliamp's documentation**, kept as it is upstream so that ddmus can keep following cliamp. ddmus is a fork: it shares cliamp's player, but not its interface or all of its sources, so these pages fit ddmus only in part. They have not been rewritten for ddmus. Wherever one applies, read `ddmus` for `cliamp`, `~/.config/ddmus` for `~/.config/cliamp`, and `ddmus.sock` and `ddmus.log` for `cliamp.sock` and `cliamp.log`.

## ddmus's pages

| Page | What it covers |
| --- | --- |
| [ddmus/navigation.md](ddmus/navigation.md) | The library, the queue, and every key |
| [ddmus/search.md](ddmus/search.md) | Search and its query language |
| [ddmus/catalog.md](ddmus/catalog.md) | What the catalog stores, when it syncs, Spotify rate limits |
| [ddmus/youtube.md](ddmus/youtube.md) | YouTube Music setup |
| [ddmus/artwork.md](ddmus/artwork.md) | Album artwork and terminals |
| [ddmus/layout.md](ddmus/layout.md) | How ddmus uses the window |
| [ddmus/files.md](ddmus/files.md) | Files and folders, running beside cliamp |
| [ddmus/upstream.md](ddmus/upstream.md), [ddmus/releasing.md](ddmus/releasing.md) | For contributors: following cliamp, cutting a release |

The other files in `ddmus/` are records of how 1.0 was prepared.

## cliamp's pages that describe something ddmus has

Each describes a part ddmus shares with cliamp. The key presses they mention are cliamp's; ddmus's keys are in [ddmus/navigation.md](ddmus/navigation.md).

| Page | In ddmus |
| --- | --- |
| [spotify.md](spotify.md) | Creating your own Spotify Developer app applies. Browsing Spotify is the library's job: see [ddmus/catalog.md](ddmus/catalog.md) |
| [audio-quality.md](audio-quality.md) | The audio settings in `config.toml` apply |
| [configuration.md](configuration.md) | The player settings, custom radio stations (`radios.toml`) and the WSL2 notes apply. The sections on other providers and plugins do not. ddmus's own settings are the `[ddmus]` section: see `config.toml.example` |
| [themes.md](themes.md) | The theme files and settings apply. Choose a theme with `--start-theme` or `ddmus theme`; the `t` picker is not in ddmus |
| [playlists.md](playlists.md) | The playlist file format and the `ddmus playlist` commands apply. The playlist-manager keys do not |
| [lyrics.md](lyrics.md) | Lyrics open with `y` in the queue |
| [history.md](history.md) | `ddmus history` applies. Browsing history inside the player is cliamp's |
| [mediactl.md](mediactl.md) | Media keys and desktop controls apply; ddmus's MPRIS name is `org.mpris.MediaPlayer2.ddmus` |
| [remote-control.md](remote-control.md), [upgrading-ipc-v2.md](upgrading-ipc-v2.md) | The socket API applies, except the `mono` operation and the plugin operations, which ddmus does not have |
| [cli.md](cli.md) | Many commands are the same, but `--daemon`, `--mono`, `upgrade`, `protocol`, `open` and `plugins` are not in ddmus. `ddmus --help` is the accurate list |
| [radio.md](radio.md) | The station directory and the cliamp radio channels are under Radio → Browse Stations. The radio pane, its keys and its location feature are cliamp's |
| [youtube-music.md](youtube-music.md) | The two ways of signing in are the same. ddmus has no separate YouTube (non-music) source: read [ddmus/youtube.md](ddmus/youtube.md) instead |
| [streaming.md](streaming.md), [yt-dlp.md](yt-dlp.md), [ssh-streaming.md](ssh-streaming.md) | Playing a URL given on the command line. ddmus inherits this from cliamp and does not test it; podcast feeds are not part of ddmus |

## cliamp's pages that do not apply to ddmus

ddmus 1.0 does not include what these describe. The code may still be in the tree, unused, so that ddmus can keep merging cliamp's changes.

| Pages | Why |
| --- | --- |
| [audiobookshelf.md](audiobookshelf.md), [emby.md](emby.md), [jellyfin.md](jellyfin.md), [lyrion.md](lyrion.md), [mixcloud.md](mixcloud.md), [navidrome.md](navidrome.md), [netease.md](netease.md), [plex.md](plex.md), [podcasts.md](podcasts.md), [qobuz.md](qobuz.md), [soundcloud.md](soundcloud.md), [tidal.md](tidal.md), [yandex.md](yandex.md) | Sources ddmus does not offer. Its sources are Spotify, YouTube Music, local files and radio |
| [plugins.md](plugins.md), [community-plugins.md](community-plugins.md) | ddmus loads no plugins |
| [headless.md](headless.md) | ddmus has no headless mode |
| [url-scheme.md](url-scheme.md) | ddmus neither registers nor handles `cliamp://` links |
| [keybindings.md](keybindings.md) | cliamp's keys. ddmus's are in [ddmus/navigation.md](ddmus/navigation.md) |
| [provider-development.md](provider-development.md) | Written for cliamp's contributors |
