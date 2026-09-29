package library

import (
	"context"
	"fmt"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
)

// LocalCatalog returns the Local menu. Albums, Artists and Genres come from
// the catalog's index of dir, so they open instantly; Folders opens the file
// browser; Playlists are the local provider's saved playlists. Every catalog
// level reloads when an index of the folder lands.
func LocalCatalog(cat catalog.Catalog, prov playlist.Provider, dir string) Level {
	b := &catalogBrowser{cat: cat, prov: prov, provider: catalog.Local,
		pending: "Indexing your music… (press r to retry if this persists)",
		none:    "No music found in " + dir}
	return local(prov,
		Entry{Title: "Albums", Open: b.list("Albums", b.localAlbums)},
		Entry{Title: "Artists", Open: b.list("Artists", b.localArtists)},
		Entry{Title: "Genres", Open: b.list("Genres", b.localGenres)},
	)
}

func (b *catalogBrowser) localAlbums(ctx context.Context) ([]Entry, error) {
	albums, err := b.cat.Albums(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	return b.syncingPlaceholder(ctx, b.localAlbumEntries(albums))
}

func (b *catalogBrowser) localAlbumEntries(albums []catalog.Album) []Entry {
	entries := make([]Entry, len(albums))
	for i, a := range albums {
		detail := a.Artist
		if a.Year > 0 {
			detail = fmt.Sprintf("%s · %d", detail, a.Year)
		}
		entries[i] = Entry{ID: catalogID(a.ID), Title: a.Title, Detail: detail, Open: b.list(a.Title, func(ctx context.Context) ([]Entry, error) {
			tracks, _, err := b.cat.AlbumTracks(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			return catalogTrackEntries(tracks), nil
		})}
	}
	return entries
}

func (b *catalogBrowser) localArtists(ctx context.Context) ([]Entry, error) {
	artists, err := b.cat.Artists(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(artists))
	for i, a := range artists {
		entries[i] = Entry{ID: catalogID(a.ID), Title: a.Name, Open: b.list(a.Name, func(ctx context.Context) ([]Entry, error) {
			albums, err := b.cat.ArtistAlbums(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			return b.localAlbumEntries(albums), nil
		})}
	}
	return b.syncingPlaceholder(ctx, entries)
}

func (b *catalogBrowser) localGenres(ctx context.Context) ([]Entry, error) {
	genres, err := b.cat.Genres(ctx, b.provider)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(genres))
	for i, g := range genres {
		entries[i] = Entry{ID: g.Name, Title: g.Name, Detail: albumCount(g.AlbumCount), Open: b.list(g.Name, func(ctx context.Context) ([]Entry, error) {
			albums, err := b.cat.GenreAlbums(ctx, b.provider, g.Name)
			if err != nil {
				return nil, err
			}
			return b.localAlbumEntries(albums), nil
		})}
	}
	return b.syncingPlaceholder(ctx, entries)
}

func albumCount(n int) string {
	if n == 1 {
		return "1 album"
	}
	return fmt.Sprintf("%d albums", n)
}
