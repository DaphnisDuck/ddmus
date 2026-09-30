# ddmus files and names

DaphnisDuck's Music Player (`ddmus`) keeps its own files and runtime names, so it can be installed and run alongside cliamp.

| What | ddmus | cliamp |
|---|---|---|
| Config (config.toml, playlists, plugins, history, resume, radio favorites, credentials, log, IPC socket) | `~/.config/ddmus` | `~/.config/cliamp` |
| Data (music catalog `library.db`, album-art cache, plugin stores) | `~/.local/share/ddmus` | `~/.local/share/cliamp` |
| Default downloads (`Ctrl+S`) | `~/Music/ddmus` | `~/Music/cliamp` |
| MPRIS (media keys) | `org.mpris.MediaPlayer2.ddmus` | `org.mpris.MediaPlayer2.cliamp` |
| Client name and device ID reported to Plex, Navidrome, Emby, Jellyfin and podcast servers | `ddmus` | `cliamp` |
| PulseAudio/PipeWire stream matched when switching output device | `ddmus` | `cliamp` |
| Config override variable | `DDMUS_CONFIG_DIR` | `CLIAMP_CONFIG_DIR` |

All of these come from `internal/appdir/ddmus.go` (`Name`, `ConfigDirEnv`, `DownloadsDir`); `internal/appmeta` derives its client name from `appdir.Name`.

- **Config directory order:** `CLIAMP_CONFIG_DIR`, then `DDMUS_CONFIG_DIR`, then `$XDG_CONFIG_HOME/ddmus`, then `~/.config/ddmus`. `CLIAMP_CONFIG_DIR` comes first because upstream's tests set it to isolate themselves, and exporting `DDMUS_CONFIG_DIR` must not send them to your real config. So don't export `CLIAMP_CONFIG_DIR` globally if you run both players. Under `go test`, `DDMUS_CONFIG_DIR` is ignored entirely, so tests never touch your real config.
- **File names:** files keep upstream's names (`cliamp.log`, `cliamp.sock`) inside the ddmus directory.
- **CLI subcommands:** `ddmus pause`, `ddmus next` and the rest talk to the ddmus socket, so they control ddmus and never cliamp.

## Moving from omatunes (before v0.6)

Until v0.6 this player was called omatunes and kept its files under that name. It does not read or move them, since `~/.config/omatunes` may belong to the unrelated omatunes player. To carry over your settings, sign-ins and library, quit the old build and move them once:

```sh
mv -T ~/.config/omatunes ~/.config/ddmus
mv -T ~/.local/share/omatunes ~/.local/share/ddmus
[ -d ~/Music/omatunes ] && mv -T ~/Music/omatunes ~/Music/ddmus
sed -i 's/^\[omatunes\]/[ddmus]/' ~/.config/ddmus/config.toml
```

- Check first that those folders are this player's (they hold `config.toml`, `library.db` and `cliamp.log`).
- If you already started ddmus once, it created `~/.config/ddmus` and `~/.local/share/ddmus` folders. Remove them first; `mv -T` refuses to move onto a folder that holds anything.
- Afterwards the section header in `config.toml` should read `[ddmus]`. Any path you set there that points into an omatunes folder (a `[downloads] directory`, say) needs updating by hand.
- `OMATUNES_CONFIG_DIR` is now `DDMUS_CONFIG_DIR`.

## Moving settings from cliamp

ddmus does not read cliamp's directory. To start from your cliamp settings, copy them once:

```sh
rsync -a --exclude='*.log' --exclude='*.sock' ~/.config/cliamp/ ~/.config/ddmus/
rsync -a ~/.local/share/cliamp/ ~/.local/share/ddmus/
```

## Still shared

- **Spotify sign-in:**
  - A copied `spotify_credentials.json` holds the same Spotify refresh token as cliamp's. If Spotify rotates the token for one app, the other asks you to sign in again. Run `ddmus spotify reset` for a separate sign-in.
  - Both apps use the fixed OAuth callback port `127.0.0.1:19872`, so don't sign in to both at the same moment.
- **`cliamp://` links:** `ddmus protocol register` would register ddmus as the handler for `cliamp://` links, using the same `cliamp-url-handler.desktop` file as cliamp. Don't register it if cliamp should keep handling those links.

## After an upstream merge

Upstream tests hard-code `cliamp` as a path segment under a temporary HOME. ddmus rewrites those literals. If a merge brings in new ones, re-apply the rewrite:

```sh
files=$(grep -rl --include=*_test.go -E '"\.config", "cliamp"|"Music", "cliamp"|share", "cliamp"|\.config/cliamp' . | grep -v upgrade/)
sed -i -E 's/"\.config", "cliamp"/".config", "ddmus"/g; s/"Music", "cliamp"/"Music", "ddmus"/g; s/("share", )"cliamp"/\1"ddmus"/g; s#\.config/cliamp#.config/ddmus#g' $files
```
