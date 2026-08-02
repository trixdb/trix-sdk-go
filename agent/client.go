package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
)

// Client is the consumer-facing handle for issuing Query() calls.
type Client struct {
	baseURL string
	hc      *http.Client
	apiKey  string
	bufSize int
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// BaseURL is the trix-api root, e.g. "http://localhost:3739".
	BaseURL string
	// APIKey is sent as `Authorization: Bearer <key>` if non-empty.
	APIKey string
	// HTTPClient overrides the default http.Client.
	HTTPClient *http.Client
	// EventBufferSize sets the channel buffer for streamed events. Default 64.
	EventBufferSize int
}

// NewClient constructs a Client. BaseURL is required.
func NewClient(opts ClientOptions) *Client {
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
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
	}
}

// QueryOptions mirrors trix-bots/src/sdk/query.ts QueryOptions.
type QueryOptions struct {
	SessionID string `json:"sessionId,omitempty"`
	SpaceID   string `json:"spaceId,omitempty"`
	Model     string `json:"model,omitempty"`
	UserText  string `json:"userText,omitempty"`
	// LastEventID enables SSE resume.
	LastEventID int64 `json:"-"`
}

// Query opens an SSE connection to /v1/agent/run and streams typed events.
// The returned channel is closed when the stream ends, errors, or ctx is done.
//
// `errCh` receives at most one error. If the request fails to open it is
// returned synchronously; mid-stream errors arrive on errCh asynchronously.
func (c *Client) Query(ctx context.Context, opts QueryOptions) (<-chan StreamEvent, <-chan error, error) {
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
		body, _ := io.ReadAll(resp.Body)
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
