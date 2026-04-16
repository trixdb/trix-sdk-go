package agent

import (
	"strings"
	"testing"
)

func TestReadSSEFrames(t *testing.T) {
	raw := `id: 1
event: content_delta
data: {"type":"content_delta","delta":"hi","contentIndex":0}

id: 2
event: stop
data: {"type":"stop","reason":"clear"}

`
	out := make(chan SSEFrame, 8)
	if err := ReadSSEFrames(strings.NewReader(raw), out); err != nil {
		t.Fatalf("read: %v", err)
	}
	frames := drainFrames(out)
	if len(frames) != 2 {
		t.Fatalf("got %d frames want 2", len(frames))
	}
	if frames[0].ID != 1 || frames[0].Event != "content_delta" {
		t.Errorf("frame 0 wrong: %+v", frames[0])
	}
	if frames[1].ID != 2 || frames[1].Event != "stop" {
		t.Errorf("frame 1 wrong: %+v", frames[1])
	}
}

func TestReadSSEFramesIgnoresHeartbeats(t *testing.T) {
	raw := `:hb

id: 1
event: stop
data: {"type":"stop","reason":"clear"}

:another

`
	out := make(chan SSEFrame, 8)
	_ = ReadSSEFrames(strings.NewReader(raw), out)
	frames := drainFrames(out)
	if len(frames) != 1 {
		t.Fatalf("got %d frames; want 1 (heartbeats ignored)", len(frames))
	}
}

func TestReadSSEFramesMultiLineData(t *testing.T) {
	raw := `event: x
data: line1
data: line2

`
	out := make(chan SSEFrame, 8)
	_ = ReadSSEFrames(strings.NewReader(raw), out)
	frames := drainFrames(out)
	if len(frames) != 1 {
		t.Fatalf("got %d frames want 1", len(frames))
	}
	if frames[0].Data != "line1\nline2" {
		t.Errorf("multi-line data not joined: %q", frames[0].Data)
	}
}

func TestReadSSEFramesFlushesPendingAtEOF(t *testing.T) {
	// No trailing blank line.
	raw := `event: stop
data: {"type":"stop","reason":"clear"}`
	out := make(chan SSEFrame, 8)
	_ = ReadSSEFrames(strings.NewReader(raw), out)
	frames := drainFrames(out)
	if len(frames) != 1 {
		t.Fatalf("expected pending frame to flush at EOF, got %d frames", len(frames))
	}
}

func TestDecodeFrameToEvent(t *testing.T) {
	frame := SSEFrame{
		ID:    7,
		Event: "memory_cited",
		Data:  `{"type":"memory_cited","memoryId":"mem-7"}`,
	}
	ev, err := DecodeFrameToEvent(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Type != EventTypeMemoryCited {
		t.Errorf("type = %s", ev.Type)
	}
	if ev.MemoryCited == nil || ev.MemoryCited.MemoryID != "mem-7" {
		t.Errorf("payload not populated")
	}
}

func drainFrames(c <-chan SSEFrame) []SSEFrame {
	out := []SSEFrame{}
	for f := range c {
		out = append(out, f)
	}
	return out
}
