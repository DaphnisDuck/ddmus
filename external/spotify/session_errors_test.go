// omatunes: tests for silent sign-in error classification.

package spotify

import (
	"errors"
	"fmt"
	"testing"

	"github.com/devgianlu/go-librespot/ap"
	"github.com/devgianlu/go-librespot/login5"

	"github.com/bjarneo/cliamp/playlist"
)

func TestSilentSessionError(t *testing.T) {
	outage := errors.New("faield unmarshalling LoginResponse: proto: cannot parse invalid wire-format data")
	tests := []struct {
		name      string
		err       error
		needsAuth bool
	}{
		{"no stored credentials", errNoStoredCreds, true},
		{"refresh token revoked", fmt.Errorf("spotify: %w", playlist.ErrNeedsAuth), true},
		{"login5 rejected the credential", fmt.Errorf("spotify: stored auth: %w", &login5.LoginError{}), true},
		{"accesspoint rejected the credential", fmt.Errorf("spotify: stored auth: %w", &ap.AccesspointLoginError{}), true},
		{"login5 outage", fmt.Errorf("spotify: stored auth: failed requesting login5 endpoint: %w", outage), false},
		{"offline", errors.New("dial tcp: lookup apresolve.spotify.com: no such host"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := silentSessionError(tt.err)
			if errors.Is(got, playlist.ErrNeedsAuth) != tt.needsAuth {
				t.Fatalf("silentSessionError() = %v, needs auth %v", got, !tt.needsAuth)
			}
			if !tt.needsAuth && !errors.Is(got, tt.err) {
				t.Errorf("silentSessionError() = %v, lost the cause", got)
			}
		})
	}
}
