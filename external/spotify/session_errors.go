// omatunes: classify silent sign-in failures. Upstream ensureSession reported
// every failure as playlist.ErrNeedsAuth, so an outage (login5 answering 503
// "no healthy upstream") read as "sign-in required" and would prompt a
// pointless sign-in. Only a missing or rejected credential needs one.

package spotify

import (
	"errors"
	"fmt"

	"github.com/devgianlu/go-librespot/ap"
	"github.com/devgianlu/go-librespot/login5"

	"github.com/bjarneo/cliamp/playlist"
)

// errNoStoredCreds means there is no saved Spotify sign-in.
var errNoStoredCreds = errors.New("no stored credentials")

// silentSessionError maps a NewSessionSilent failure to what the caller
// should do: ErrNeedsAuth when signing in again would fix it, otherwise the
// cause itself (network trouble, a Spotify outage), to retry later.
func silentSessionError(err error) error {
	var loginErr *login5.LoginError
	var apErr *ap.AccesspointLoginError
	switch {
	case errors.Is(err, playlist.ErrNeedsAuth), errors.Is(err, errNoStoredCreds),
		errors.As(err, &loginErr), errors.As(err, &apErr):
		return playlist.ErrNeedsAuth
	default:
		return fmt.Errorf("spotify: sign-in unavailable: %w", err)
	}
}
