# The music catalog

omatunes keeps your Spotify library and an index of your local music folder in one SQLite database, `~/.local/share/omatunes/library.db`. The library screens read from it, so lists open instantly and keep working offline. Playback still goes to Spotify, or to the file itself.

## What it holds

- **Spotify:** saved albums, followed artists, liked songs, and the playlists you own or follow, with their tracks. Album track lists are cached as you open albums, and in the background for the rest of your saved albums.
- **YouTube Music:** your music playlists (and those listed in `youtube_playlists`) with their tracks, and Liked Music. Each track's artist, album and year is read in the background, which gives YouTube its Albums and Artists; an album holds only the tracks of it you have. See [youtube.md](youtube.md).
- **Local:** every audio file under the music folder, grouped into albums, artists and genres. The folder is `initial_directory` in `config.toml`, else `$XDG_MUSIC_DIR`, else `~/Music`.
- **Radio:** your favorite stations and the built-in and `radios.toml` stations, so [search](search.md) finds them. They are read from local files at every start and whenever you star or unstar a station. Browsing Radio still reads them directly.
- **Search index:** a full-text index of everything above, kept in step with every change. See [search.md](search.md).
- **Not in the catalog:** the radio directory, local playlists, and an artist's full Spotify discography. These are always loaded live.

No passwords or tokens are stored in the catalog. Spotify credentials stay in `~/.config/omatunes/spotify_credentials.json`.

## When it updates

| Source | At startup | On `r` | After a failure |
|---|---|---|---|
| Spotify | if the last successful sync is older than `spotify_refresh` (30 minutes by default) | yes | retries after 1 minute, doubling up to every 30 minutes |
| YouTube Music | if the last successful sync is older than `youtube_refresh` (2 hours by default) | yes | the same retries |
| Local | always; only files whose size or modification time changed are read again | yes | the same retries |

`r` syncs the source you're browsing. At the Music root it syncs every source. Syncs run in the background, and the screen refreshes in place when one lands, keeping the cursor on the same item.

**Spotify album tracks:** opening an album whose tracks aren't cached yet loads it from Spotify and caches it. A background fill then caches your other saved albums one at a time, newest saved first, about one a second. It pauses while you open an album or a sync runs, and when Spotify rate-limits it, it waits as long as Spotify asks (at least 5 seconds, doubling up to 10 minutes).

## Status

The top of the library shows each source's state:

- `↻ syncing`
- `✓ synced 2m ago`
- `sync failed · cached`: the last attempt failed, and you're browsing what the catalog already holds.

With both Spotify and Local, each status is prefixed with its source. Details of a failure are in `~/.config/omatunes/cliamp.log`.

## Failure safety

A sync either applies completely or changes nothing:

- **Spotify:** if any page of a collection fails, or the library changes while it's being read, that collection keeps what it had.
- **Local:** if the music folder is missing, or is empty while the catalog holds files (an unmounted drive, for example), the index is left as it is. Files under a folder omatunes can't read are kept too.
- **Sign-in:** a Spotify sign-in that fails because Spotify is unreachable or down is reported as that error, not as "sign-in required". The sync retries by itself.

## Settings

In `~/.config/omatunes/config.toml`:

```toml
[omatunes]
spotify_refresh = "30m"   # sync Spotify at startup if the last sync is older than this; "0s" syncs every time
youtube_refresh = "2h"    # the same for YouTube Music
youtube_playlists = []    # other people's YouTube playlists to sync, by link (see youtube.md)
```

Durations use Go's format: `90s`, `30m`, `2h`. An invalid value keeps the default.

## Starting over

The catalog can always be rebuilt from Spotify and your music folder. To reset it, quit omatunes and delete `~/.local/share/omatunes/library.db`, along with the `library.db-wal` and `library.db-shm` files next to it. The next start syncs everything again.
