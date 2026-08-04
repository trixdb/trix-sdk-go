package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestQueryReturnsTypedAPIError proves a non-2xx surfaces a *APIError carrying
// the status code and body, so callers can branch on StatusCode instead of
// string-matching.
func TestQueryReturnsTypedAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	_, _, err := c.Query(context.Background(), QueryOptions{})
	if err == nil {
		t.Fatal("expected error on 429")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d; want 429", apiErr.StatusCode)
	}
	if apiErr.Body != "slow down" {
		t.Errorf("Body = %q; want %q", apiErr.Body, "slow down")
	}
}

// TestQueryRejectsNonEventStream proves a 200 with a non-SSE Content-Type is a
// loud error (matched via errors.Is) rather than a silent empty stream.
func TestQueryRejectsNonEventStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"gateway misrouted"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	_, _, err := c.Query(context.Background(), QueryOptions{})
	if err == nil {
		t.Fatal("expected error for non-event-stream 200")
	}
	if !errors.Is(err, ErrNotEventStream) {
		t.Fatalf("expected ErrNotEventStream, got %v", err)
	}
}

// TestQueryAcceptsEventStreamWithCharset proves the parameterised media type the
// live API sends ("text/event-stream; charset=utf-8") is accepted.
func TestQueryAcceptsEventStreamWithCharset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("id: 1\nevent: stop\ndata: {\"type\":\"stop\",\"reason\":\"clear\"}\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	events, errs, err := c.Query(context.Background(), QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	n := 0
	for range events {
		n++
	}
	select {
	case e := <-errs:
		if e != nil {
			t.Fatalf("stream error: %v", e)
		}
	default:
	}
	if n != 1 {
		t.Errorf("got %d events; want 1", n)
	}
}

// TestQueryAcceptsMissingContentType documents the lenient branch: a 200 with no
// Content-Type header still streams (only a *wrong* type is rejected).
func TestQueryAcceptsMissingContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil // suppress Go's default sniffed type
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("id: 1\nevent: stop\ndata: {\"type\":\"stop\",\"reason\":\"clear\"}\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientOptions{BaseURL: srv.URL})
	events, _, err := c.Query(context.Background(), QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	n := 0
	for range events {
		n++
	}
	if n != 1 {
		t.Errorf("got %d events; want 1 (missing Content-Type is tolerated)", n)
	}
}

func TestIsEventStream(t *testing.T) {
	cases := []struct {
		ct   string
		want bool
	}{
		{"text/event-stream", true},
		{"text/event-stream; charset=utf-8", true},
		{"Text/Event-Stream", true},
		{"", true}, // lenient
		{"application/json", false},
		{"text/html; charset=utf-8", false},
		{"garbage;;;", false},
	}
	for _, tc := range cases {
		if got := isEventStream(tc.ct); got != tc.want {
			t.Errorf("isEventStream(%q) = %v; want %v", tc.ct, got, tc.want)
		}
	}
}

// TestAPIErrorMessage covers the with/without-body message forms.
func TestAPIErrorMessage(t *testing.T) {
	withBody := (&APIError{StatusCode: 401, Status: "401 Unauthorized", Body: "bad key"}).Error()
	if !strings.Contains(withBody, "401 Unauthorized") || !strings.Contains(withBody, "bad key") {
		t.Errorf("with-body message missing parts: %q", withBody)
	}
	noBody := (&APIError{StatusCode: 500, Status: "500 Internal Server Error"}).Error()
	if !strings.Contains(noBody, "500 Internal Server Error") || strings.Contains(noBody, "—") {
		t.Errorf("no-body message wrong: %q", noBody)
	}
}
