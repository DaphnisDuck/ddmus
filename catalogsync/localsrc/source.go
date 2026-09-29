// Package localsrc is the catalog sync source for the local music folder.
// It walks the folder, rereads tags only for files whose size or modification
// time changed, and groups every file into albums and artists the way the
// v0.1 in-memory scan did.
package localsrc

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bjarneo/cliamp/catalog"
	"github.com/bjarneo/cliamp/catalogsync"
	"github.com/bjarneo/cliamp/player"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/resolve"
)

// Files is the source's one collection: every audio file under the folder.
const Files = "files"

// tagBatch is how many changed files are read between cancellation checks.
const tagBatch = 256

// Index is the part of the catalog the source reads to skip unchanged files.
type Index interface {
	IndexedFiles(ctx context.Context) ([]catalog.IndexedFile, error)
}

// Source indexes one music folder.
type Source struct {
	dir      string
	index    Index
	readTags func(paths []string) []playlist.Track // replaced in tests
}

var _ catalogsync.Source = (*Source)(nil)

// New returns a Source for dir, comparing against index.
func New(dir string, index Index) *Source {
	return &Source{dir: dir, index: index, readTags: resolve.TracksFromPaths}
}

// Dir is the indexed folder.
func (s *Source) Dir() string { return s.dir }

// Provider implements catalogsync.Source.
func (*Source) Provider() string { return catalog.Local }

// Collections implements catalogsync.Source.
func (*Source) Collections() []string { return []string{Files} }

// Fetch implements catalogsync.Source. It fails, leaving the catalog as it
// is, when the folder is missing, or is empty while the catalog holds files
// (an unmounted drive looks like that). Files under a subfolder it cannot
// read are kept as last indexed rather than dropped.
func (s *Source) Fetch(ctx context.Context, collection string, _ catalogsync.Known) (catalog.Snapshot, error) {
	if collection != Files {
		return catalog.Snapshot{}, fmt.Errorf("unknown collection %q", collection)
	}
	indexed, err := s.index.IndexedFiles(ctx)
	if err != nil {
		return catalog.Snapshot{}, err
	}
	found, unreadable, err := walk(ctx, s.dir)
	if err != nil {
		return catalog.Snapshot{}, err
	}
	if len(found) == 0 && len(indexed) > 0 {
		return catalog.Snapshot{}, fmt.Errorf("no music found in %s; keeping the %d indexed files", s.dir, len(indexed))
	}

	stored := make(map[string]catalog.IndexedFile, len(indexed))
	for _, f := range indexed {
		stored[f.Path] = f
	}
	files := make([]file, 0, len(found))
	var changed []int // indexes into files
	for _, st := range found {
		if prev, ok := stored[st.Path]; ok && prev.Size == st.Size && prev.MTimeNS == st.MTimeNS {
			files = append(files, file{stat: st, track: storedTrack(prev.Track, st.Path)})
		} else {
			changed = append(changed, len(files))
			files = append(files, file{stat: st})
		}
		delete(stored, st.Path)
	}
	// What is left was not found: deleted, unless it sits in a folder the
	// walk could not read.
	removed := 0
	for _, prev := range stored {
		if under(prev.Path, unreadable) {
			files = append(files, file{stat: prev.FileStat, track: storedTrack(prev.Track, prev.Path)})
		} else {
			removed++
		}
	}
	if len(changed) == 0 && removed == 0 {
		return catalog.Snapshot{}, catalogsync.ErrUnchanged
	}

	for start := 0; start < len(changed); start += tagBatch {
		if err := ctx.Err(); err != nil {
			return catalog.Snapshot{}, err
		}
		batch := changed[start:min(start+tagBatch, len(changed))]
		paths := make([]string, len(batch))
		for i, fi := range batch {
			paths[i] = files[fi].stat.Path
		}
		for i, t := range s.readTags(paths) {
			t.Path = paths[i]
			files[batch[i]].track = t
		}
	}
	return snapshot(files), nil
}

// file is one indexed audio file and its track.
type file struct {
	stat  catalog.FileStat
	track playlist.Track
}

// walk lists the audio files under dir with their size and modification
// time. It fails if dir itself cannot be read; unreadable subfolders are
// skipped and returned.
func walk(ctx context.Context, dir string) ([]catalog.FileStat, []string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("music folder: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("music folder %s is not a directory", dir)
	}
	var files []catalog.FileStat
	var unreadable []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if path == dir {
				return fmt.Errorf("music folder: %w", err)
			}
			if d == nil || d.IsDir() {
				unreadable = append(unreadable, path)
				return fs.SkipDir
			}
			unreadable = append(unreadable, path) // an unreadable file keeps its entry
			return nil
		}
		if d.IsDir() || !player.SupportedExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		fi, err := os.Stat(path) // follows symlinks, as playback does
		if err != nil || !fi.Mode().IsRegular() {
			unreadable = append(unreadable, path)
			return nil
		}
		files = append(files, catalog.FileStat{Path: path, Size: fi.Size(), MTimeNS: fi.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return files, unreadable, nil
}

// under reports whether path is one of roots or inside one.
func under(path string, roots []string) bool {
	for _, r := range roots {
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// storedTrack rebuilds the tags of an unchanged file from its catalog row.
func storedTrack(t catalog.Track, path string) playlist.Track {
	return playlist.Track{
		Path:         path,
		Title:        t.Title,
		Artist:       t.Artist,
		Album:        t.AlbumTitle,
		Genre:        t.Genre,
		Year:         t.Year,
		TrackNumber:  t.TrackNo,
		DurationSecs: int(t.Duration.Seconds()),
	}
}
