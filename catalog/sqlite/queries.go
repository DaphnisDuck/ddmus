package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

// Library list queries filter entities by membership (id IN library_items),
// which lists an item once even when two collections hold it, and read the
// credits denormalized onto albums and tracks, so no row needs a join.

// memberIDs selects the item IDs of a provider's library members of a kind.
const memberIDs = `SELECT item_id FROM library_items WHERE provider = ? AND kind = ?`

const albumColumns = `al.id, al.provider, al.provider_id, al.title, al.artist_credit,
	al.year, al.track_count, al.artwork_url, al.tracks_cached_at IS NOT NULL`

const trackColumns = `t.id, t.provider, t.provider_id, t.title, t.artist_credit,
	COALESCE(t.album_id, 0), COALESCE(al.title, t.album_title), t.disc, t.track_no,
	t.duration_ms, t.playable_uri, t.genre, t.year`

// Albums implements catalog.Catalog.
func (s *Store) Albums(ctx context.Context, provider string) ([]catalog.Album, error) {
	return queryAll(ctx, s.db, scanAlbum, `SELECT `+albumColumns+` FROM albums al
		WHERE al.id IN (`+memberIDs+`)
		ORDER BY al.sort_artist, al.sort_title`, provider, catalog.KindAlbum)
}

// AlbumTracks implements catalog.Catalog.
func (s *Store) AlbumTracks(ctx context.Context, albumID int64) ([]catalog.Track, bool, error) {
	var cached bool
	err := s.db.QueryRowContext(ctx, `SELECT tracks_cached_at IS NOT NULL FROM albums WHERE id = ?`, albumID).Scan(&cached)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("album %d: %w", albumID, catalog.ErrNotFound)
	}
	if err != nil {
		return nil, false, fmt.Errorf("album %d: %w", albumID, err)
	}
	tracks, err := queryAll(ctx, s.db, scanTrack, `SELECT `+trackColumns+`
		FROM tracks t LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.album_id = ?
		ORDER BY t.disc, t.track_no, t.title`, albumID)
	if err != nil {
		return nil, false, err
	}
	return tracks, cached, nil
}

// Artists implements catalog.Catalog.
func (s *Store) Artists(ctx context.Context, provider string) ([]catalog.Artist, error) {
	return queryAll(ctx, s.db, func(r *sql.Rows) (a catalog.Artist, err error) {
		err = r.Scan(&a.ID, &a.Ref.Provider, &a.Ref.ProviderID, &a.Name, &a.ImageURL)
		return a, err
	}, `SELECT ar.id, ar.provider, ar.provider_id, ar.name, ar.image_url FROM artists ar
		WHERE ar.id IN (`+memberIDs+`)
		ORDER BY ar.sort_name`, provider, catalog.KindArtist)
}

// ArtistAlbums implements catalog.Catalog.
func (s *Store) ArtistAlbums(ctx context.Context, artistID int64) ([]catalog.Album, error) {
	return queryAll(ctx, s.db, scanAlbum, `SELECT `+albumColumns+`
		FROM album_artists x JOIN albums al ON al.id = x.album_id
		WHERE x.artist_id = ?
		ORDER BY al.year DESC, al.sort_title`, artistID)
}

// Playlists implements catalog.Catalog.
func (s *Store) Playlists(ctx context.Context, provider string) ([]catalog.Playlist, error) {
	return queryAll(ctx, s.db, func(r *sql.Rows) (p catalog.Playlist, err error) {
		err = r.Scan(&p.ID, &p.Ref.Provider, &p.Ref.ProviderID, &p.Name, &p.Own, &p.TrackCount)
		return p, err
	}, `SELECT p.id, p.provider, p.provider_id, p.name, p.own, p.track_count FROM playlists p
		WHERE p.id IN (`+memberIDs+`)
		ORDER BY p.own DESC, lower(p.name)`, provider, catalog.KindPlaylist)
}

// PlaylistTracks implements catalog.Catalog.
func (s *Store) PlaylistTracks(ctx context.Context, playlistID int64) ([]catalog.Track, error) {
	return queryAll(ctx, s.db, scanTrack, `SELECT `+trackColumns+`
		FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE pt.playlist_id = ?
		ORDER BY pt.position`, playlistID)
}

// LikedTracks implements catalog.Catalog.
func (s *Store) LikedTracks(ctx context.Context, provider string) ([]catalog.Track, error) {
	return queryAll(ctx, s.db, scanTrack, `SELECT `+trackColumns+`
		FROM library_items li JOIN tracks t ON t.id = li.item_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE li.provider = ? AND li.kind = ?
		ORDER BY li.added_at DESC, t.id`, provider, catalog.KindTrack)
}

// SyncStatus implements catalog.Catalog.
func (s *Store) SyncStatus(ctx context.Context, provider string) ([]catalog.CollectionStatus, error) {
	return queryAll(ctx, s.db, func(r *sql.Rows) (st catalog.CollectionStatus, err error) {
		var attempt, success sql.NullInt64
		err = r.Scan(&st.Collection, &attempt, &success, &st.LastError)
		st.LastAttempt, st.LastSuccess = millisTime(attempt), millisTime(success)
		return st, err
	}, `SELECT collection, last_attempt_at, last_success_at, last_error
		FROM sync_state WHERE provider = ? ORDER BY collection`, provider)
}

// queryAll runs query and scans every row with scan.
func queryAll[T any](ctx context.Context, db *sql.DB, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog query: %w", err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog rows: %w", err)
	}
	return out, nil
}

func scanAlbum(r *sql.Rows) (a catalog.Album, err error) {
	err = r.Scan(&a.ID, &a.Ref.Provider, &a.Ref.ProviderID, &a.Title, &a.Artist,
		&a.Year, &a.TrackCount, &a.ArtworkURL, &a.TracksCached)
	return a, err
}

func scanTrack(r *sql.Rows) (t catalog.Track, err error) {
	var durationMS int64
	err = r.Scan(&t.ID, &t.Ref.Provider, &t.Ref.ProviderID, &t.Title, &t.Artist,
		&t.AlbumID, &t.AlbumTitle, &t.Disc, &t.TrackNo, &durationMS, &t.PlayableURI,
		&t.Genre, &t.Year)
	t.Duration = time.Duration(durationMS) * time.Millisecond
	return t, err
}

func millisTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64)
}
