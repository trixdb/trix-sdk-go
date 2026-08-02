// Integration test for the Retrix-style embedding pattern.
//
// Spins up an httptest SSE server emitting a realistic memory-native
// research-v1 transcript, embeds the SDK behind the same `EmbeddedAgent`
// facade a downstream consumer would, and asserts the assembled
// `RunOutcome` reflects what flowed across the wire.

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trixdb/trix-sdk-go/agent"
)

// A realistic memory-native turn: retrieval → injection → assistant cites a
// memory mid-stream → consolidation reports → stop.
const memoryNativeTranscript = `id: 1
event: memory_retrieved
data: {"type":"memory_retrieved","memoryIds":["mem-prefs","mem-style"],"latencyMs":34,"timedOut":false}

id: 2
event: message_start
data: {"type":"message_start","message":{"id":"msg-1","model":"claude-sonnet-4-6"},"usage":{"input_tokens":420,"output_tokens":0}}

id: 3
event: content_delta
data: {"type":"content_delta","delta":"Per saved preferences ","contentIndex":0}

id: 4
event: memory_cited
data: {"type":"memory_cited","memoryId":"mem-prefs"}

id: 5
event: content_delta
data: {"type":"content_delta","delta":"the answer is concise: 42.","contentIndex":0}

id: 6
event: message_stop
data: {"type":"message_stop","stopReason":"end_turn","usage":{"input_tokens":420,"output_tokens":12}}

id: 7
event: budget_state
data: {"type":"budget_state","spent":{"tokens":432,"usd":0.0042},"band":"ok","pct":0.04}

id: 8
event: stop
data: {"type":"stop","reason":"clear"}

`

func newMockSSEServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/run" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}

func TestEmbedMemoryNativeTurn(t *testing.T) {
	srv := newMockSSEServer(t, memoryNativeTranscript)
	t.Cleanup(srv.Close)

	a := NewEmbeddedAgent(srv.URL, "", &bytes.Buffer{})
	outcome, err := a.Run(context.Background(), agent.QueryOptions{
		SpaceID:  "space-A",
		UserText: "what is the answer?",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Assistant text reconstructed from streamed deltas.
	want := "Per saved preferences the answer is concise: 42."
	if outcome.AssistantText != want {
		t.Errorf("assistant text mismatch:\n  want %q\n  got  %q", want, outcome.AssistantText)
	}

	// Memory citation surfaced into outcome.
	if len(outcome.CitedMemoryIDs) != 1 || outcome.CitedMemoryIDs[0] != "mem-prefs" {
		t.Errorf("cited memories: %v", outcome.CitedMemoryIDs)
	}

	// Budget state captured.
	if outcome.BudgetSpent.Tokens != 432 {
		t.Errorf("budget tokens = %d", outcome.BudgetSpent.Tokens)
	}
	if outcome.BudgetSpent.USD == 0 {
		t.Errorf("budget USD should be non-zero")
	}

	// Final stop reason recorded.
	if outcome.StopReason != "clear" {
		t.Errorf("stop reason = %q", outcome.StopReason)
	}
}

func TestEmbedSurfacesAuth(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(memoryNativeTranscript))
	}))
	t.Cleanup(srv.Close)
	a := NewEmbeddedAgent(srv.URL, "sk-trix-test", &bytes.Buffer{})
	_, err := a.Run(context.Background(), agent.QueryOptions{UserText: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotAuth != "Bearer sk-trix-test" {
		t.Errorf("auth header = %q", gotAuth)
	}
}

func TestEmbedReturnsErrorOnAgentError(t *testing.T) {
	body := `id: 1
event: error
data: {"type":"error","error":{"code":"stream_failure","message":"upstream 529"}}

id: 2
event: stop
data: {"type":"stop","reason":"error"}

`
	srv := newMockSSEServer(t, body)
	t.Cleanup(srv.Close)
	a := NewEmbeddedAgent(srv.URL, "", &bytes.Buffer{})
	_, err := a.Run(context.Background(), agent.QueryOptions{UserText: "hi"})
	if err == nil || !strings.Contains(err.Error(), "stream_failure") {
		t.Errorf("expected error containing stream_failure; got %v", err)
	}
}

func TestEmbedReturnsErrorOnHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	t.Cleanup(srv.Close)
	a := NewEmbeddedAgent(srv.URL, "wrong-key", &bytes.Buffer{})
	_, err := a.Run(context.Background(), agent.QueryOptions{UserText: "hi"})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestEmbedRespectsContextCancellation(t *testing.T) {
	// Slow server: write only after a long delay, so client can cancel first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	a := NewEmbeddedAgent(srv.URL, "", &bytes.Buffer{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = a.Run(ctx, agent.QueryOptions{UserText: "slow"})
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("Run should have returned quickly after context timeout")
	}
}

func TestEmbedHandlesEmptyResponse(t *testing.T) {
	srv := newMockSSEServer(t, "")
	t.Cleanup(srv.Close)
	a := NewEmbeddedAgent(srv.URL, "", &bytes.Buffer{})
	outcome, err := a.Run(context.Background(), agent.QueryOptions{UserText: "hi"})
	if err != nil {
		t.Fatalf("empty response should not error: %v", err)
	}
	if outcome.AssistantText != "" {
		t.Errorf("expected empty assistant text; got %q", outcome.AssistantText)
	}
}
