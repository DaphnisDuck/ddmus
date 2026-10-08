# ddsonic Post-1.0 Ideas and Investigations

> **Status:** Brainstorming and investigation only  
> **Scope:** After the 1.0 release  
> **Important:** Nothing in this document is a committed feature, milestone, release target, or implementation plan unless it is promoted into the project roadmap separately.

This document preserves ideas discussed during development of ddsonic that may be worth investigating after 1.0. Its purpose is to prevent good ideas from being lost while keeping the 1.0 release focused.


This is the canonical catalog of post-1.0 ideas and investigations. [The project plan](../../plan.md#post-10-backlog) remains authoritative for scheduled work and existing deferred engineering decisions. Recording or consolidating an idea here does not promote it into that roadmap or change the 1.0 feature freeze. See [existing deferred work](#existing-deferred-work) for items already tracked there.

## Guiding Principle

ddsonic should remain a **fast, terminal-native music player**, not gradually become a sprawling media-management platform.

Every post-1.0 feature should justify the complexity it introduces.

When evaluating an idea, ask:

- Does this make listening to music better?
- Does it reinforce ddsonic's provider-independent architecture?
- Can it remain understandable and discoverable in a terminal UI?
- Does it introduce significant background machinery, configuration, or operational complexity?
- Could the same benefit be achieved with a smaller design?
- Would removing this feature make ddsonic feel meaningfully worse?

Complexity is a cost, not a sign of maturity.

---

## Architectural Direction

### Music should belong to the user, not the provider

The long-term conceptual model should increasingly treat Spotify, YouTube Music, local files, and possible future providers as **sources of music**, rather than allowing provider boundaries to define the user's library.

Ideally:

- an album is an album;
- an artist is an artist;
- a track is a track;
- a playlist is a playlist;

regardless of where ddsonic can obtain the playable content.

Providers then become implementations capable of satisfying some subset of those objects.

This direction could eventually enable:

- cross-provider playlists;
- source-independent favorites;
- local-first playback resolution;
- graceful provider substitution;
- unified search and discovery;
- richer metadata assembled from multiple sources.

This is a direction, not a requirement to build a giant universal music database.

Radio may reasonably remain a somewhat separate concept because its semantics differ from catalog-based music.

---

## Ideas

### Track Info / Metadata Redesign


**Status:** post-1.0 investigation/design item; unscheduled, not an approved implementation specification.

**Motivation:** long metadata values beside the album artwork can be truncated at the terminal's right edge, with no value scrolling or other way to expose the hidden content. The existing vertical field-list scrolling does not solve this. Investigate the information architecture and responsive layout, not just adding marquee scrolling.

**Investigation scope:**

- Inventory metadata available from Local files, Spotify, YouTube Music and Internet Radio where applicable. Distinguish what each source can supply from what ddsonic currently reads, stores and displays; record missing or conditional fields rather than assuming parity.
- Extend that inventory as additional providers are investigated or restored after 1.0, including Apple Music and inherited providers. This does not approve adding or restoring a provider.
- Determine the common, source-agnostic Track Info core and which richer fields should appear opportunistically when a provider or metadata-enrichment source supplies them.
- Investigate field priority, grouping and placement beside, below or elsewhere around the album artwork.
- Compare additional horizontal space, wrapping, truncation, marquee/scrolling values, adaptive relocation of fields, and expandable or secondary detail presentation. Do not assume marquee scrolling is preferred; consider how users reach the full value under each option.
- Test candidate designs across narrow, typical, wide and very wide terminals, resizing between them, and different album-art sizes, including absent or unsupported artwork. Reuse the responsive-layout size matrix as a starting point, not a fixed design constraint.
- Include unusually long classical titles, works, movements, composer/performer credits, orchestra, conductor, soloists and similarly verbose metadata, alongside sparse metadata.

**Architectural relationships (reuse these workstreams rather than duplicate them):**

- Provider capability modeling and source abstraction: [Capability-Based Provider Architecture](#capability-based-provider-architecture), building on [M4](../../plan.md#m4-multi-provider-expansion), [capability-driven menus](../../plan.md#capability-driven-menus-m41) and the [architecture invariants](../../plan.md#architecture-invariants). Capability decisions belong in the adapters, not provider-specific view branches; the metadata inventory should inform the shared model.
- Metadata enrichment: [Metadata Enrichment](#metadata-enrichment), building on [M4's YouTube Music plan](../../plan.md#m4-implementation-plan-youtube-music-v04), [YouTube enrichment](youtube.md#albums-and-artists-enrichment) and the [catalog](catalog.md). Reuse enrichment results; this item does not independently authorize enrichment or metadata correction.
- Full provider/library unification and source abstraction: [music should belong to the user, not the provider](#music-should-belong-to-the-user-not-the-provider). Keep the Track Info presentation aligned with that shared music model without defining a parallel one.
- Classical music modeling: [Classical Music Metadata](#classical-music-metadata); use its work/movement and performer relationships when available, without deciding the underlying schema in this UI investigation.
- Responsive terminal UI and artwork: [M9](../../plan.md#m9-implementation-plan-responsive-layout-v09), [layout notes](layout.md), [M10](../../plan.md#m10-implementation-plan-album-artwork-in-track-info-v010) and [artwork layout](artwork.md#layout). Investigate changes to field/artwork geometry within this item; protocol and library coverage belongs to [Album Artwork Across More Terminals](#album-artwork-across-more-terminals).
- Additional providers: [Additional Providers](#additional-providers) and [Apple Music Provider](#apple-music-provider) remains the home for provider expansion; this investigation covers how their metadata fits the common presentation.

**Complexity guardrail:** ddsonic models music first and sources second. Richer information should be available when useful without making Track Info provider-specific or visually cluttered. Prioritize comprehension over exposing every field a provider returns: more capable without becoming more complicated.

**Investigation output:** a source/field inventory, candidate layouts and overflow tradeoffs tested against the size/artwork/metadata cases above, and a recommendation with open decisions for review. Field selection, layout, interaction and implementation remain undecided; nothing here changes the 1.0 feature freeze.

---

### Album Artwork Across More Terminals

Investigate whether an alternative terminal-image library, or extending the current approach, could display album artwork on more terminal types. Consolidates the existing Sixel/iTerm2/other-artwork-protocol backlog item; this is a post-1.0 feasibility investigation, not approval to replace a library or promise terminal support.

Use the [current artwork architecture](artwork.md) and [M10 findings](../../plan.md#m10-implementation-plan-album-artwork-in-track-info-v010) as the baseline. Reassess library maturity when investigating; historical library assessments are not permanent exclusions.

Compare candidates on:

- actual terminal/protocol coverage, including Sixel, iTerm2 and candidates such as WezTerm, Konsole and foot, while retaining the existing Kitty/Ghostty behavior;
- integration with Bubble Tea's cell-based rendering, resizing, placement and reliable cleanup without image artifacts;
- capability detection, behavior through tmux, and a clean metadata-only fallback when graphics are unavailable;
- image quality, performance, dependency size, licensing, maintenance and implementation complexity.

Produce a tested terminal/protocol compatibility matrix and a recommendation comparing alternative libraries with extending or retaining the current implementation. Do not assume a library change alone guarantees broader support. Coordinate geometry and fallback behavior with [Track Info / Metadata Redesign](#track-info--metadata-redesign), keeping protocol/library selection in this investigation.

---

### Metadata Enrichment

Existing foundation: [YouTube enrichment](youtube.md#albums-and-artists-enrichment) already supplies catalog metadata. Investigate broader coverage and provenance without replacing that work by default. Track Info presentation is covered [separately](#track-info--metadata-redesign).

Investigate automatically enriching incomplete metadata supplied by local files or providers.

Examples include:

- missing album art;
- incomplete release information;
- composer/work/movement metadata;
- inconsistent genres;
- missing artist or album relationships.

Prefer **automated, read-only enrichment** over turning ddsonic into a tag editor.

Enrichment should:

- never silently overwrite source files;
- never modify provider-owned metadata;
- retain provenance where practical;
- distinguish authoritative source metadata from inferred/enriched metadata;
- account for confidence when matching records.

An open design question is whether enriched metadata should be:

1. persisted in ddsonic's database,
2. cached with expiration/refresh behavior, or
3. resolved dynamically.

Do not decide this merely because one implementation is easiest.

---

### Classical Music Metadata

Investigate first-class representation of classical music.

The normal artist → album → track model poorly represents:

- composers;
- works;
- movements;
- conductors;
- orchestras;
- soloists;
- catalog numbers;
- compositions spanning multiple tracks.

A useful model might recognize a work such as a symphony or concerto as an object containing movements while retaining compatibility with providers that expose those movements as ordinary tracks.

Avoid designing a musicological database for its own sake. The goal is to make actual listening, browsing, searching, and queueing better.

---

### Genre Normalization

Provider and local-file genre metadata can be inconsistent, overly specific, contradictory, or absent.

Investigate a normalized genre layer that can map source metadata into useful ddsonic categories without destroying the original metadata.

Possible uses include:

- browsing;
- searching;
- generated playlists;
- rediscovery;
- automatic EQ selection.

Normalization should be reversible or retain source provenance.

---

### Automatic Genre-Based EQ

Consolidates the existing "Auto EQ from normalized genre metadata" backlog item; depends on [Genre Normalization](#genre-normalization).

If useful genre normalization exists, investigate automatically selecting EQ profiles based on the current music.

This should remain predictable and overrideable.

Avoid behavior where playback suddenly sounds different for reasons the user cannot discover.

---

### Generated / Random Playlists

Explore richer ways to generate listening sessions from the unified library.

Potential dimensions include:

- genre;
- artist;
- era;
- neglected music;
- favorites;
- recently added music;
- listening history;
- combinations of those criteria.

The goal should be useful rediscovery rather than building a recommendation engine merely because recommendation engines exist.

---

### Listening History and Rediscovery

Investigate maintaining enough listening history to enable useful personal-library features.

Examples:

- albums not played in a long time;
- forgotten favorites;
- recently discovered artists;
- frequently skipped material;
- music added but never played.

Prefer transparent rules over opaque recommendation algorithms.

---

### Smarter Queue

Investigate ways the queue can become more useful without becoming unpredictable.

Possibilities include:

- inserting related material;
- continuing playback after the explicit queue ends;
- queue generation from albums, artists, genres, or playlists;
- better manipulation of upcoming items.

The user should always be able to understand why something is playing.

---

### Library Health

Investigate tools for identifying problems in the unified catalog.

Examples:

- missing local files;
- provider items that are no longer available;
- duplicate or suspicious matches;
- unresolved playlist entries;
- incomplete metadata;
- stale cached provider information.

This may become especially useful as ddsonic's source-independent catalog grows.

---

### Offline Awareness

Investigate explicitly representing whether music is currently playable without network access.

This could allow ddsonic to:

- distinguish local and remote availability;
- filter/search for offline-playable music;
- avoid repeatedly attempting unavailable providers;
- make sensible source-resolution decisions while disconnected.

---

### Graceful Provider Failover

If ddsonic knows that multiple providers can satisfy the same logical recording, investigate resolving playback through another source when the preferred source is unavailable.

For example, a track requested through a provider-backed playlist might also exist locally.

Any failover system must be conservative about track identity. Playing the wrong recording is worse than reporting that the requested track is unavailable.

---

### Portable / Cross-Provider Playlists

Consolidates the existing "Cross-source, ddsonic-owned playlists" backlog item; relate identity and resolution to [source abstraction](#music-should-belong-to-the-user-not-the-provider) and [provider failover](#graceful-provider-failover).

Investigate playlists whose identity belongs to ddsonic rather than a specific provider.

A playlist could contain logical music references that resolve at playback time through available providers.

This could allow one playlist to contain music sourced from:

- local files;
- Spotify;
- YouTube Music;
- future providers.

Provider-native playlists should still remain usable without forcing conversion into ddsonic-native playlists.

---

### Generated Noise Source

Together with [Ambient / Nature Audio](#ambient--nature-audio), this expands the existing "Ambient or generated sounds" backlog item into two related investigations.

Investigate a lightweight source capable of generating continuous audio such as:

- white noise;
- pink noise;
- brown noise;
- simple ambient noise.

This may be useful for sleep, concentration, or background listening.

Keep this implementation small. ddsonic should not become an audio synthesis workstation.

---

### Ambient / Nature Audio

Investigate optional access to appropriately licensed ambient material such as:

- rain;
- streams;
- forests;
- ocean;
- similar soundscapes.

Prefer generated audio or clearly licensed/CC0 sources.

Avoid introducing licensing uncertainty or maintaining a large bundled audio collection.

---

### Additional Providers

Preserve the existing selection constraint: only providers the owner uses and can test; inherited providers hidden for 1.0 are candidates, not promised restorations. Include metadata capabilities in the [Track Info inventory](#track-info--metadata-redesign).

Continue evaluating providers that make sense within ddsonic's architecture.

Provider additions should not require provider-specific behavior to leak throughout the application.

The usefulness of a provider should be weighed against:

- API stability;
- authentication complexity;
- playback restrictions;
- maintenance burden;
- architectural exceptions required.

---

### Apple Music Provider

Investigate whether Apple Music can realistically be supported as a provider.

This is specifically a **feasibility investigation**, not a promised provider.

Questions include:

- API accessibility;
- authentication;
- playback restrictions;
- DRM;
- Linux compatibility;
- whether playback can fit ddsonic's existing architecture without unreasonable hacks.

A legitimate result of the investigation may be **do not implement**.

---

### Three-State Keymap / Key-Hint Display

**Status:** post-1.0 UX investigation; unscheduled, not a committed feature or roadmap item.

Some screens expose enough actions that the keybinding footer consumes substantial vertical space. Investigate extending the existing on/off keymap toggle into three display modes:

- **Minimal:** deliberately selected important/common controls for the current screen, preserving discoverability while reclaiming list space.
- **Full:** the complete set of relevant keybindings currently exposed by the screen.
- **Hidden:** no keybinding footer/help information.

The existing toggle key could cycle through `Minimal → Full → Hidden → Minimal`. Evaluate whether this order is intuitive or another order works better; it is not fixed.

**Default behavior:** compare Minimal's useful guidance and smaller footprint with Full's maximum discoverability for new users. Minimal appears attractive because complex screens would no longer devote several rows to keybinding documentation while still showing enough to operate the interface. Recording this idea does not decide the default.

**Defining Minimal:** each screen should deliberately select its essential actions, rather than take the first N bindings or truncate the Full list. Where applicable, candidates include navigation, play/select/open, back, search and the key that changes or expands the key-hint display. Less common, advanced or contextual actions remain available in Full. Investigate whether existing action/keybinding metadata can express this selection without maintaining duplicated lists or keybinding definitions.

**Persistence and scope:** investigate whether the selected mode should apply globally across screens, persist across sessions and have a configurable default. A global user preference seems desirable, but validate it against the existing UI/configuration architecture. Individual screens may still define which actions qualify as Minimal.

**Relationship to the [Searchable Command Palette](#searchable-command-palette):** a coherent future discoverability model could offer:

- Minimal key hints for everyday actions immediately visible;
- Full key hints for all relevant shortcuts on the current screen;
- a command palette for searchable access to available commands/actions.

If a common command/action registry shared by hotkeys, the command palette and CLI/IPC control is developed, investigate deriving key hints from the same action metadata to avoid independent descriptions of the same actions. Command-palette work is not a prerequisite if three-state key hints can be implemented cleanly on their own.

**Design goals:** preserve useful discoverability, reclaim vertical space, select essential actions by screen, cycle predictably and behave consistently across screens. Reuse existing metadata where practical. Avoid arbitrary truncation, hiding essential navigation from inexperienced users, duplicated definitions and significant architecture for a small UX improvement.

**Investigation output:** screen-specific essential-action selections and a recommendation on cycling order, default mode, preference scope/persistence and metadata reuse. These decisions and implementation remain open; current keymap behavior is unchanged.

---

### Alphabetical Radio Station Sorting

**Status:** post-1.0 UX improvement; unscheduled, not part of the 1.0 release scope. Recorded only: nothing is implemented and current behavior is unchanged.

Radio station lists currently appear in an unintuitive or effectively arbitrary order. After 1.0, present stations sorted alphabetically by their displayed station name.

**Desired behavior:**

- Sorting is deterministic.
- Sort by the user-visible station name.
- Sorting is case-insensitive, unless the existing UI has an established sorting convention that should be followed instead.
- Apply the behavior consistently anywhere ddsonic presents a comparable list of radio stations, where appropriate.
- Backend, database or provider retrieval order must not determine the visible ordering accidentally.

**Before implementation:** verify whether any radio view currently has an intentional ordering semantic that should be preserved, such as favorites with explicit user-defined ordering. If such a case exists, document it rather than blindly replacing it.

---

## Architectural Investigations

### Capability-Based Provider Architecture

Extend the existing [M4 capability-driven menus](../../plan.md#capability-driven-menus-m41) and [architecture invariants](../../plan.md#architecture-invariants), rather than treating capability detection as new. Include metadata availability as well as operations; adapters retain capability decisions outside the view layer.

Investigate formalizing provider capabilities.

Rather than assuming every provider supports every operation, providers could advertise capabilities such as:

- search;
- streaming;
- library browsing;
- favorites;
- playlists;
- playlist modification;
- metadata lookup;
- offline/local playback.

The UI and command layer could then expose functionality based on capabilities rather than provider-specific conditionals.

This could substantially simplify future provider additions.

---

### Searchable Command Palette

Investigate an in-TUI searchable command palette.

A hotkey could open a menu of available actions with fuzzy/search filtering.

Goals:

- improve discoverability;
- reduce dependence on memorized hotkeys;
- avoid permanently cluttering the terminal UI;
- expose less-frequently-used actions without burying them.

Longer term, consider whether:

- hotkeys,
- the command palette, and
- the public CLI/IPC control interface

should consume a common **command/action registry**.

This could prevent the same action from being independently implemented three different ways.

Related UX investigation: [Three-State Keymap / Key-Hint Display](#three-state-keymap--key-hint-display) could complement the palette and reuse common action metadata, without depending on palette implementation.

---

### Daemon / Persistent Playback

Consolidates the existing daemon/headless backlog idea, previously described as likely a 2.0 architectural project. That description is a scale estimate, not a release commitment; the inherited daemon surface remains outside 1.0.

Investigate whether playback should eventually be able to survive the lifetime of the TUI.

Potential benefits include:

- closing/restarting the interface without interrupting music;
- external control;
- better integration with desktop/media controls;
- multiple control clients.

However, this introduces significant architectural complexity.

Questions include:

- process lifecycle;
- IPC;
- ownership of playback state;
- configuration reload behavior;
- upgrades;
- crash recovery;
- debugging;
- cross-platform behavior.

Do not implement a daemon merely because mature music players often have one.

A valid outcome is that keeping playback inside the TUI remains the better design.

---

### Multi-Device Synchronization

Investigate whether any useful form of synchronization across ddsonic installations is worth supporting.

Possible synchronized state might include:

- playlists;
- favorites;
- listening history;
- configuration;
- library metadata.

Actual synchronized playback is a substantially different and harder problem and should not be implied by basic state synchronization.

This area carries a high risk of turning ddsonic into a distributed application. Require a compelling use case before introducing that complexity.

---

### Long-Term cliamp Independence

Related existing decision: [module naming and go install](../../plan.md#post-10-backlog) stays deferred while upstream merge compatibility earns its cost. This broader investigation does not authorize a module rename or supersede the already-recorded first-1.1 upstream merge.

Investigate whether ddsonic should eventually reduce or eliminate its architectural dependence on upstream cliamp.

ddsonic began as a cliamp fork, but its goals and architecture may eventually diverge enough that inherited structures become liabilities rather than useful foundations.

The investigation should include:

- identifying inherited subsystems still closely modeled on cliamp;
- determining which remain appropriate for ddsonic;
- identifying compatibility constraints that no longer provide value;
- evaluating opportunities to simplify or modernize inherited code;
- deciding whether continued upstream conceptual compatibility provides meaningful benefits.

This does **not** imply a rewrite.

Prefer incremental replacement or refactoring when justified.

The goal is ddsonic architectural ownership, not independence for symbolic reasons.

---

## Existing Deferred Work

These items already have a home in [plan.md's post-1.0 backlog](../../plan.md#post-10-backlog); this index preserves visibility without creating parallel specifications. Their recorded decisions and conditions remain in force. In particular, the full upstream merge is already designated the first 1.1 work; the brainstorming status of new ideas here does not unschedule it.

- InnerTube discovery of saved YouTube Music playlists.
- Clear the whole queue with `X`, with Ctrl+Z undo; related to [Smarter Queue](#smarter-queue).
- YouTube cookie playlist count/completeness check.
- Broad-prefix search speed, only if it becomes a complaint.
- Linux arm64, macOS, Homebrew and `.deb`/`.rpm`, once someone can test them.
- Module naming and `go install`; related to [Long-Term cliamp Independence](#long-term-cliamp-independence).
- Full upstream merge of the latest release tag, not `main`, as the first 1.1 work; preserve [upstream-triage decisions](upstream-triage.md).
- TOML inline comments.
- A ddsonic ASCII-duck visualizer.
- Nix packaging written and tested for ddsonic.
- A properly licensed replacement for `xlab/vorbis-go`, including feasibility of `jfreymuth/oggvorbis` and go-librespot changes.

Also retain [plan.md's open questions](../../plan.md#open-questions): separating Local's scan folder from the file browser's start folder, and catalog/offline artwork storage. The existing [artwork cache](artwork.md) already stores downloaded images; clarify any additional offline guarantee before treating artwork caching as new work.

Release findings remain in their release records: [rc.1 carried findings](releases/v1.0.0-rc.1.md) and [rc.3's deferred carriage-return install-path issue](releases/v1.0.0-rc.3.md). This catalog does not change their severity, waivers or release gates.

---

## Explicit Non-Commitments

The following distinction is important:

**An idea appearing in this document does not mean it will be implemented.**

In particular, this document does not commit ddsonic to:

- Apple Music support;
- daemon mode;
- multi-device synchronization;
- provider failover;
- a metadata service;
- classical-specific database architecture;
- automatic recommendations;
- synchronized playback;
- severing all cliamp-derived architecture;
- any particular post-1.0 release schedule.

Some investigations should end with a deliberate decision **not to build the feature**.

That is a successful outcome.

---

## Evaluation Framework

Before promoting one of these ideas into the roadmap, answer:

1. **User value:** What concrete listening problem does this solve?
2. **Architectural fit:** Does it reinforce or fight ddsonic's existing design?
3. **Complexity:** What permanent machinery must exist afterward?
4. **Discoverability:** Can users understand and find the feature?
5. **Predictability:** Can users understand what ddsonic is doing?
6. **Maintenance:** What external APIs, services, protocols, or data must remain functional?
7. **Failure behavior:** What happens when the feature or dependency breaks?
8. **Smaller alternative:** Is there an 80% solution with 20% of the architecture?
9. **Exit cost:** Could the feature be removed later without destabilizing the application?
10. **Identity:** Does this still feel like ddsonic?

---

## Suggested Post-1.0 Approach

Do not treat this document as the 1.1 backlog.

After 1.0:

1. Gather actual user experience with the released application.
2. Fix defects and obvious usability problems first.
3. Identify architectural friction revealed by real usage.
4. Select a small number of investigations from this document.
5. Prototype when uncertainty is high.
6. Promote only validated ideas into the roadmap.
7. Continue aggressively rejecting unnecessary complexity.

The best post-1.0 ddsonic is not necessarily the version with the most features.

It is the version where the music player feels increasingly coherent while its internals become easier, rather than harder, to understand.
