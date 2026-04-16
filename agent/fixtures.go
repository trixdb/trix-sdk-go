package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// FixtureLine matches the TS NDJSON shape:
//
//	{ "seq": <int>, "event": <StreamEvent> }
type FixtureLine struct {
	Seq   int64       `json:"seq"`
	Event StreamEvent `json:"event"`
}

// ParseFixture reads NDJSON fixture lines from `r`. Blank lines are skipped.
func ParseFixture(r io.Reader) ([]FixtureLine, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := []FixtureLine{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var fl FixtureLine
		if err := json.Unmarshal([]byte(line), &fl); err != nil {
			return nil, fmt.Errorf("parse fixture line %q: %w", line, err)
		}
		out = append(out, fl)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseFixtureString is a convenience wrapper.
func ParseFixtureString(raw string) ([]FixtureLine, error) {
	return ParseFixture(strings.NewReader(raw))
}

// Replay yields fixture events on a channel; closes the channel when done.
// Useful for swapping a real Client for deterministic test inputs.
func Replay(lines []FixtureLine, out chan<- StreamEvent) {
	defer close(out)
	for _, l := range lines {
		out <- l.Event
	}
}

// CompareFixtures returns nil when two event lists agree, ignoring fields
// that are inherently nondeterministic (`MessageStart.Message.ID`).
func CompareFixtures(a, b []StreamEvent) error {
	if len(a) != len(b) {
		return fmt.Errorf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Type != b[i].Type {
			return fmt.Errorf("event[%d] type differs: %s vs %s", i, a[i].Type, b[i].Type)
		}
		if err := compareEvent(i, a[i], b[i]); err != nil {
			return err
		}
	}
	return nil
}

func compareEvent(i int, a, b StreamEvent) error {
	// Marshal both with the nondeterministic id zeroed out.
	aj, err := json.Marshal(stripNondeterministic(a))
	if err != nil {
		return err
	}
	bj, err := json.Marshal(stripNondeterministic(b))
	if err != nil {
		return err
	}
	if string(aj) != string(bj) {
		return fmt.Errorf("event[%d] payload differs:\n  a=%s\n  b=%s", i, aj, bj)
	}
	return nil
}

// stripNondeterministic returns a shallow copy with id-like fields blanked.
func stripNondeterministic(e StreamEvent) StreamEvent {
	if e.MessageStart != nil {
		ms := *e.MessageStart
		ms.Message.ID = ""
		e.MessageStart = &ms
	}
	if e.ToolUseStart != nil {
		t := *e.ToolUseStart
		t.ToolUseID = ""
		e.ToolUseStart = &t
	}
	if e.ToolUseDelta != nil {
		t := *e.ToolUseDelta
		t.ToolUseID = ""
		e.ToolUseDelta = &t
	}
	if e.ToolResult != nil {
		t := *e.ToolResult
		t.ToolUseID = ""
		e.ToolResult = &t
	}
	return e
}
