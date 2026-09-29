package sqlite

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/catalog"
)

func TestCacheAlbumTracks(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	apply(t, s, catalog.Snapshot{Collection: "albums", Albums: []catalog.AlbumRecord{
		{Ref: sref("al1"), Title: "Symphonies", Year: 1987, ArtworkURL: "https://img"},
	}})
	albums, err := s.Albums(ctx, catalog.Spotify)
	if err != nil || len(albums) != 1 {
		t.Fatalf("Albums() = %v, %v", albums, err)
	}
	id := albums[0].ID

	// The API's simplified tracks name no album; they join the cached one.
	err = s.CacheAlbumTracks(ctx, sref("al1"), []catalog.TrackRecord{
		{Ref: sref("t2"), Title: "II.", Disc: 1, TrackNo: 2, PlayableURI: "spotify:track:t2", Artists: []catalog.ArtistRecord{artistRec("ar1", "Ozawa")}},
		{Ref: sref("t1"), Title: "I.", Disc: 1, TrackNo: 1, PlayableURI: "spotify:track:t1", Duration: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	tracks, cached, err := s.AlbumTracks(ctx, id)
	if err != nil || !cached {
		t.Fatalf("AlbumTracks() cached=%v err=%v", cached, err)
	}
	var titles []string
	for _, tr := range tracks {
		titles = append(titles, tr.Title)
		if tr.AlbumID != id || tr.AlbumTitle != "Symphonies" || tr.Year != 1987 || tr.ArtworkURL != "https://img" {
			t.Errorf("track %s = %+v, want the album's details", tr.Title, tr)
		}
	}
	if !slices.Equal(titles, []string{"I.", "II."}) || tracks[1].Artist != "Ozawa" {
		t.Errorf("tracks = %+v", tracks)
	}
	var count int
	if err := s.db.QueryRow(`SELECT track_count FROM albums WHERE id = ?`, id).Scan(&count); err != nil || count != 2 {
		t.Errorf("track_count = %d, %v; want the cached count when Spotify gave none", count, err)
	}
	if left, err := s.UncachedAlbums(ctx, catalog.Spotify); err != nil || len(left) != 0 {
		t.Errorf("UncachedAlbums() after caching = %v, %v", left, err)
	}
}

func TestCacheAlbumTracksRejects(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.CacheAlbumTracks(ctx, sref("missing"), nil); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("unknown album: %v, want ErrNotFound", err)
	}
	apply(t, s, catalog.Snapshot{Collection: "albums", Albums: []catalog.AlbumRecord{{Ref: sref("al1"), Title: "A"}}})
	foreign := catalog.TrackRecord{Ref: catalog.Ref{Provider: catalog.Local, ProviderID: "x"}, Title: "x"}
	if err := s.CacheAlbumTracks(ctx, sref("al1"), []catalog.TrackRecord{foreign}); err == nil {
		t.Error("a foreign track was cached")
	}
	// The failed write left the album uncached.
	if left, _ := s.UncachedAlbums(ctx, catalog.Spotify); len(left) != 1 {
		t.Errorf("UncachedAlbums() = %v, want the album still uncached", left)
	}
}

func TestUncachedAlbumsNewestSavedFirst(t *testing.T) {
	s := openTemp(t)
	day := func(d int) time.Time { return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC) }
	apply(t, s, catalog.Snapshot{Collection: "albums", Albums: []catalog.AlbumRecord{
		{Ref: sref("old"), Title: "Old", AddedAt: day(1)},
		{Ref: sref("new"), Title: "New", AddedAt: day(3)},
		{Ref: sref("mid"), Title: "Mid", AddedAt: day(2)},
	}})
	// An album known only through a liked track is not a library album.
	apply(t, s, catalog.Snapshot{Collection: "liked", Tracks: []catalog.TrackRecord{
		{Ref: sref("t"), Title: "T", Album: &catalog.AlbumRecord{Ref: sref("other"), Title: "Other"}},
	}})
	got, err := s.UncachedAlbums(context.Background(), catalog.Spotify)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range got {
		titles = append(titles, a.Title)
	}
	if !slices.Equal(titles, []string{"New", "Mid", "Old"}) {
		t.Errorf("UncachedAlbums() = %v", titles)
	}
}
