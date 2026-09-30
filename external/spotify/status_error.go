package spotify

// omatunes: a typed Web API status error, so callers match the status
// code instead of the message text.

import "fmt"

// StatusError is a Web API response with an unexpected HTTP status. Its
// message is the one upstream formats ("http status 404 Not Found: …").
type StatusError struct {
	Code   int
	Status string // e.g. "404 Not Found"
	Body   string // the first 512 bytes, or why they could not be read
	read   bool   // Body is the response body
}

func (e *StatusError) Error() string {
	if !e.read {
		return fmt.Sprintf("http status %s (failed to read body: %s)", e.Status, e.Body)
	}
	return fmt.Sprintf("http status %s: %s", e.Status, e.Body)
}

func statusError(code int, status string, body []byte, readErr error) *StatusError {
	if readErr != nil {
		return &StatusError{Code: code, Status: status, Body: readErr.Error()}
	}
	return &StatusError{Code: code, Status: status, Body: string(body), read: true}
}
