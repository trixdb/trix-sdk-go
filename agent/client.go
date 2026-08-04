package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Client is the consumer-facing handle for issuing Query() calls.
type Client struct {
	baseURL string
	hc      *http.Client
	apiKey  string
	bufSize int
	// initErr holds a configuration error detected in NewClient; it is
	// returned from the first Query so the constructor keeps one return value.
	initErr error
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// BaseURL is the trix-api root, e.g. "http://localhost:3739".
	BaseURL string
	// APIKey is sent as `Authorization: Bearer <key>` if non-empty.
	APIKey string
	// HTTPClient overrides the default http.Client. When nil, a client with
	// bounded connect + response-header timeouts is used (the SSE body is
	// never capped).
	HTTPClient *http.Client
	// EventBufferSize sets the channel buffer for streamed events. Default 64.
	EventBufferSize int
	// AllowInsecure permits a non-HTTPS BaseURL for a non-localhost host.
	// Off by default; loopback hosts (localhost, 127.0.0.1, ::1) are always
	// allowed over plain HTTP for local development.
	AllowInsecure bool
}

// NewClient constructs a Client. BaseURL is required and its configuration is
// validated here; any error (empty/invalid BaseURL, insecure transport to a
// remote host, or a missing APIKey for a remote host) is surfaced from the
// first Query call, keeping the constructor a single return value. When
// HTTPClient is nil a client with bounded connect + response-header timeouts is
// installed that deliberately does not cap the long-lived SSE response body.
func NewClient(opts ClientOptions) *Client {
	if opts.HTTPClient == nil {
		opts.HTTPClient = defaultHTTPClient()
	}
	bs := opts.EventBufferSize
	if bs <= 0 {
		bs = 64
	}
	return &Client{
		baseURL: opts.BaseURL,
		hc:      opts.HTTPClient,
		apiKey:  opts.APIKey,
		bufSize: bs,
		initErr: validateConfig(opts),
	}
}

// defaultHTTPClient bounds connection setup and time-to-first-byte without
// capping the long-lived SSE body: it sets a DialContext timeout and
// ResponseHeaderTimeout on the transport and deliberately leaves
// http.Client.Timeout unset (that would abort an open stream).
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// validateConfig rejects an empty, unparseable, or non-absolute BaseURL, then
// checks transport + credential posture.
func validateConfig(opts ClientOptions) error {
	if opts.BaseURL == "" {
		return errors.New("trix: BaseURL is required")
	}
	u, err := url.Parse(opts.BaseURL)
	if err != nil {
		return fmt.Errorf("trix: invalid BaseURL %q: %w", opts.BaseURL, err)
	}
	if !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("trix: BaseURL must be an absolute URL, got %q", opts.BaseURL)
	}
	return validatePosture(u, opts)
}

// validatePosture forbids plain HTTP to a remote host (unless AllowInsecure)
// and an empty APIKey to a remote host (which would hit the API unauthenticated).
func validatePosture(u *url.URL, opts ClientOptions) error {
	local := isLocalHost(u.Hostname())
	if u.Scheme != "https" && !local && !opts.AllowInsecure {
		return fmt.Errorf("trix: refusing non-HTTPS BaseURL %q for remote host (set AllowInsecure to override)", opts.BaseURL)
	}
	if opts.APIKey == "" && !local {
		return fmt.Errorf("trix: APIKey is required for non-localhost BaseURL %q", opts.BaseURL)
	}
	return nil
}

// isLocalHost reports whether host is a loopback name/address, for which
// insecure transport and anonymous access are tolerated (local development).
func isLocalHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// QueryOptions mirrors the wire body of POST /v1/agent/run (the documented
// RUN_BODY_SCHEMA fields of trix-api's agent-sdk route).
type QueryOptions struct {
	SessionID string `json:"sessionId,omitempty"`
	SpaceID   string `json:"spaceId,omitempty"`
	Model     string `json:"model,omitempty"`
	UserText  string `json:"userText,omitempty"`
	// SystemPrompt is the system-prompt block(s) for the run — the wire field
	// `systemPrompt` (a JSON array of strings). Omitted when empty.
	SystemPrompt []string `json:"systemPrompt,omitempty"`
	// LastEventID enables SSE resume: set it to the ID of the last StreamEvent
	// you received (see StreamEvent.ID) and the server replays only events after
	// it. Sent as the `Last-Event-ID` header; omitted when <= 0.
	LastEventID int64 `json:"-"`
}

// Query opens an SSE connection to /v1/agent/run and streams typed events.
// The returned channel is closed when the stream ends, errors, or ctx is done.
//
// `errCh` receives at most one error. If the request fails to open it is
// returned synchronously; mid-stream errors arrive on errCh asynchronously.
func (c *Client) Query(ctx context.Context, opts QueryOptions) (<-chan StreamEvent, <-chan error, error) {
	if c.initErr != nil {
		return nil, nil, c.initErr
	}
	body, err := json.Marshal(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal QueryOptions: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/agent/run", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if opts.LastEventID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(opts.LastEventID, 10))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Bound the error body so a hostile/huge response can't OOM the client.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // 64 KiB
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("agent run failed: %s — %s", resp.Status, string(body))
	}
	return c.streamFrom(ctx, resp.Body)
}

// streamFrom is the test seam: feed any io.ReadCloser shaped like an SSE
// response. It owns `body` and guarantees it is closed on every exit path —
// normal EOF, decode error, or ctx cancellation — reclaiming both worker
// goroutines and the underlying connection.
func (c *Client) streamFrom(ctx context.Context, body io.ReadCloser) (<-chan StreamEvent, <-chan error, error) {
	events := make(chan StreamEvent, c.bufSize)
	// Buffered for two potential senders (reader + decoder); sends are also
	// non-blocking so neither goroutine can ever wedge on errs.
	errs := make(chan error, 2)
	frames := make(chan SSEFrame, c.bufSize)

	var once sync.Once
	closeBody := func() { once.Do(func() { _ = body.Close() }) }
	stop := make(chan struct{})

	go watchCancel(ctx, stop, closeBody)
	go readFrames(body, frames, errs, stop, closeBody)
	go decodeFrames(ctx, frames, events, errs, closeBody)
	return events, errs, nil
}

// watchCancel closes the body when ctx is cancelled, unblocking any Read so the
// reader can finish. It exits once the reader signals completion via `stop`.
func watchCancel(ctx context.Context, stop <-chan struct{}, closeBody func()) {
	select {
	case <-ctx.Done():
		closeBody()
	case <-stop:
	}
}

// readFrames parses SSE frames from body into `frames` (ReadSSEFrames closes
// `frames` on return), then signals `stop` and closes the body.
func readFrames(body io.ReadCloser, frames chan<- SSEFrame, errs chan<- error, stop chan<- struct{}, closeBody func()) {
	defer closeBody()
	defer close(stop)
	if err := ReadSSEFrames(body, frames); err != nil {
		trySendErr(errs, err)
	}
}

// decodeFrames turns frames into typed events. Every send selects on ctx so a
// vanished consumer cannot pin the goroutine; on any early exit it closes the
// body and drains `frames` so the reader can finish instead of blocking on send.
func decodeFrames(ctx context.Context, frames <-chan SSEFrame, events chan<- StreamEvent, errs chan<- error, closeBody func()) {
	defer close(events)
	for frame := range frames {
		ev, err := DecodeFrameToEvent(frame)
		if err != nil {
			trySendErr(errs, fmt.Errorf("decode frame %d: %w", frame.ID, err))
			closeBody()
			discardFrames(frames)
			return
		}
		select {
		case events <- ev:
		case <-ctx.Done():
			closeBody()
			discardFrames(frames)
			return
		}
	}
}

// trySendErr delivers err without ever blocking. errs is buffered for both
// possible senders, so a full buffer just means an error is already in flight.
func trySendErr(errs chan<- error, err error) {
	select {
	case errs <- err:
	default:
	}
}

// discardFrames drops remaining frames so the reader goroutine can run to
// completion (close body, close frames) rather than blocking on a send.
func discardFrames(frames <-chan SSEFrame) {
	for range frames {
	}
}
