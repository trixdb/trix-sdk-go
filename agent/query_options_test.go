package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestQueryOptionsWireBody proves QueryOptions serialises to exactly the
// documented RUN_BODY_SCHEMA fields: systemPrompt is sent as a JSON array,
// unset fields are omitted, and LastEventID never appears in the body (it
// travels as the Last-Event-ID header instead).
func TestQueryOptionsWireBody(t *testing.T) {
	var gotBody map[string]any
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		gotHeader = r.Header.Get("Last-Event-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	events, _, err := c.Query(context.Background(), QueryOptions{
		SessionID:    "s1",
		SpaceID:      "space-A",
		UserText:     "hi",
		SystemPrompt: []string{"be terse", "cite memories"},
		LastEventID:  7,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range events { //nolint:revive // drain so the request completes
	}

	sp, ok := gotBody["systemPrompt"].([]any)
	if !ok || len(sp) != 2 || sp[0] != "be terse" || sp[1] != "cite memories" {
		t.Errorf("systemPrompt not sent as a 2-element array: %#v", gotBody["systemPrompt"])
	}
	if _, present := gotBody["lastEventId"]; present {
		t.Errorf("LastEventID must not appear in the body: %#v", gotBody)
	}
	if _, present := gotBody["LastEventID"]; present {
		t.Errorf("LastEventID must not appear in the body: %#v", gotBody)
	}
	if gotHeader != "7" {
		t.Errorf("Last-Event-ID header = %q; want 7", gotHeader)
	}
}

// TestQueryOptionsOmitsEmptySystemPrompt proves an unset SystemPrompt is omitted
// (omitempty) rather than sent as null/[].
func TestQueryOptionsOmitsEmptySystemPrompt(t *testing.T) {
	b, err := json.Marshal(QueryOptions{UserText: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := m["systemPrompt"]; present {
		t.Errorf("empty SystemPrompt should be omitted, got: %s", b)
	}
}
