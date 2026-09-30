package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

// maxErrorLen caps a stored sync error message.
const maxErrorLen = 500

// ApplySnapshot implements catalog.Writer.
func (s *Store) ApplySnapshot(ctx context.Context, snap catalog.Snapshot) error {
	if snap.Provider == "" || snap.Collection == "" {
		return errors.New("apply snapshot: provider and collection are required")
	}
	tx, err := s.wdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("apply snapshot %s/%s: begin: %w", snap.Provider, snap.Collection, err)
	}
	defer tx.Rollback() // no-op after Commit

	w := newWriter(ctx, tx, snap.Provider)
	w.snap, w.exact = &snap, snap.Files
	if err := w.apply(); err != nil {
		return fmt.Errorf("apply snapshot %s/%s: %w", snap.Provider, snap.Collection, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply snapshot %s/%s: commit: %w", snap.Provider, snap.Collection, err)
	}
	return nil
}

// RecordSyncSuccess implements catalog.Writer.
func (s *Store) RecordSyncSuccess(ctx context.Context, provider, collection string) error {
	now := time.Now().UnixMilli()
	_, err := s.wdb.ExecContext(ctx, `INSERT INTO sync_state (provider, collection, last_attempt_at, last_success_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (provider, collection) DO UPDATE SET
			last_attempt_at = excluded.last_attempt_at, last_success_at = excluded.last_success_at, last_error = ''`,
		provider, collection, now, now)
	if err != nil {
		return fmt.Errorf("record sync success %s/%s: %w", provider, collection, err)
	}
	return nil
}

// RecordSyncFailure implements catalog.Writer.
func (s *Store) RecordSyncFailure(ctx context.Context, provider, collection string, cause error) error {
	msg := "unknown error"
	if cause != nil {
		msg = cause.Error()
	}
	// Status is for display; keep provider error bodies from growing the row.
	if len(msg) > maxErrorLen {
		msg = msg[:maxErrorLen] + "…"
	}
	_, err := s.wdb.ExecContext(ctx, `INSERT INTO sync_state (provider, collection, last_attempt_at, last_error)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (provider, collection) DO UPDATE SET
			last_attempt_at = excluded.last_attempt_at, last_error = excluded.last_error`,
		provider, collection, time.Now().UnixMilli(), msg)
	if err != nil {
		return fmt.Errorf("record sync failure %s/%s: %w", provider, collection, err)
	}
	return nil
}

// PlaylistSnapshots implements catalog.Writer.
func (s *Store) PlaylistSnapshots(ctx context.Context, provider string) (map[string]string, error) {
	type row struct{ id, snapshot string }
	rows, err := queryAll(ctx, s.wdb, func(r *sql.Rows) (v row, err error) {
		err = r.Scan(&v.id, &v.snapshot)
		return v, err
	}, `SELECT provider_id, snapshot FROM playlists WHERE provider = ?`, provider)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.id] = r.snapshot
	}
	return out, nil
}

// CacheAlbumTracks implements catalogsync.AlbumStore. It writes an album's
// complete track list and marks the album cached, in one transaction. The
// album must already be in the catalog; the tracks are attached to it
// whatever album they name.
func (s *Store) CacheAlbumTracks(ctx context.Context, album catalog.Ref, tracks []catalog.TrackRecord) error {
	tx, err := s.wdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cache album %s: begin: %w", album.ProviderID, err)
	}
	defer tx.Rollback()
	w := newWriter(ctx, tx, album.Provider)
	if err := w.albumTracks(album, tracks); err != nil {
		return fmt.Errorf("cache album %s: %w", album.ProviderID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cache album %s: commit: %w", album.ProviderID, err)
	}
	return nil
}

// snapWriter writes provider records inside one transaction: a whole
// snapshot, or one album's track list.
type snapWriter struct {
	ctx      context.Context
	tx       *sql.Tx
	provider string            // every record's provider
	snap     *catalog.Snapshot // set when applying a snapshot
	// exact writes records as they are, blanks included: a local index is
	// the files' whole truth, so a cleared tag must clear. Other records may
	// be partial, and their zero values keep what is stored.
	exact bool
	now   int64 // Unix ms
	gen   int64

	// Artists already written by this snapshot; many tracks share one.
	artistIDs map[catalog.Ref]int64
	// Albums written from the snapshot's own album list, whose tracks may
	// reuse the ID. An album seen only through a track is not cached here:
	// a later, fuller record of it must still land.
	listedAlbums map[catalog.Ref]int64
}

func newWriter(ctx context.Context, tx *sql.Tx, provider string) *snapWriter {
	return &snapWriter{
		ctx: ctx, tx: tx, provider: provider, now: time.Now().UnixMilli(),
		artistIDs: map[catalog.Ref]int64{}, listedAlbums: map[catalog.Ref]int64{},
	}
}

// merge is the upsert value of col: the new value when writing exactly,
// otherwise the stored value wherever the new one is zero. col and zero are
// constants from this file.
func (w *snapWriter) merge(col, zero string) string {
	if w.exact {
		return "excluded." + col
	}
	return "COALESCE(NULLIF(excluded." + col + ", " + zero + "), " + col + ")"
}

func (w *snapWriter) exec(query string, args ...any) error {
	_, err := w.tx.ExecContext(w.ctx, query, args...)
	return err
}

func (w *snapWriter) queryID(query string, args ...any) (int64, error) {
	var id int64
	err := w.tx.QueryRowContext(w.ctx, query, args...).Scan(&id)
	return id, err
}

// ref fills in the writer's provider when a record leaves it empty, and
// rejects records the writer cannot own: an entity of another provider
// would escape both providers' sweeps.
func (w *snapWriter) ref(r catalog.Ref) (catalog.Ref, error) {
	if r.Provider == "" {
		r.Provider = w.provider
	}
	if r.Provider != w.provider {
		return r, fmt.Errorf("record of provider %q written as %q", r.Provider, w.provider)
	}
	if r.ProviderID == "" {
		return r, errors.New("record without a provider ID")
	}
	return r, nil
}

func (w *snapWriter) apply() error {
	var err error
	w.gen, err = w.queryID(`INSERT INTO sync_state (provider, collection, generation) VALUES (?, ?, 1)
		ON CONFLICT (provider, collection) DO UPDATE SET generation = generation + 1
		RETURNING generation`, w.snap.Provider, w.snap.Collection)
	if err != nil {
		return fmt.Errorf("next generation: %w", err)
	}

	add := func(kind catalog.Kind, what string, id int64, err error, addedAt time.Time) error {
		if err == nil {
			err = w.member(kind, id, addedAt)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	}
	for _, a := range w.snap.Albums {
		id, err := w.album(a)
		if err := add(catalog.KindAlbum, "album "+a.Ref.ProviderID, id, err, a.AddedAt); err != nil {
			return err
		}
		if ref, err := w.ref(a.Ref); err == nil {
			w.listedAlbums[ref] = id
		}
	}
	for _, a := range w.snap.Artists {
		id, err := w.artist(a)
		if err := add(catalog.KindArtist, "artist "+a.Ref.ProviderID, id, err, time.Time{}); err != nil {
			return err
		}
	}
	for _, t := range w.snap.Tracks {
		id, err := w.track(t)
		if err := add(catalog.KindTrack, "track "+t.Ref.ProviderID, id, err, t.AddedAt); err != nil {
			return err
		}
	}
	for _, p := range w.snap.Playlists {
		id, err := w.playlist(p)
		if err := add(catalog.KindPlaylist, "playlist "+p.Ref.ProviderID, id, err, p.AddedAt); err != nil {
			return err
		}
	}

	if w.snap.Files {
		if err := w.reconcileFiles(); err != nil {
			return fmt.Errorf("reconcile files: %w", err)
		}
	}

	// Reconcile: this collection's members the fetch did not see are gone.
	if err := w.exec(`DELETE FROM library_items WHERE provider = ? AND collection = ? AND last_seen_gen < ?`,
		w.snap.Provider, w.snap.Collection, w.gen); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	if err := w.exec(`UPDATE sync_state SET last_attempt_at = ?, last_success_at = ?, last_error = ''
		WHERE provider = ? AND collection = ?`, w.now, w.now, w.snap.Provider, w.snap.Collection); err != nil {
		return fmt.Errorf("record success: %w", err)
	}
	return nil
}

func (w *snapWriter) member(kind catalog.Kind, itemID int64, addedAt time.Time) error {
	added, explicit := w.now, !addedAt.IsZero()
	if explicit {
		added = addedAt.UnixMilli()
	}
	return w.exec(`INSERT INTO library_items (provider, collection, kind, item_id, added_at, last_seen_gen)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, collection, kind, item_id) DO UPDATE SET
			last_seen_gen = excluded.last_seen_gen,
			added_at = CASE WHEN ? THEN excluded.added_at ELSE added_at END`,
		w.snap.Provider, w.snap.Collection, kind, itemID, added, w.gen, explicit)
}

func (w *snapWriter) artist(a catalog.ArtistRecord) (int64, error) {
	ref, err := w.ref(a.Ref)
	if err != nil {
		return 0, err
	}
	if id, ok := w.artistIDs[ref]; ok {
		return id, nil
	}
	id, err := w.queryID(`INSERT INTO artists (provider, provider_id, name, sort_name, image_url, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, provider_id) DO UPDATE SET
			name = excluded.name, sort_name = excluded.sort_name,
			image_url = COALESCE(NULLIF(excluded.image_url, ''), image_url),
			updated_at = excluded.updated_at
		RETURNING id`, ref.Provider, ref.ProviderID, a.Name, catalog.SortKey(a.Name), a.ImageURL, w.now)
	if err != nil {
		return 0, fmt.Errorf("upsert artist %s: %w", ref.ProviderID, err)
	}
	w.artistIDs[ref] = id
	return id, nil
}

// credit writes artists and returns their IDs and joined display credit.
func (w *snapWriter) credit(artists []catalog.ArtistRecord) ([]int64, string, error) {
	ids := make([]int64, len(artists))
	names := make([]string, len(artists))
	for i, a := range artists {
		id, err := w.artist(a)
		if err != nil {
			return nil, "", err
		}
		ids[i], names[i] = id, a.Name
	}
	return ids, strings.Join(names, ", "), nil
}

func (w *snapWriter) album(a catalog.AlbumRecord) (int64, error) {
	ref, err := w.ref(a.Ref)
	if err != nil {
		return 0, err
	}
	artistIDs, credit, err := w.credit(a.Artists)
	if err != nil {
		return 0, err
	}
	sortArtist := ""
	switch {
	case a.Credit != "":
		credit, sortArtist = a.Credit, catalog.SortKey(a.Credit)
	case len(a.Artists) > 0:
		sortArtist = catalog.SortKey(a.Artists[0].Name)
	}
	// Zero values are unknown unless writing exactly (see merge), here and
	// in the track upsert.
	id, err := w.queryID(`INSERT INTO albums (provider, provider_id, title, sort_title, artist_credit, sort_artist,
			year, track_count, artwork_url, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, provider_id) DO UPDATE SET
			title = excluded.title, sort_title = excluded.sort_title,
			artist_credit = `+w.merge("artist_credit", "''")+`,
			sort_artist = `+w.merge("sort_artist", "''")+`,
			year = `+w.merge("year", "0")+`,
			track_count = `+w.merge("track_count", "0")+`,
			artwork_url = `+w.merge("artwork_url", "''")+`,
			updated_at = excluded.updated_at
		RETURNING id`, ref.Provider, ref.ProviderID, a.Title, catalog.SortKey(a.Title), credit, sortArtist,
		a.Year, a.TrackCount, a.ArtworkURL, w.now)
	if err != nil {
		return 0, fmt.Errorf("upsert album %s: %w", ref.ProviderID, err)
	}
	if len(artistIDs) > 0 {
		if err := w.replaceCredits("album_artists", "album_id", id, artistIDs); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (w *snapWriter) track(t catalog.TrackRecord) (int64, error) {
	var albumID sql.NullInt64
	if t.Album != nil {
		id, err := w.trackAlbum(*t.Album)
		if err != nil {
			return 0, err
		}
		albumID = sql.NullInt64{Int64: id, Valid: true}
	}
	id, err := w.writeTrack(t, albumID)
	if err != nil || t.File == nil {
		return id, err
	}
	f := t.File
	if err := w.exec(`INSERT INTO local_files (path, size, mtime_ns, track_id) VALUES (?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET
			size = excluded.size, mtime_ns = excluded.mtime_ns, track_id = excluded.track_id`,
		f.Path, f.Size, f.MTimeNS, id); err != nil {
		return 0, fmt.Errorf("index file %s: %w", f.Path, err)
	}
	return id, nil
}

// trackAlbum returns a track's album ID, reusing one this snapshot listed.
func (w *snapWriter) trackAlbum(a catalog.AlbumRecord) (int64, error) {
	if ref, err := w.ref(a.Ref); err == nil {
		if id, ok := w.listedAlbums[ref]; ok {
			return id, nil
		}
	}
	return w.album(a)
}

// reconcileFiles drops the file index rows of files the snapshot did not
// list: they were deleted or moved.
func (w *snapWriter) reconcileFiles() error {
	keep := make(map[string]bool, len(w.snap.Tracks))
	for _, t := range w.snap.Tracks {
		if t.File != nil {
			keep[t.File.Path] = true
		}
	}
	rows, err := w.tx.QueryContext(w.ctx, `SELECT path FROM local_files`)
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		if !keep[path] {
			gone = append(gone, path)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, path := range gone {
		if err := w.exec(`DELETE FROM local_files WHERE path = ?`, path); err != nil {
			return err
		}
	}
	return nil
}

// writeTrack upserts a track on albumID, ignoring t.Album.
func (w *snapWriter) writeTrack(t catalog.TrackRecord, albumID sql.NullInt64) (int64, error) {
	ref, err := w.ref(t.Ref)
	if err != nil {
		return 0, err
	}
	artistIDs, credit, err := w.credit(t.Artists)
	if err != nil {
		return 0, err
	}
	// An enriched track keeps the title and credits its enrichment found;
	// a sync knows only the video's.
	var id int64
	var enriched bool
	err = w.tx.QueryRowContext(w.ctx, `INSERT INTO tracks (provider, provider_id, title, artist_credit, album_id, album_title,
			disc, track_no, duration_ms, playable_uri, genre, year, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, provider_id) DO UPDATE SET
			title = CASE WHEN enriched_at IS NULL THEN excluded.title ELSE title END,
			artist_credit = CASE WHEN enriched_at IS NULL THEN `+w.merge("artist_credit", "''")+` ELSE artist_credit END,
			album_id = COALESCE(excluded.album_id, album_id),
			album_title = `+w.merge("album_title", "''")+`,
			disc = `+w.merge("disc", "0")+`,
			track_no = `+w.merge("track_no", "0")+`,
			duration_ms = `+w.merge("duration_ms", "0")+`,
			playable_uri = `+w.merge("playable_uri", "''")+`,
			genre = `+w.merge("genre", "''")+`,
			year = `+w.merge("year", "0")+`,
			updated_at = excluded.updated_at
		RETURNING id, enriched_at IS NOT NULL`, ref.Provider, ref.ProviderID, t.Title, credit, albumID, t.AlbumTitle,
		t.Disc, t.TrackNo, t.Duration.Milliseconds(), t.PlayableURI, t.Genre, t.Year, w.now).Scan(&id, &enriched)
	if err != nil {
		return 0, fmt.Errorf("upsert track %s: %w", ref.ProviderID, err)
	}
	if len(artistIDs) > 0 && !enriched {
		if err := w.replaceCredits("track_artists", "track_id", id, artistIDs); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// albumTracks writes an album's track list onto the stored album and marks
// it cached. Tracks without a year take the album's.
func (w *snapWriter) albumTracks(album catalog.Ref, tracks []catalog.TrackRecord) error {
	ref, err := w.ref(album)
	if err != nil {
		return err
	}
	var id int64
	var year int
	err = w.tx.QueryRowContext(w.ctx, `SELECT id, year FROM albums WHERE provider = ? AND provider_id = ?`,
		ref.Provider, ref.ProviderID).Scan(&id, &year)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find album: %w", err)
	}
	albumID := sql.NullInt64{Int64: id, Valid: true}
	for _, t := range tracks {
		if t.Year == 0 {
			t.Year = year
		}
		if _, err := w.writeTrack(t, albumID); err != nil {
			return err
		}
	}
	return w.exec(`UPDATE albums SET tracks_cached_at = ?,
			track_count = CASE WHEN track_count = 0 THEN ? ELSE track_count END
		WHERE id = ?`, w.now, len(tracks), id)
}

// replaceCredits rewrites an album's or track's artist credits in order.
// table and column are constants from this file, never user input.
func (w *snapWriter) replaceCredits(table, column string, id int64, artistIDs []int64) error {
	if err := w.exec(`DELETE FROM `+table+` WHERE `+column+` = ?`, id); err != nil {
		return fmt.Errorf("clear %s: %w", table, err)
	}
	for pos, artistID := range artistIDs {
		// A repeated artist keeps its first position.
		if err := w.exec(`INSERT OR IGNORE INTO `+table+` (`+column+`, artist_id, position) VALUES (?, ?, ?)`,
			id, artistID, pos); err != nil {
			return fmt.Errorf("write %s: %w", table, err)
		}
	}
	return nil
}

func (w *snapWriter) playlist(p catalog.PlaylistRecord) (int64, error) {
	ref, err := w.ref(p.Ref)
	if err != nil {
		return 0, err
	}
	// The change marker only advances with the tracks it describes, so a
	// playlist whose tracks were not fetched is refetched next time.
	snapshot := ""
	if p.TracksFetched {
		snapshot = p.Snapshot
	}
	id, err := w.queryID(`INSERT INTO playlists (provider, provider_id, name, own, snapshot, track_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, provider_id) DO UPDATE SET
			name = excluded.name, own = excluded.own,
			snapshot = CASE WHEN ? THEN excluded.snapshot ELSE snapshot END,
			track_count = excluded.track_count, updated_at = excluded.updated_at
		RETURNING id`, ref.Provider, ref.ProviderID, p.Name, p.Own, snapshot, p.TrackCount, w.now, p.TracksFetched)
	if err != nil {
		return 0, fmt.Errorf("upsert playlist %s: %w", ref.ProviderID, err)
	}
	if !p.TracksFetched {
		return id, nil
	}
	if err := w.exec(`DELETE FROM playlist_tracks WHERE playlist_id = ?`, id); err != nil {
		return 0, fmt.Errorf("clear playlist tracks: %w", err)
	}
	for pos, t := range p.Tracks {
		trackID, err := w.track(t)
		if err != nil {
			return 0, err
		}
		if err := w.exec(`INSERT INTO playlist_tracks (playlist_id, position, track_id) VALUES (?, ?, ?)`,
			id, pos, trackID); err != nil {
			return 0, fmt.Errorf("write playlist track: %w", err)
		}
	}
	return id, nil
}

// Sweep implements catalog.Writer. First, albums that left the library
// drop their cached track lists. Then it deletes provider's entities that
// nothing keeps alive, in dependency order: playlists, then tracks, then
// albums, then artists. What keeps an entity alive:
//
//	playlist: library membership
//	track:    membership, a playlist, the local file index, or an album
//	          whose track list is cached (only library albums are)
//	album:    membership, a cached track list, or a surviving track
//	artist:   membership, or a credit on a surviving album or track
//
// A cached album keeps its whole track list, so the album-track cache is
// never left partial. An album kept only by a surviving track (say, one
// still-liked song) is uncached and fetches its tracks again when opened.
func (s *Store) Sweep(ctx context.Context, provider string) error {
	const members = `SELECT item_id FROM library_items WHERE provider = ? AND kind = ?`
	stmts := []struct {
		query string
		args  []any
	}{
		// An album that left the library drops its cached track list, so
		// the cache follows the library instead of growing forever.
		{`UPDATE albums SET tracks_cached_at = NULL
			WHERE provider = ? AND tracks_cached_at IS NOT NULL AND id NOT IN (` + members + `)`,
			[]any{provider, provider, catalog.KindAlbum}},
		{`DELETE FROM playlists WHERE provider = ? AND id NOT IN (` + members + `)`,
			[]any{provider, provider, catalog.KindPlaylist}},
		{`DELETE FROM tracks WHERE provider = ?
			AND id NOT IN (` + members + `)
			AND id NOT IN (SELECT track_id FROM playlist_tracks)
			AND id NOT IN (SELECT track_id FROM local_files WHERE track_id IS NOT NULL)
			AND (album_id IS NULL OR album_id NOT IN (SELECT id FROM albums WHERE tracks_cached_at IS NOT NULL))`,
			[]any{provider, provider, catalog.KindTrack}},
		{`DELETE FROM albums WHERE provider = ?
			AND id NOT IN (` + members + `)
			AND tracks_cached_at IS NULL
			AND id NOT IN (SELECT album_id FROM tracks WHERE album_id IS NOT NULL)`,
			[]any{provider, provider, catalog.KindAlbum}},
		{`DELETE FROM artists WHERE provider = ?
			AND id NOT IN (` + members + `)
			AND id NOT IN (SELECT artist_id FROM album_artists)
			AND id NOT IN (SELECT artist_id FROM track_artists)`,
			[]any{provider, provider, catalog.KindArtist}},
	}
	tx, err := s.wdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sweep %s: begin: %w", provider, err)
	}
	defer tx.Rollback()
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return fmt.Errorf("sweep %s: %w", provider, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sweep %s: commit: %w", provider, err)
	}
	return nil
}
