// ddmus: a stalled token endpoint must not hold a session start forever.

package spotify

import (
	"context"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type stallTransport struct{}

func (stallTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestSilentTokenRefreshIsBounded(t *testing.T) {
	// The stall sits in the default transport, which a client without a
	// transport of its own (http.DefaultClient included) goes through.
	oldClient, oldTransport := tokenHTTPClient, http.DefaultTransport
	t.Cleanup(func() { tokenHTTPClient, http.DefaultTransport = oldClient, oldTransport })
	http.DefaultTransport = stallTransport{}
	tokenHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		_, err := silentTokenRefresh(context.Background(), "id", "rt") // ddmus: takes the caller's context
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("timed-out refresh succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh still blocked on the token endpoint")
	}
}

// An established session refreshes its expired Web API token through the
// same bounded client.
func TestWebAPITokenRefreshIsBounded(t *testing.T) {
	oldClient, oldTransport := tokenHTTPClient, http.DefaultTransport
	t.Cleanup(func() { tokenHTTPClient, http.DefaultTransport = oldClient, oldTransport })
	http.DefaultTransport = stallTransport{}
	tokenHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	expired := &oauth2.Token{AccessToken: "old", RefreshToken: "rt", Expiry: time.Now().Add(-time.Hour)}
	source := webAPITokenSource("id", expired, storedCreds{RefreshToken: "rt"})
	done := make(chan error, 1)
	go func() {
		_, err := source.Token()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("timed-out refresh succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh of an expired token still blocked on the token endpoint")
	}
}
