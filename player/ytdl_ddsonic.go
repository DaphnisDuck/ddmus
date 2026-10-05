package player

// ddmus: upstream 70e5f79e's clock for a yt-dlp seek by restart, ported
// without the player refactors it sits on upstream.

import "time"

// ytdlPlayedPosition is the position of the audio that played. A prefetched
// yt-dlp decoder reads ahead of the speaker, so a seek by restart counts from
// the prefetch, as Position does. The caller holds the speaker lock.
func ytdlPlayedPosition(cur *trackPipeline) time.Duration {
	if cur.livePrefetch != nil {
		return cur.livePrefetch.Position() + cur.streamOffset
	}
	return cur.format.SampleRate.D(cur.decoder.Position()) + cur.streamOffset
}
