package agent

import (
	"context"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

// recordingBody wraps an io.PipeReader and records when Close is called, so a
// test can assert the SDK reclaimed the connection.
type recordingBody struct {
	pr     *io.PipeReader
	once   sync.Once
	closed chan struct{}
}

func (b *recordingBody) Read(p []byte) (int, error) { return b.pr.Read(p) }

func (b *recordingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.pr.Close()
}

// TestStreamFromReclaimsResourcesOnCancel is a regression test for the
// goroutine + connection leak: a consumer reads one event, abandons the stream,
// and cancels ctx. The body must be closed and both worker goroutines must
// return to baseline. Against the pre-fix streamFrom (no ctx, blocking sends)
// the pipeline wedges on a full buffer and the body is never closed.
func TestStreamFromReclaimsResourcesOnCancel(t *testing.T) {
	base := runtime.NumGoroutine()

	pr, pw := io.Pipe()
	body := &recordingBody{pr: pr, closed: make(chan struct{})}

	// Infinite producer: keeps emitting frames until the reader closes the
	// pipe. Without ctx-aware sends the whole pipeline stalls with full
	// buffers and the body is leaked.
	go func() {
		const frame = "id: 1\nevent: content_delta\n" +
			`data: {"type":"content_delta","delta":"x","contentIndex":0}` + "\n\n"
		for {
			if _, err := io.WriteString(pw, frame); err != nil {
				return
			}
		}
	}()

	c := NewClient(ClientOptions{BaseURL: "http://localhost:1"})
	ctx, cancel := context.WithCancel(context.Background())
	events, errs, err := c.streamFrom(ctx, body)
	if err != nil {
		t.Fatalf("streamFrom: %v", err)
	}

	// Consume exactly one event, then abandon the stream mid-flight.
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("never received first event")
	}
	cancel()

	// Cancellation must close the body (reclaim the connection).
	select {
	case <-body.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("body was not closed after ctx cancellation (connection leak)")
	}

	// Drain the tail so the decoder can finish closing `events`.
	for range events {
	}
	select {
	case <-errs:
	default:
	}

	assertGoroutinesReturn(t, base)
}

// assertGoroutinesReturn polls until the live goroutine count settles back to
// baseline (with a little scheduler slack), failing if the workers leaked.
func assertGoroutinesReturn(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= base+1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines leaked: have %d, want <= %d", runtime.NumGoroutine(), base+1)
}
