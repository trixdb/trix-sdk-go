package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const fakeSSEStream = `id: 1
event: message_start
data: {"type":"message_start","message":{"id":"msg-1","model":"x"},"usage":{"input_tokens":1,"output_tokens":0}}

id: 2
event: content_delta
data: {"type":"content_delta","delta":"hello","contentIndex":0}

id: 3
event: stop
data: {"type":"stop","reason":"clear"}

`

func TestClientQueryStreamsEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/run" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing SSE Accept header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeSSEStream))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	events, errs, err := c.Query(context.Background(), QueryOptions{SessionID: "s1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	got := []StreamEvent{}
	for ev := range events {
		got = append(got, ev)
	}
	select {
	case e := <-errs:
		if e != nil {
			t.Fatalf("stream error: %v", e)
		}
	default:
	}
	if len(got) != 3 {
		t.Fatalf("got %d events; want 3", len(got))
	}
	if got[0].Type != EventTypeMessageStart || got[2].Type != EventTypeStop {
		t.Errorf("event order/types wrong")
	}
}

func TestClientForwardsAPIKey(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(""))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(ClientOptions{BaseURL: srv.URL, APIKey: "sk-test"})
	_, _, _ = c.Query(context.Background(), QueryOptions{})
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header = %q", gotAuth)
	}
}

func TestClientForwardsLastEventID(t *testing.T) {
	gotID := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("Last-Event-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(ClientOptions{BaseURL: srv.URL})
	_, _, _ = c.Query(context.Background(), QueryOptions{LastEventID: 42})
	if gotID != "42" {
		t.Errorf("last-event-id header = %q", gotID)
	}
}

func TestClientReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(ClientOptions{BaseURL: srv.URL})
	_, _, err := c.Query(context.Background(), QueryOptions{})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
