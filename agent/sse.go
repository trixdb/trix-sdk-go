package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// SSEFrame is one decoded server-sent-events block.
type SSEFrame struct {
	ID    int64
	Event string
	Data  string
}

// maxFrameDataBytes caps the total size of one frame's accumulated `data:`
// lines, bounding the memory a hostile/buggy server can force the client to
// buffer (the scanner already caps a single line at 1 MiB).
const maxFrameDataBytes = 8 << 20 // 8 MiB

// ReadSSEFrames consumes an SSE stream from `r` and returns frames via `out`.
// Closes `out` when the reader yields io.EOF or any other error.
//
// Heartbeats (`:hb` or any line starting with `:`) are silently dropped.
//
// Returns the last error observed (nil on EOF).
func ReadSSEFrames(r io.Reader, out chan<- SSEFrame) error {
	defer close(out)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	cur := SSEFrame{ID: -1}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Empty line terminates the frame.
			if cur.Event != "" || cur.Data != "" {
				out <- cur
			}
			cur = SSEFrame{ID: -1}
			continue
		}
		if strings.HasPrefix(line, ":") {
			// Comment / heartbeat — drop.
			continue
		}
		switch {
		case strings.HasPrefix(line, "id:"):
			s := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				cur.ID = n
			}
		case strings.HasPrefix(line, "event:"):
			cur.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			// Per spec, multiple data: lines are joined with newline.
			body := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if len(cur.Data)+len(body)+1 > maxFrameDataBytes {
				return fmt.Errorf("sse frame exceeds %d bytes", maxFrameDataBytes)
			}
			if cur.Data == "" {
				cur.Data = body
			} else {
				cur.Data += "\n" + body
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	// Flush any in-progress frame at EOF (most servers terminate with a blank line).
	if cur.Event != "" || cur.Data != "" {
		out <- cur
	}
	return nil
}

// DecodeFrameToEvent converts an SSEFrame's data field into a typed StreamEvent
// and carries the frame's `id:` through onto StreamEvent.ID (the resume cursor),
// so a caller can learn where to resume from. A frame with no id (sentinel -1)
// leaves ID at its zero value.
func DecodeFrameToEvent(frame SSEFrame) (StreamEvent, error) {
	var ev StreamEvent
	if err := json.Unmarshal([]byte(frame.Data), &ev); err != nil {
		return ev, err
	}
	if frame.ID >= 0 {
		ev.ID = frame.ID
	}
	return ev, nil
}
