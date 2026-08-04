package agent

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// APIError is returned from Query when the agent-run request completes with a
// non-2xx HTTP status. It carries the status code so callers can react
// programmatically — retry on 429/5xx, fail fast on 401/403 — rather than
// string-matching a message. Retrieve it with errors.As:
//
//	var apiErr *agent.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
//	    // back off and retry
//	}
type APIError struct {
	// StatusCode is the HTTP status code, e.g. 401.
	StatusCode int
	// Status is the HTTP status line, e.g. "401 Unauthorized".
	Status string
	// Body is the response body, trimmed and bounded to 64 KiB.
	Body string
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("trix: agent run failed: %s", e.Status)
	}
	return fmt.Sprintf("trix: agent run failed: %s — %s", e.Status, e.Body)
}

// ErrNotEventStream is returned from Query when a 2xx response does not carry a
// text/event-stream Content-Type — typically a proxy/captive-portal HTML page
// or a JSON error body served with status 200, which would otherwise parse to
// zero frames and masquerade as an empty-but-successful run. Match it with
// errors.Is(err, agent.ErrNotEventStream).
var ErrNotEventStream = errors.New("trix: response is not text/event-stream")

// newAPIError builds an *APIError from a non-2xx response, bounding the body
// read so a hostile/huge response can't OOM the client. It consumes and closes
// resp.Body.
func newAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // 64 KiB
	_ = resp.Body.Close()
	return &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       strings.TrimSpace(string(body)),
	}
}

// isEventStream reports whether a Content-Type header denotes an SSE stream.
// It tolerates parameters ("text/event-stream; charset=utf-8") and case. An
// empty Content-Type is treated leniently (accepted): some otherwise-valid
// servers omit it, and the goal is to reject a *wrong* type (HTML/JSON error
// pages), not to punish omission.
func isEventStream(contentType string) bool {
	if contentType == "" {
		return true
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "text/event-stream"
}
