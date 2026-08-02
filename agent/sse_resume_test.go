package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// contentDeltaJSON is a minimal valid content_delta payload reused below.
const contentDeltaJSON = `{"type":"content_delta","delta":"x","contentIndex":0}`

// TestDecodeFrameToEventCapturesID proves the frame parser + decoder carry the
// SSE `id:` through onto StreamEvent.ID (the resume cursor). Before the fix the
// id was parsed into SSEFrame.ID then dropped, so a caller could never learn it.
func TestDecodeFrameToEventCapturesID(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		wantID int64
	}{
		{"numeric id", "id: 1\nevent: content_delta\ndata: " + contentDeltaJSON + "\n\n", 1},
		{"large id", "id: 987654321\nevent: content_delta\ndata: " + contentDeltaJSON + "\n\n", 987654321},
		{"no id line", "event: content_delta\ndata: " + contentDeltaJSON + "\n\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := make(chan SSEFrame, 4)
			if err := ReadSSEFrames(strings.NewReader(tc.raw), out); err != nil {
				t.Fatalf("read frames: %v", err)
			}
			frames := drainFrames(out)
			if len(frames) != 1 {
				t.Fatalf("got %d frames; want 1", len(frames))
			}
			ev, err := DecodeFrameToEvent(frames[0])
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if ev.ID != tc.wantID {
				t.Errorf("ev.ID = %d; want %d", ev.ID, tc.wantID)
			}
		})
	}
}

// TestResumeReSendsCapturedID closes the loop end-to-end: the client streams a
// run, the caller tracks the last StreamEvent.ID it saw, and on reconnect that
// tracked id is forwarded as the `Last-Event-ID` header. This is the exact
// resume flow that was unusable while received ids were never surfaced.
func TestResumeReSendsCapturedID(t *testing.T) {
	var mu sync.Mutex
	resumeHeader := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		lei := r.Header.Get("Last-Event-ID")
		if lei == "" { // first connection: emit ids 1..3
			for i := 1; i <= 3; i++ {
				fmt.Fprintf(w, "id: %d\nevent: content_delta\ndata: %s\n\n", i, contentDeltaJSON)
			}
			return
		}
		mu.Lock() // reconnect: record what the client replayed
		resumeHeader = lei
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	last := streamLastID(t, c, QueryOptions{})
	if last != 3 {
		t.Fatalf("captured last id = %d; want 3", last)
	}

	// Reconnect with the tracked id; the drain guarantees the request completed.
	_ = streamLastID(t, c, QueryOptions{LastEventID: last})
	mu.Lock()
	got := resumeHeader
	mu.Unlock()
	if got != "3" {
		t.Errorf("reconnect Last-Event-ID = %q; want %q", got, "3")
	}
}

// streamLastID runs one Query to completion and returns the last event id seen.
func streamLastID(t *testing.T, c *Client, opts QueryOptions) int64 {
	t.Helper()
	evCh, errCh, err := c.Query(context.Background(), opts)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var last int64
	for ev := range evCh {
		last = ev.ID
	}
	select {
	case streamErr := <-errCh:
		if streamErr != nil {
			t.Fatalf("stream error: %v", streamErr)
		}
	default:
	}
	return last
}
