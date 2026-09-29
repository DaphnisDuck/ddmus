package sqlite

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

// seed loads a small two-provider catalog:
//
//	spotify: artists Ozawa (followed), Muti; albums Mahler 5 (Ozawa, saved,
//	tracks cached), Roman Trilogy (Muti+Ozawa, saved, not cached), Rarity
//	(Ozawa, not saved); playlists Mine (own), Theirs (followed); one liked
//	track; one failed and one successful sync collection.
//	local: one album, so provider filtering is exercised.
func seed(t *testing.T, s *Store) {
	t.Helper()
	stmts := []string{
		`INSERT INTO artists (id, provider, provider_id, name, sort_name, updated_at) VALUES
			(1, 'spotify', 'ar-ozawa', 'Seiji Ozawa', 'ozawa, seiji', 0),
			(2, 'spotify', 'ar-muti',  'Riccardo Muti', 'muti, riccardo', 0),
			(3, 'local',   'Marriner', 'Neville Marriner', 'marriner, neville', 0)`,
		`INSERT INTO albums (id, provider, provider_id, title, sort_title, artist_credit, sort_artist, year, track_count, tracks_cached_at, updated_at) VALUES
			(10, 'spotify', 'al-m5',     'Mahler: Symphony No. 5', 'mahler: symphony no. 5', 'Seiji Ozawa',                'seiji ozawa',      1990, 2, 1, 0),
			(11, 'spotify', 'al-roman',  'Roman Trilogy',          'roman trilogy',          'Riccardo Muti, Seiji Ozawa', 'riccardo muti',    1985, 3, NULL, 0),
			(12, 'spotify', 'al-rarity', 'Rarity',                 'rarity',                 'Seiji Ozawa',                'seiji ozawa',      2001, 1, NULL, 0),
			(13, 'local',   '/m/roman',  'Roman Trilogy',          'roman trilogy',          'Neville Marriner',           'neville marriner', 1976, 1, 1, 0)`,
		`INSERT INTO album_artists (album_id, artist_id, position) VALUES
			(10, 1, 0), (11, 2, 0), (11, 1, 1), (12, 1, 0), (13, 3, 0)`,
		`INSERT INTO tracks (id, provider, provider_id, title, artist_credit, album_id, disc, track_no, duration_ms, playable_uri, updated_at) VALUES
			(100, 'spotify', 't-m5-2', 'II. Stürmisch bewegt', 'Seiji Ozawa', 10, 1, 2, 900000, 'spotify:track:t-m5-2', 0),
			(101, 'spotify', 't-m5-1', 'I. Trauermarsch',      'Seiji Ozawa', 10, 1, 1, 780000, 'spotify:track:t-m5-1', 0),
			(102, 'spotify', 't-loose', 'Loose Single',        'Riccardo Muti, Seiji Ozawa', NULL, 1, 0, 200000, 'spotify:track:t-loose', 0)`,
		`UPDATE tracks SET album_title = 'Some Single' WHERE id = 102`,
		`INSERT INTO track_artists (track_id, artist_id, position) VALUES (100, 1, 0), (101, 1, 0), (102, 2, 0), (102, 1, 1)`,
		`INSERT INTO playlists (id, provider, provider_id, name, own, track_count, updated_at) VALUES
			(20, 'spotify', 'pl-theirs', 'Theirs', 0, 0, 0),
			(21, 'spotify', 'pl-mine',   'Mine',   1, 2, 0)`,
		`INSERT INTO playlist_tracks (playlist_id, position, track_id) VALUES (21, 0, 102), (21, 1, 100)`,
		`INSERT INTO library_items (provider, collection, kind, item_id, added_at, last_seen_gen) VALUES
			('spotify', 'albums', 'album', 10, 1, 1), ('spotify', 'albums', 'album', 11, 1, 1),
			('spotify', 'artists', 'artist', 1, 1, 1),
			('spotify', 'playlists', 'playlist', 20, 1, 1), ('spotify', 'playlists', 'playlist', 21, 1, 1),
			('spotify', 'liked', 'track', 101, 5, 1), ('spotify', 'liked', 'track', 102, 9, 1),
			-- The same album held by a second collection is still listed once.
			('spotify', 'other', 'album', 10, 1, 1),
			('local', 'files', 'album', 13, 1, 1)`,
		`INSERT INTO sync_state (provider, collection, generation, last_attempt_at, last_success_at, last_error) VALUES
			('spotify', 'albums', 1, 2000, 2000, ''),
			('spotify', 'playlists', 1, 3000, 1000, 'http status 429')`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
}

func seeded(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s := openTemp(t)
	seed(t, s)
	return s, context.Background()
}

func TestAlbums(t *testing.T) {
	s, ctx := seeded(t)
	got, err := s.Albums(ctx, catalog.Spotify)
	if err != nil {
		t.Fatal(err)
	}
	// Library albums only (not Rarity), by first artist (Muti < Ozawa).
	if len(got) != 2 || got[0].Title != "Roman Trilogy" || got[1].Title != "Mahler: Symphony No. 5" {
		t.Fatalf("Albums() = %+v", got)
	}
	roman, mahler := got[0], got[1]
	if roman.Artist != "Riccardo Muti, Seiji Ozawa" || roman.TracksCached || roman.Ref != (catalog.Ref{Provider: "spotify", ProviderID: "al-roman"}) {
		t.Errorf("Roman Trilogy = %+v", roman)
	}
	if !mahler.TracksCached || mahler.Year != 1990 || mahler.TrackCount != 2 {
		t.Errorf("Mahler 5 = %+v", mahler)
	}
	local, _ := s.Albums(ctx, catalog.Local)
	if len(local) != 1 || local[0].Artist != "Neville Marriner" {
		t.Errorf("local Albums() = %+v", local)
	}
}

func TestAlbumTracks(t *testing.T) {
	s, ctx := seeded(t)
	tracks, cached, err := s.AlbumTracks(ctx, 10)
	if err != nil || !cached {
		t.Fatalf("AlbumTracks(10) cached=%v err=%v", cached, err)
	}
	if len(tracks) != 2 || tracks[0].Title != "I. Trauermarsch" || tracks[1].TrackNo != 2 {
		t.Fatalf("tracks = %+v, want track order", tracks)
	}
	first := tracks[0]
	if first.Artist != "Seiji Ozawa" || first.AlbumTitle != "Mahler: Symphony No. 5" || first.AlbumID != 10 ||
		first.Duration != 13*time.Minute || first.PlayableURI != "spotify:track:t-m5-1" {
		t.Errorf("first track = %+v", first)
	}

	if tracks, cached, err := s.AlbumTracks(ctx, 11); err != nil || cached || len(tracks) != 0 {
		t.Errorf("uncached album: %d tracks, cached=%v, err=%v", len(tracks), cached, err)
	}
	if _, _, err := s.AlbumTracks(ctx, 999); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("AlbumTracks(missing) error = %v, want ErrNotFound", err)
	}
}

func TestArtistsAndArtistAlbums(t *testing.T) {
	s, ctx := seeded(t)
	artists, err := s.Artists(ctx, catalog.Spotify)
	if err != nil || len(artists) != 1 || artists[0].Name != "Seiji Ozawa" {
		t.Fatalf("Artists() = %+v, %v; want only the followed artist", artists, err)
	}
	albums, err := s.ArtistAlbums(ctx, artists[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range albums {
		titles = append(titles, a.Title)
	}
	// Every credited album, saved or not, newest first.
	if want := []string{"Rarity", "Mahler: Symphony No. 5", "Roman Trilogy"}; !slices.Equal(titles, want) {
		t.Errorf("ArtistAlbums() = %v, want %v", titles, want)
	}
}

func TestPlaylists(t *testing.T) {
	s, ctx := seeded(t)
	pls, err := s.Playlists(ctx, catalog.Spotify)
	if err != nil || len(pls) != 2 || pls[0].Name != "Mine" || !pls[0].Own || pls[1].Own {
		t.Fatalf("Playlists() = %+v, %v; want own first", pls, err)
	}
	tracks, err := s.PlaylistTracks(ctx, pls[0].ID)
	if err != nil || len(tracks) != 2 || tracks[0].Title != "Loose Single" {
		t.Fatalf("PlaylistTracks() = %+v, %v", tracks, err)
	}
	loose := tracks[0]
	if loose.AlbumID != 0 || loose.AlbumTitle != "Some Single" || loose.Artist != "Riccardo Muti, Seiji Ozawa" {
		t.Errorf("album-less track = %+v", loose)
	}
}

func TestLikedTracks(t *testing.T) {
	s, ctx := seeded(t)
	liked, err := s.LikedTracks(ctx, catalog.Spotify)
	if err != nil || len(liked) != 2 || liked[0].Title != "Loose Single" {
		t.Fatalf("LikedTracks() = %+v, %v; want most recently liked first", liked, err)
	}
}

func TestSyncStatus(t *testing.T) {
	s, ctx := seeded(t)
	st, err := s.SyncStatus(ctx, catalog.Spotify)
	if err != nil || len(st) != 2 {
		t.Fatalf("SyncStatus() = %+v, %v", st, err)
	}
	albums, pls := st[0], st[1]
	if albums.Collection != "albums" || albums.LastError != "" || !albums.LastSuccess.Equal(time.UnixMilli(2000)) {
		t.Errorf("albums status = %+v", albums)
	}
	if !strings.Contains(pls.LastError, "429") || !pls.LastAttempt.After(pls.LastSuccess) {
		t.Errorf("playlists status = %+v, want a failed attempt after the last success", pls)
	}
	if none, _ := s.SyncStatus(ctx, catalog.Local); len(none) != 0 {
		t.Errorf("local SyncStatus() = %+v, want none", none)
	}
}
