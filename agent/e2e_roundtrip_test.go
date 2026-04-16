package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestE2E_SseRoundTrip_FromTsFixture is a cross-language end-to-end test that
// closes the loop between the TS and Go SDKs:
//
//  1. A TS-generated NDJSON fixture (canonical contract under
//     trix-sdk-go/testdata/cross-lang-fixture.ndjson) is loaded.
//  2. An httptest server re-serialises each line as a TS-style SSE frame —
//     `id: N\nevent: T\ndata: {json}\n\n` — the exact wire format trix-api
//     emits from streamSse.
//  3. The Go SDK's Client.Query() consumes the stream.
//  4. Every received StreamEvent is reserialised through the Go Marshal path
//     and compared against the input fixture with messageId/id elided.
//
// If the wire byte-for-byte contract between TS `streamSse` and Go
// `ReadSSEFrames` ever drifts, this test fails.
func TestE2E_SseRoundTrip_FromTsFixture(t *testing.T) {
	fixture := filepath.Join("..", "testdata", "cross-lang-fixture.ndjson")
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := ParseFixtureString(string(raw))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		flusher, _ := w.(http.Flusher)
		for i, line := range parsed {
			data, err := json.Marshal(line.Event)
			if err != nil {
				t.Errorf("marshal event %d: %v", i, err)
				return
			}
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", i+1, line.Event.Type, data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{BaseURL: srv.URL})
	received := make([]StreamEvent, 0, len(parsed))
	evCh, errCh, err := client.Query(context.Background(), QueryOptions{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for ev := range evCh {
		received = append(received, ev)
	}
	select {
	case streamErr := <-errCh:
		if streamErr != nil {
			t.Fatalf("stream error: %v", streamErr)
		}
	default:
	}

	if len(received) != len(parsed) {
		t.Fatalf("received %d events; want %d", len(received), len(parsed))
	}
	for i, ev := range received {
		if ev.Type != parsed[i].Event.Type {
			t.Errorf("event %d type: got %s, want %s", i, ev.Type, parsed[i].Event.Type)
		}
	}

	// CompareFixtures takes []StreamEvent; lift from FixtureLine → StreamEvent.
	fixtureEvents := make([]StreamEvent, len(parsed))
	for i, line := range parsed {
		fixtureEvents[i] = line.Event
	}
	if err := CompareFixtures(fixtureEvents, received); err != nil {
		t.Fatalf("structural mismatch after roundtrip: %v", err)
	}
}

// TestE2E_ResumeGapHonoured verifies the Go client forwards Last-Event-ID and
// the server's gap-skip works over real HTTP.
func TestE2E_ResumeGapHonoured(t *testing.T) {
	fixture := filepath.Join("..", "testdata", "cross-lang-fixture.ndjson")
	raw, _ := os.ReadFile(fixture)
	parsed, _ := ParseFixtureString(string(raw))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := 0
		if lei := r.Header.Get("Last-Event-ID"); lei != "" {
			fmt.Sscanf(lei, "%d", &start)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i, line := range parsed {
			seq := i + 1
			if seq <= start {
				continue
			}
			data, _ := json.Marshal(line.Event)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", seq, line.Event.Type, data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{BaseURL: srv.URL})
	evCh, errCh, err := client.Query(context.Background(), QueryOptions{LastEventID: 3})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	count := 0
	for range evCh {
		count++
	}
	select {
	case streamErr := <-errCh:
		if streamErr != nil {
			t.Fatalf("stream error: %v", streamErr)
		}
	default:
	}
	if want := len(parsed) - 3; count != want {
		t.Fatalf("got %d events after resume; want %d", count, want)
	}
}
