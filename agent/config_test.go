package agent

import (
	"context"
	"net/http"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name    string
		opts    ClientOptions
		wantErr bool
	}{
		{"empty baseurl rejected", ClientOptions{}, true},
		{"relative baseurl rejected", ClientOptions{BaseURL: "/v1/agent"}, true},
		{"missing scheme rejected", ClientOptions{BaseURL: "localhost:3739"}, true},
		{"unparseable rejected", ClientOptions{BaseURL: "://nope"}, true},
		{"http remote rejected", ClientOptions{BaseURL: "http://api.example.com", APIKey: "k"}, true},
		{"http remote with AllowInsecure accepted", ClientOptions{BaseURL: "http://api.example.com", APIKey: "k", AllowInsecure: true}, false},
		{"https remote with key accepted", ClientOptions{BaseURL: "https://api.example.com", APIKey: "k"}, false},
		{"https remote empty key rejected", ClientOptions{BaseURL: "https://api.example.com"}, true},
		{"http localhost accepted", ClientOptions{BaseURL: "http://127.0.0.1:3739"}, false},
		{"http localhost name accepted", ClientOptions{BaseURL: "http://localhost:3739"}, false},
		{"http ipv6 loopback accepted", ClientOptions{BaseURL: "http://[::1]:3739"}, false},
		{"empty key on localhost accepted", ClientOptions{BaseURL: "http://localhost:3739"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(tc.opts)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateConfig(%+v): err=%v, wantErr=%v", tc.opts, err, tc.wantErr)
			}
		})
	}
}

// TestQuerySurfacesConfigError proves the stored initErr is returned
// synchronously from Query (an invalid BaseURL: plain HTTP to a remote host).
func TestQuerySurfacesConfigError(t *testing.T) {
	c := NewClient(ClientOptions{BaseURL: "http://api.example.com", APIKey: "k"})
	_, _, err := c.Query(context.Background(), QueryOptions{})
	if err == nil {
		t.Fatal("expected config error surfaced from Query")
	}
}

// TestQueryRejectsEmptyKeyForRemote guards against silently hitting a remote
// API with no Authorization header.
func TestQueryRejectsEmptyKeyForRemote(t *testing.T) {
	c := NewClient(ClientOptions{BaseURL: "https://api.example.com"})
	_, _, err := c.Query(context.Background(), QueryOptions{})
	if err == nil {
		t.Fatal("expected error for empty API key against a remote host")
	}
}

// TestDefaultHTTPClientBoundsWithoutCappingBody verifies the default client
// bounds connect + first byte but never caps the SSE body via Client.Timeout.
func TestDefaultHTTPClientBoundsWithoutCappingBody(t *testing.T) {
	c := NewClient(ClientOptions{BaseURL: "https://api.example.com", APIKey: "k"})
	if c.hc.Timeout != 0 {
		t.Errorf("http.Client.Timeout must stay 0 (would cap the SSE body), got %v", c.hc.Timeout)
	}
	tr, ok := c.hc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.hc.Transport)
	}
	if tr.ResponseHeaderTimeout == 0 {
		t.Error("ResponseHeaderTimeout should be set to bound time-to-first-byte")
	}
	if tr.DialContext == nil {
		t.Error("DialContext should be set to bound connection setup")
	}
}
