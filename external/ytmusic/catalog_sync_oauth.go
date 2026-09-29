// omatunes: whole-collection fetchers for the catalog sync in OAuth mode,
// through the YouTube Data API. Like the cookie fetchers, each returns
// catalog records carrying YouTube IDs or an error, never a partial
// collection. Kept in its own file so upstream merges stay conflict-free.

package ytmusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/youtube/v3"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/playlist"
)

// OAuthCatalog reads a YouTube account for the catalog through the Data
// API, signed in with the stored OAuth credentials. It never signs in
// interactively: without stored credentials it fails with
// playlist.ErrNeedsAuth.
type OAuthCatalog struct {
	clientID, clientSecret string
	// service returns the API client and the account's classification
	// cache scope; replaced in tests.
	service func(ctx context.Context) (*youtube.Service, string, error)
}

// NewOAuthCatalog returns an OAuthCatalog for the client.
func NewOAuthCatalog(clientID, clientSecret string) *OAuthCatalog {
	c := &OAuthCatalog{clientID: clientID, clientSecret: clientSecret}
	c.service = c.silentService
	return c
}

func (c *OAuthCatalog) silentService(ctx context.Context) (*youtube.Service, string, error) {
	sess, err := NewSessionSilent(ctx, c.clientID, c.clientSecret)
	if err != nil {
		return nil, "", fmt.Errorf("youtube: %v: %w", err, playlist.ErrNeedsAuth)
	}
	return sess.Service(), sess.cacheScope, nil
}

// PlaylistRecords returns the account's music playlists, without tracks,
// classified as in cliamp's OAuth mode. Snapshot is the item count and the
// playlist's etag, so an unchanged playlist is not reread.
func (c *OAuthCatalog) PlaylistRecords(ctx context.Context) ([]catalog.PlaylistRecord, error) {
	svc, scope, err := c.service(ctx)
	if err != nil {
		return nil, err
	}
	type listed struct {
		entry playlistEntry
		etag  string
	}
	var all []listed
	for page := ""; ; {
		call := svc.Playlists.List([]string{"snippet", "contentDetails"}).Mine(true).MaxResults(50).Context(ctx)
		if page != "" {
			call = call.PageToken(page)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("youtube: list playlists: %w", err)
		}
		for _, p := range resp.Items {
			if p.Id == "" || notOwnLists[p.Id] {
				continue
			}
			var count int
			if p.ContentDetails != nil {
				count = int(p.ContentDetails.ItemCount)
			}
			all = append(all, listed{playlistEntry{ID: p.Id, Name: p.Snippet.Title, TrackCount: count}, p.Etag})
		}
		if page = resp.NextPageToken; page == "" {
			break
		}
	}
	entries := make([]playlistEntry, len(all))
	for i, l := range all {
		entries[i] = l.entry
	}
	music := classifyPlaylists(ctx, svc, entries, nil, scope)
	var records []catalog.PlaylistRecord
	for _, l := range all {
		if music[l.entry.ID] {
			records = append(records, catalog.PlaylistRecord{
				Ref: youtubeRef(l.entry.ID), Name: l.entry.Name, Own: true, TrackCount: l.entry.TrackCount,
				Snapshot: fmt.Sprintf("%d:%s", l.entry.TrackCount, l.etag),
			})
		}
	}
	return records, nil
}

// PlaylistRecord returns one playlist, by ID, without tracks: for a
// playlist someone else owns, which no list of yours includes. A playlist
// the API does not return fails with an error wrapping catalog.ErrForbidden.
func (c *OAuthCatalog) PlaylistRecord(ctx context.Context, playlistID string) (catalog.PlaylistRecord, error) {
	svc, _, err := c.service(ctx)
	if err != nil {
		return catalog.PlaylistRecord{}, err
	}
	resp, err := svc.Playlists.List([]string{"snippet", "contentDetails"}).Id(playlistID).Context(ctx).Do()
	if err != nil {
		return catalog.PlaylistRecord{}, fmt.Errorf("youtube: playlist %s: %w", playlistID, apiError(err))
	}
	if len(resp.Items) == 0 {
		return catalog.PlaylistRecord{}, fmt.Errorf("youtube: playlist %s: %w", playlistID, catalog.ErrForbidden)
	}
	p := resp.Items[0]
	var count int
	if p.ContentDetails != nil {
		count = int(p.ContentDetails.ItemCount)
	}
	return catalog.PlaylistRecord{Ref: youtubeRef(playlistID), Name: p.Snippet.Title, TrackCount: count,
		Snapshot: fmt.Sprintf("%d:%s", count, p.Etag)}, nil
}

// PlaylistTrackRecords returns a playlist's tracks in order. A playlist
// that is gone or private fails with an error wrapping catalog.ErrForbidden.
func (c *OAuthCatalog) PlaylistTrackRecords(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error) {
	return c.playlistTracks(ctx, playlistID)
}

// LikedTrackRecords returns the account's Liked Music.
func (c *OAuthCatalog) LikedTrackRecords(ctx context.Context) ([]catalog.TrackRecord, error) {
	return c.playlistTracks(ctx, likedMusicID)
}

func (c *OAuthCatalog) playlistTracks(ctx context.Context, playlistID string) ([]catalog.TrackRecord, error) {
	svc, _, err := c.service(ctx)
	if err != nil {
		return nil, err
	}
	var tracks []catalog.TrackRecord
	var items []itemInfo
	read, total := 0, -1
	for page := ""; ; {
		call := svc.PlaylistItems.List([]string{"snippet", "contentDetails"}).PlaylistId(playlistID).MaxResults(50).Context(ctx)
		if page != "" {
			call = call.PageToken(page)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("youtube: playlist %s: %w", playlistID, apiError(err))
		}
		if resp.PageInfo != nil {
			if total >= 0 && int(resp.PageInfo.TotalResults) != total {
				return nil, fmt.Errorf("youtube: playlist %s: %w", playlistID, ErrIncomplete)
			}
			total = int(resp.PageInfo.TotalResults)
		}
		// Every item counts toward completeness, playable or not.
		read += len(resp.Items)
		for _, it := range resp.Items {
			rec, ok := itemRecord(it)
			if !ok {
				continue
			}
			tracks = append(tracks, rec)
			items = append(items, itemInfo{videoID: rec.Ref.ProviderID})
		}
		if page = resp.NextPageToken; page == "" || len(resp.Items) == 0 {
			break
		}
	}
	if total >= 0 && read != total {
		return nil, fmt.Errorf("youtube: playlist %s: read %d of %d: %w", playlistID, read, total, ErrIncomplete)
	}
	// Durations are a separate call per 50 videos; one that fails leaves
	// its tracks' durations unknown rather than failing the playlist.
	durations := (*baseProvider)(nil).fetchDurations(ctx, svc, items)
	for i := range tracks {
		tracks[i].Duration = time.Duration(durations[tracks[i].Ref.ProviderID]) * time.Second
	}
	return tracks, nil
}

// itemRecord maps a playlist item. Private and deleted videos have no
// track.
func itemRecord(it *youtube.PlaylistItem) (catalog.TrackRecord, bool) {
	if it == nil || it.Snippet == nil || it.ContentDetails == nil || it.ContentDetails.VideoId == "" {
		return catalog.TrackRecord{}, false
	}
	title := it.Snippet.Title
	if title == "Private video" || title == "Deleted video" {
		return catalog.TrackRecord{}, false
	}
	id := it.ContentDetails.VideoId
	rec := catalog.TrackRecord{Ref: youtubeRef(id), Title: title, PlayableURI: watchURL(id)}
	if artist, ok := channelArtist(it.Snippet.VideoOwnerChannelId, it.Snippet.VideoOwnerChannelTitle); ok {
		rec.Artists = []catalog.ArtistRecord{artist}
	}
	if added, err := time.Parse(time.RFC3339, it.Snippet.PublishedAt); err == nil {
		rec.AddedAt = added
	}
	return rec, true
}

// apiError marks a playlist the API will not show (gone, or private to
// someone else) as catalog.ErrForbidden, so a sync keeps what it had.
func apiError(err error) error {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && (gerr.Code == http.StatusNotFound || gerr.Code == http.StatusForbidden) {
		return fmt.Errorf("%w: %w", catalog.ErrForbidden, err)
	}
	return err
}
