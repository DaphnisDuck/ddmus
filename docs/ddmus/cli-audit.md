# ddmus 1.0 CLI audit

Working record for Workstream 1 of the [release plan](release-plan.md). Every public command and option ends in one of four categories: **verified and supported**, **fixed and supported**, **intentionally retained and documented**, or **removed before 1.0**. Rows marked *to verify* still need a behavior check. Checks so far (2026-10-01) ran ddmus in an isolated home (config, data, cache and state under a scratch HOME, its own IPC socket, a private D-Bus session, ALSA's null device, generated test tracks).

Inventory taken 2026-10-01 from `ddmus --help`, recursively, at 5597fd0 (`release-1.0`): 31 visible root commands plus 2 hidden (`upgrade`, `radio`), 37 subcommands, 25 global options.

## Commands

| Command | Subcommands | Status | Notes |
| --- | --- | --- | --- |
| `play` `pause` `toggle` `next` `prev` `stop` | | verified (state changes) | Against a live isolated ddmus over IPC. Timing (positions) not checkable on the null audio device; covered by real-audio use |
| `status` | | **fix** | Works, `--json` too; its text output still prints `Mono:` (goes with mono) |
| `volume` `seek` `speed` `shuffle` `repeat` | | **fix** | Work, and out-of-range values clamp (`speed 9` → 2.00x, `volume 99` → +6 dB). But `repeat bogus` and `shuffle bogus` exit 0 and change the setting; invalid values must fail |
| `eq` | | verified | Presets and `--band`. An invalid band says only "invalid parameters"; could name the 0–9 range |
| `device` | | to verify | `device list` printed nothing on the null audio device; check on real hardware |
| `load` | | verified | Loads a local playlist. A missing one gives a raw "internal_error … no such file" message |
| `queue` | | verified | Appends a file or URL; an unreachable URL is accepted silently (resolved asynchronously) |
| `theme` `vis` | | verified | `list`, set, and an unknown name fails cleanly ("not found") |
| `visstream` | | verified | NDJSON frames |
| `mono` | | **removed** (slices 1, 2) | M8 removed mono from the UI. Also the IPC `mono` operation (v1 and v2), so nothing else reaches it |
| `remote` | `state` `capabilities` `call` `job` `cancel` `events` | **fix** | Works. `capabilities` advertises `mono`, `plugin.call`, `plugin.commands` and generic `provider.*` operations that can name hidden providers: remove or restrict them with the decided removals. Replies carry `"id":"cliamp"` (protocol identity; decide whether to keep for compatibility). `lyrics`: keep and verify (decided 2026-10-01; the lyrics view is part of the queue screen). `save` (download the current track): verify against the supported providers, then decide |
| `playlist` | `list` `create` `rename` `add` `dirs` `show` `remove` `delete` `dedupe` `sort` `doctor` `export` `import` `bookmark` `bookmarks` `enrich` | verified, small **fixes** | All 16 work in isolation, and the playlists appear under Local → Playlists. Fix: `remove --index 9` reports "index 8 out of range"; `delete` of a missing playlist shows a raw filesystem error. `create --ssh` not exercised (needs an SSH host) |
| `history` | `clear` | verified, small **fix** | Works, `--json` too. `history clear` lists `--limit` and `--json`, which mean nothing to it |
| `youtube` | `signin` | verified (unconfigured path) | Without a client it points to `docs/ddmus/youtube.md`. The browser flow is the owner's check (scenario F) |
| `spotify` | `reset` | verified | "No stored Spotify credentials to remove" in isolation |
| `setup` | | **fix** | Offers 12 providers and never sets up Local; help says it writes `~/.config/cliamp/config.toml`. Rework to Spotify, YouTube Music and the Local folder |
| `plugins` | `list` `install` `trust` `remove` `call` `commands` | **removed** (slices 1, 2) | With plugin loading at startup and the IPC plugin operations; existing plugin config stays inert |
| `qobuz` | `reset` | **remove** (decided) | Hidden provider |
| `tidal` | `reset` `probe` | **remove** (decided) | Hidden provider |
| `open` | | **remove** (decided) | `cliamp://` dispatch; help examples name navidrome and ytmusic |
| `protocol` | `register` `unregister` `status` | **remove** (decided) | `cliamp://` registration; cleanup notes for registrations ddmus made |
| `upgrade` (hidden) | | **fix** (decided 2026-10-01) | Keep the hidden refusal stub; its advice becomes "update with your package manager (AUR: `ddmus-bin`) or download the latest release". Leave upstream's unused `upgrade/` package untouched (not compiled in; deleting it would cost merge conflicts). Not in the golden file |
| `radio` (hidden) | | **remove** (decided 2026-10-01) | cliamp's easter egg: live listener stats for cliamp's radio channels from radio.cliamp.stream (`--stats`, `--globe`, `--json`) |

## Global options

| Option | Status | Notes |
| --- | --- | --- |
| `--help`, `-h` | verified | |
| `--version`, `-v` | **fix** | Works with `make build`; a plain `go build` has no version and the flag disappears. Every build should answer ("dev" at least) |
| `--provider` | **fixed** (slice 2) | Lists 20 providers, with `cliamp` as the local provider's name; `--provider plex` is accepted silently. `--provider radio` does load a station at startup. Narrow to the supported ones (decided) |
| `--daemon`, `-d` | **remove** (decided) | |
| `--playlist` | verified | With `--auto-play`, starts the playlist |
| `--auto-play` `--shuffle` `--repeat` `--vol` `--eq-preset` | to verify | |
| `--mono` | **remove** (decided 2026-10-01) | With the `mono` command |
| `--expand-playlist` | to verify | YouTube Music `list=` URLs |
| `--expanded` | **remove** (decided 2026-10-01) | No visible effect in the library UI; its help names cliamp's Ctrl+X state |
| `--simplified` | **remove** (decided 2026-10-01) | No visible effect on the library screens |
| `--help-bar` | **fix** | Works (`--no-help-bar` hides the key bar). Its help says "? still opens the full keymap"; on ddmus's library screens `?` opens nothing |
| `--low-power` | to verify | No visible change on the library screens (it lowers UI cadence and disables the visualizer); confirm the CPU effect |
| `--start-theme` `--visualizer` `--visualizer-60fps` | to verify | An unknown `--start-theme` is ignored silently |
| `--audio-device` `--sample-rate` `--bit-depth` `--buffer-ms` `--resample-quality` | to verify | Audio engine, unchanged by ddmus |
| `--log-level` | to verify | |

## Terminology found in help text

- Every usage error says `usage: cliamp …` (`volume`, `seek`, `eq`, `device`, `theme`, `vis`, `load`, `queue`, `remote events`, all `playlist` subcommands).
- `upgrade`'s refusal (fix decided).
- IPC files: decided 2026-10-01, `cliamp.sock` and `cliamp.log` in `~/.config/ddmus/` become `ddmus.sock` and `ddmus.log` (two tagged one-line edits; the `.pid` file follows the socket; note it in the changelog and `docs/ddmus/files.md`). The v2 `"id":"cliamp"` is only the request label `ddmus remote` sends and the player echoes; it stays.

## Radio

- cliamp radio's channels (upstream's `radio.cliamp.stream`, under Radio → Browse Stations) stay in ddmus (decided 2026-10-01). They are the default `--provider` (key `cliamp`); when `--provider` is narrowed, that key stays, and the docs call it "cliamp radio".

- `plugins call` / `plugins commands`: "in the running cliamp" (goes with plugins).
- `open`, `protocol`: `cliamp://` and `cliamp protocol register` (go with the removals).
- `setup`: "writes ~/.config/cliamp/config.toml" (wrong path; ddmus writes `~/.config/ddmus`).
- `--provider`: `cliamp` as a provider name.

## Inherited behavior to harden (Gate B, release-hardening)

- cliamp's Spotify search ("Search Spotify for …"): Enter on an album or track replaces the queue (decided); `a` and `q` unchanged.
- Stop during that screen's album load must cancel it.

Done 2026-10-02: Enter (track or album) plays through `spotSearchPlay` (`ui/model/library_search.go`), which replaces the queue as library plays do; `a` and `q` are unchanged. `stopByUser` also invalidates the overlay's pending album request, whatever its action, so a Stop from a key, IPC or media controls drops it (an album to append would otherwise auto-play into the stopped player). Tests: `ui/model/library_spotsearch_test.go`. The overlay serves every live-searching provider, so YouTube Music's live search behaves the same.

## Slice 1: the command-line surface (2026-10-01)

Done on `release-1.0`; the runtime removals are slice 2, below.

- `cli_ddmus.go`: `ddmusApp` is upstream's `buildApp` as ddmus ships it. The removed commands (`mono`, `open`, `plugins`, `protocol`, `qobuz`, `radio`, `tidal`) become hidden stubs that say `"mono" is not part of ddmus 1.0`, instead of falling through to the player as a file name. The removed flags (`--daemon`, `--mono`, `--expanded`, `--simplified`) are gone. `--help-bar` is reworded; `history`'s options no longer reach `history clear`; a build without a version answers `--version` with `dev`. Upstream's `buildApp` and its tests are untouched.
- `testdata/cli-surface.golden` pins every visible command, alias, option, default and scope (`TestCLISurfaceGolden`; regenerate with `DDMUS_UPDATE_GOLDEN=1`).
- Usage errors say `usage: ddmus …` (`cliError`, one hook in `main`).
- `shuffle` and `repeat` reject unknown values, in the CLI and in the player's IPC handler (`ipc.ValidModeName`).
- `status` prints no `Mono:` line; `playlist remove` reports the index as given; `upgrade`'s refusal points to the package manager and the releases page.
- The log and socket are `ddmus.log` and `ddmus.sock` (`docs/ddmus/files.md`). The `cliamp://` cleanup note is in `docs/ddmus/files.md`.

## Slice 2: runtime removals (2026-10-02)

What slice 1 took off the command line, slice 2 makes unreachable at runtime (`providers_ddmus.go`, three tagged lines in `main.go`):

- **Providers:** ddmus 1.0's are cliamp radio (`cliamp`), `radio`, `local`, `spotify` and `ytmusic`. `hideProviders` clears the other providers' config in memory right after the config loads, so none is constructed; `config.toml` is never rewritten, so their sections stay as they were. `supportedOnly` then drops what main builds regardless of config: the podcast directory, YouTube's non-music views (`yt`, `youtube`) and servers named by environment variables (`NAVIDROME_*`, `LYRION_*`). With the list narrowed, nothing else can reach them: IPC's `provider.*` operations answer only for these five, and the Shift+letter provider hotkeys (which the library screens already swallow) find nothing to switch to.
- **`--provider`** accepts `cliamp`, `radio`, `spotify` and `ytmusic` (a validator in `ddmusApp`; upstream never offered `local` as a start provider); a `provider =` in `config.toml` naming another falls back to the default.
- **Plugins** never load: `luaplugin.New` isn't called, so no plugin file is read and every plugin hook is skipped. `[plugins.*]` config stays inert.
- **IPC** drops `mono`, `plugin.call` and `plugin.commands` from the operation registry, so `remote capabilities` no longer lists them and calling one answers `unknown_operation`.

Tests: `providers_ddmus_test.go` (`TestHideProvidersTurnsOffUnsupported`, `TestHideProvidersKeepsSupportedDefault`, `TestSupportedOnly`, `TestDDMUSOperationsDropRemoved`, `TestProviderFlagNarrowed`). Live (isolated config with `provider = "plex"`, `[plex]`, `[navidrome]`, `[soundcloud]`, `[qobuz]` and a plugin file): `provider.list` returned cliamp, radio and local; `mono` and `plugin.commands` answered `unknown_operation`; no `plugins.log` was created.

Not done here: setup still offers cliamp's providers (its own slice), and upstream's provider code stays compiled in (deleting it would cost upstream merges).

