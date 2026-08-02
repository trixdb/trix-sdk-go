package agent

import (
	"strings"
	"testing"
)

// A single SSE frame whose accumulated data: lines exceed maxFrameDataBytes must
// be rejected rather than buffered without bound (memory-DoS guard, issue #4).
func TestReadSSEFramesBoundsFrameSize(t *testing.T) {
	// Each line stays under the 1 MiB scanner cap; ~10 * 900 KB > 8 MiB, with no
	// terminating blank line, so it all accumulates into one frame.
	line := "data:" + strings.Repeat("x", 900_000) + "\n"
	raw := strings.Repeat(line, 10)

	out := make(chan SSEFrame, 4)
	go func() { //nolint:revive // drain so a partial emit can't block
		for range out {
		}
	}()

	err := ReadSSEFrames(strings.NewReader(raw), out)
	if err == nil {
		t.Fatal("expected an error for an oversize frame, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected an 'exceeds' error, got: %v", err)
	}
}
