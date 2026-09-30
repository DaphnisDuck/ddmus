# YouTube Music

ddmus syncs your YouTube Music playlists and Liked Music into its catalog, so they browse and search offline next to Spotify and your local files. In the background it also reads each track's real artist, album and year, which gives YouTube Music its own Albums and Artists lists. Tracks play through yt-dlp, as in cliamp.

```
Music → YouTube Music   Albums · Artists · Playlists · Liked Music
```

## Setup

You need [yt-dlp](https://github.com/yt-dlp/yt-dlp) (it plays every track) and at least one way of signing in. Both are cliamp's own, configured in `~/.config/ddmus/config.toml` under `[ytmusic]`, and you can use both at once.

### Browser cookies (no Google setup)

yt-dlp borrows the YouTube session of a browser you are signed in to:

```toml
[ytmusic]
enabled = true
cookies_from = "firefox"   # or chrome, chromium, brave, edge, opera, safari
```

On Linux, Chrome-family browsers encrypt their cookies with the desktop keyring. Name it after a `+`, and install the keyring support yt-dlp needs:

```toml
cookies_from = "brave+gnomekeyring"   # or "chromium+kwallet", …
```

On Arch, the GNOME keyring needs `python-secretstorage` (`sudo pacman -S python-secretstorage`). Without it yt-dlp reports `cannot decrypt v11 cookies` and YouTube treats you as signed out.

### Your own Google OAuth client

The official YouTube Data API, through a free Google Cloud OAuth client you create once (about 10 minutes):

1. At [console.cloud.google.com](https://console.cloud.google.com/), create a project and enable **YouTube Data API v3**.
2. **OAuth consent screen** (Google Auth Platform): user type **External**, scope `https://www.googleapis.com/auth/youtube.readonly`, and add **your Google account as a test user**. Without it, sign-in fails with "Access blocked: … has not completed the Google verification process".
3. **Credentials → Create Credentials → OAuth client ID**, application type **Desktop app**. Copy the client ID and secret:

```toml
[ytmusic]
enabled = true
client_id = "…apps.googleusercontent.com"
client_secret = "…"
```

Then sign in once with `ddmus youtube signin`, which opens your browser; approve read-only access (past "Google hasn't verified this app"). The token is kept in `~/.config/ddmus/ytmusic_credentials.json`. While the consent screen is in **Testing**, Google may expire the sign-in after about a week; publishing the app avoids that.

To switch to another Google account, run `ddmus youtube signin --force`: it opens Google's account chooser even when you are signed in, and replaces the stored token only when the new sign-in succeeds. The next sync replaces the old account's playlists and Liked Music.

## What syncs, and how

| | With cookies | With OAuth |
|---|---|---|
| Your playlists | yes (the youtube.com playlists feed) | often none: the API lists few or none |
| Playlists saved from others | only those listed in `youtube_playlists` | only those listed in `youtube_playlists` |
| Liked Music | yes | yes, with cleaner artist names |
| Cost | about 1.5 s per playlist per sync | quota: about 3 units per sync (10,000 a day) |

With both configured, ddmus reads Liked Music through OAuth and playlists through cookies. When OAuth needs signing in again, Liked Music falls back to cookies.

- **Only music:** each playlist is sampled once and kept only if YouTube files its videos under Music. Watch later and Liked videos are never listed. The answers are cached in `ytmusic_classification.json`, so only new playlists take a few seconds on the next sync. A new playlist that cannot be sampled yet (it is empty, or every sampled video is blocked) is left out until a later sync can tell; one already synced stays, with its tracks.
- **Playlists saved from others:** YouTube lists playlists you save or bookmark from other people nowhere a sync can read. Add them by link (one line), and they sync as followed playlists:

  ```toml
  [ddmus]
  youtube_playlists = ["https://music.youtube.com/playlist?list=PL…", "PL…"]
  ```

- **When:** at startup when the last sync is older than `youtube_refresh` (2 hours by default), and when you press `r` in YouTube Music.

  ```toml
  [ddmus]
  youtube_refresh = "6h"
  ```

## Albums and Artists (enrichment)

A playlist knows each track only as a video title and the channel that uploaded it. After each sync, ddmus reads the tracks you have, one at a time in the background (about 4–5 s each, newest first), for their real title, artists, album and year. As it goes:

- YouTube Music's **Albums** and **Artists** lists fill in, and they join **All Music** and search.
- A YouTube album holds the tracks of it you have, not the whole album; opening it plays those.
- A track with no music details (a fan upload, say) keeps its video title and uploader.
- A track yt-dlp keeps failing to read is skipped, so the rest are still read. After three passes that read other tracks but failed on it, it is left as it is (video title and uploader). A pass that reads nothing, or fails on several tracks in a row, looks like an outage: ddmus stops and tries again after the next sync, without holding it against those tracks.

The first pass over a large library takes a while (about half an hour for 360 tracks); it pauses while YouTube syncs, resumes after a restart, and slows right down if YouTube asks it to (a "not a bot" check).

## Troubleshooting

- **No YouTube Music in Music:** check that `[ytmusic]` does not say `enabled = false`, that it has a sign-in (cookies or both OAuth keys), and that `yt-dlp` is on your PATH.
- **"sync failed" for YouTube:** the log (`~/.config/ddmus/cliamp.log`) has yt-dlp's message. A 401, or `cannot decrypt`, means the cookies are not being read (see the keyring note above).
- **A saved playlist is missing:** add its link to `youtube_playlists`.
