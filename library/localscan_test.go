package library

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/playlist"
)

func TestBuildIndex(t *testing.T) {
	tracks := []playlist.Track{
		{Path: "/m/Mahler/5/CD2/01.flac", Title: "IV", Artist: "Ozawa", Album: "Mahler 5", Genre: "Classical", Year: 1990, TrackNumber: 1},
		{Path: "/m/Mahler/5/CD1/02.flac", Title: "II", Artist: "Ozawa", Album: "Mahler 5", Genre: "Classical", TrackNumber: 2},
		{Path: "/m/Mahler/5/CD1/01.flac", Title: "I", Artist: "Ozawa", Album: "mahler 5", Genre: "classical", TrackNumber: 1},
		{Path: "/m/Mix/01.mp3", Title: "A", Artist: "Ravel", Album: "Mix", Genre: "Classical"},
		{Path: "/m/Mix/02.mp3", Title: "B", Artist: "Respighi", Album: "Mix"},
		{Path: "/m/Untagged/x.mp3", Title: "x"},
	}
	idx := BuildIndex(tracks)

	type albumRow struct {
		title, artist string
		year          int
		paths         []string
	}
	var got []albumRow
	for _, a := range idx.Albums {
		row := albumRow{a.Title, a.Artist, a.Year, nil}
		for _, tr := range a.Tracks {
			row.paths = append(row.paths, tr.Path)
		}
		got = append(got, row)
	}
	want := []albumRow{
		{"Mahler 5", "Ozawa", 1990, []string{"/m/Mahler/5/CD1/01.flac", "/m/Mahler/5/CD1/02.flac", "/m/Mahler/5/CD2/01.flac"}},
		{"Mix", variousArtists, 0, []string{"/m/Mix/01.mp3", "/m/Mix/02.mp3"}},
		{"Untagged", unknownArtist, 0, []string{"/m/Untagged/x.mp3"}},
	}
	if len(got) != len(want) {
		t.Fatalf("albums = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].title != want[i].title || got[i].artist != want[i].artist || got[i].year != want[i].year || !slices.Equal(got[i].paths, want[i].paths) {
			t.Errorf("album %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	groupNames := func(groups []*Group) []string {
		var out []string
		for _, g := range groups {
			out = append(out, g.Name)
		}
		return out
	}
	if got, want := groupNames(idx.Artists), []string{"Ozawa", "Ravel", "Respighi", unknownArtist}; !slices.Equal(got, want) {
		t.Errorf("artists = %v, want %v", got, want)
	}
	if got := groupNames(idx.Genres); len(got) != 1 || !strings.EqualFold(got[0], "Classical") {
		t.Errorf("genres = %v, want one case-folded Classical", got)
	}
	if n := len(idx.Genres[0].Albums); n != 2 {
		t.Errorf("Classical albums = %d, want 2", n)
	}
}

func TestScannerCachesSuccessOnly(t *testing.T) {
	calls := 0
	fail := true
	s := &Scanner{dir: "/m", scan: func(dir string) ([]playlist.Track, error) {
		calls++
		if fail {
			return nil, errors.New("disk gone")
		}
		return []playlist.Track{{Path: dir + "/a.mp3", Album: "A"}}, nil
	}}
	ctx := context.Background()
	if _, err := s.Index(ctx); err == nil {
		t.Fatal("Index() error = nil, want scan failure")
	}
	fail = false
	for range 2 {
		idx, err := s.Index(ctx)
		if err != nil || len(idx.Albums) != 1 {
			t.Fatalf("Index() = %+v, %v", idx, err)
		}
	}
	if calls != 2 {
		t.Errorf("scans = %d, want 2 (failure retried, success cached)", calls)
	}
}

func TestLocalMenu(t *testing.T) {
	tests := []struct {
		name    string
		prov    playlist.Provider
		scanner *Scanner
		want    []string
	}{
		{"scanner and provider", &fakeProvider{name: "Local"}, NewScanner("/m"), []string{"Albums", "Artists", "Genres", "Folders", "Playlists"}},
		{"provider only", &fakeProvider{name: "Local"}, nil, []string{"Folders", "Playlists"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := load(t, local(tt.prov, tt.scanner))
			if got := titles(entries); !slices.Equal(got, tt.want) {
				t.Errorf("local menu = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalEmptyLibraryNamesDirectory(t *testing.T) {
	s := &Scanner{dir: "/m", scan: func(string) ([]playlist.Track, error) { return nil, nil }}
	rows := load(t, child(t, local(nil, s), "Albums"))
	if len(rows) != 1 || rows[0].Title != "No music found in /m" {
		t.Errorf("rows = %+v, want a message naming /m", rows)
	}
}

func TestScannerCancelLeavesScanRunning(t *testing.T) {
	release := make(chan struct{})
	calls := 0
	s := &Scanner{dir: "/m", scan: func(dir string) ([]playlist.Track, error) {
		calls++
		<-release
		return []playlist.Track{{Path: dir + "/a.mp3", Album: "A"}}, nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Index(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Index(cancelled) error = %v, want context.Canceled", err)
	}
	close(release)
	idx, err := s.Index(context.Background())
	if err != nil || len(idx.Albums) != 1 {
		t.Fatalf("Index() = %+v, %v", idx, err)
	}
	if calls != 1 {
		t.Errorf("scans = %d, want the abandoned scan reused", calls)
	}
}
