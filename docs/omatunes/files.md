# omatunes files and names

omatunes keeps its own files and runtime names, so it can be installed and run alongside cliamp.

| What | omatunes | cliamp |
|---|---|---|
| Config (config.toml, playlists, plugins, history, resume, radio favorites, credentials, log, IPC socket) | `~/.config/omatunes` | `~/.config/cliamp` |
| Data (album-art cache, plugin stores) | `~/.local/share/omatunes` | `~/.local/share/cliamp` |
| Default downloads (`Ctrl+S`) | `~/Music/omatunes` | `~/Music/cliamp` |
| MPRIS (media keys) | `org.mpris.MediaPlayer2.omatunes` | `org.mpris.MediaPlayer2.cliamp` |
| Client name and device ID reported to Plex, Navidrome, Emby, Jellyfin and podcast servers | `omatunes` | `cliamp` |
| PulseAudio/PipeWire stream matched when switching output device | `omatunes` | `cliamp` |
| Config override variable | `OMATUNES_CONFIG_DIR` | `CLIAMP_CONFIG_DIR` |

All of these come from `internal/appdir/omatunes.go` (`Name`, `ConfigDirEnv`, `DownloadsDir`); `internal/appmeta` derives its client name from `appdir.Name`.

- **Config directory order:** `CLIAMP_CONFIG_DIR`, then `OMATUNES_CONFIG_DIR`, then `$XDG_CONFIG_HOME/omatunes`, then `~/.config/omatunes`. `CLIAMP_CONFIG_DIR` comes first because upstream's tests set it to isolate themselves, and exporting `OMATUNES_CONFIG_DIR` must not send them to your real config. So don't export `CLIAMP_CONFIG_DIR` globally if you run both players. Under `go test`, `OMATUNES_CONFIG_DIR` is ignored entirely, so tests never touch your real config.
- **File names:** files keep upstream's names (`cliamp.log`, `cliamp.sock`) inside the omatunes directory.
- **CLI subcommands:** `omatunes pause`, `omatunes next` and the rest talk to the omatunes socket, so they control omatunes and never cliamp.

## Moving settings from cliamp

omatunes does not read cliamp's directory. To start from your cliamp settings, copy them once:

```sh
rsync -a --exclude='*.log' --exclude='*.sock' ~/.config/cliamp/ ~/.config/omatunes/
rsync -a ~/.local/share/cliamp/ ~/.local/share/omatunes/
```

## Still shared

- **Spotify sign-in:**
  - A copied `spotify_credentials.json` holds the same Spotify refresh token as cliamp's. If Spotify rotates the token for one app, the other asks you to sign in again. Run `omatunes spotify reset` for a separate sign-in.
  - Both apps use the fixed OAuth callback port `127.0.0.1:19872`, so don't sign in to both at the same moment.
- **`cliamp://` links:** `omatunes protocol register` would register omatunes as the handler for `cliamp://` links, using the same `cliamp-url-handler.desktop` file as cliamp. Don't register it if cliamp should keep handling those links.

## After an upstream merge

Upstream tests hard-code `cliamp` as a path segment under a temporary HOME. omatunes rewrites those literals. If a merge brings in new ones, re-apply the rewrite:

```sh
files=$(grep -rl --include=*_test.go -E '"\.config", "cliamp"|"Music", "cliamp"|share", "cliamp"|\.config/cliamp' . | grep -v upgrade/)
sed -i -E 's/"\.config", "cliamp"/".config", "omatunes"/g; s/"Music", "cliamp"/"Music", "omatunes"/g; s/("share", )"cliamp"/\1"omatunes"/g; s#\.config/cliamp#.config/omatunes#g' $files
```
