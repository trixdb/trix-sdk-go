package agent

import (
	"strings"
	"testing"
)

const sampleNDJSON = `{"seq":1,"event":{"type":"message_start","message":{"id":"m","model":"x"},"usage":{"input_tokens":1,"output_tokens":0}}}
{"seq":2,"event":{"type":"content_delta","delta":"hello","contentIndex":0}}
{"seq":3,"event":{"type":"stop","reason":"clear"}}
`

func TestParseFixture(t *testing.T) {
	lines, err := ParseFixtureString(sampleNDJSON)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines; want 3", len(lines))
	}
	if lines[0].Seq != 1 || lines[0].Event.Type != EventTypeMessageStart {
		t.Errorf("line 0 wrong: %+v", lines[0])
	}
	if lines[2].Event.Type != EventTypeStop {
		t.Errorf("line 2 type = %s", lines[2].Event.Type)
	}
}

func TestParseFixtureSkipsBlankLines(t *testing.T) {
	raw := "{\"seq\":1,\"event\":{\"type\":\"stop\",\"reason\":\"clear\"}}\n\n   \n"
	lines, err := ParseFixtureString(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}
}

func TestParseFixtureRejectsMalformed(t *testing.T) {
	_, err := ParseFixtureString("{not json\n")
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestReplay(t *testing.T) {
	lines, _ := ParseFixtureString(sampleNDJSON)
	out := make(chan StreamEvent, len(lines))
	Replay(lines, out)
	got := []StreamEvent{}
	for ev := range out {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events; want 3", len(got))
	}
}

func TestCompareFixturesEqual(t *testing.T) {
	lines, _ := ParseFixtureString(sampleNDJSON)
	a := []StreamEvent{lines[0].Event, lines[1].Event, lines[2].Event}
	b := []StreamEvent{lines[0].Event, lines[1].Event, lines[2].Event}
	if err := CompareFixtures(a, b); err != nil {
		t.Errorf("expected equal: %v", err)
	}
}

func TestCompareFixturesIgnoresMessageID(t *testing.T) {
	a := []StreamEvent{{Type: EventTypeMessageStart, MessageStart: &MessageStartPayload{
		Message: AssistantMessageRef{ID: "msg-A", Model: "x"},
		Usage:   Usage{InputTokens: 1},
	}}}
	b := []StreamEvent{{Type: EventTypeMessageStart, MessageStart: &MessageStartPayload{
		Message: AssistantMessageRef{ID: "msg-DIFFERENT", Model: "x"},
		Usage:   Usage{InputTokens: 1},
	}}}
	if err := CompareFixtures(a, b); err != nil {
		t.Errorf("expected equal (ID ignored): %v", err)
	}
}

func TestCompareFixturesReportsLengthMismatch(t *testing.T) {
	a := []StreamEvent{{Type: EventTypeStop, Stop: &StopPayload{Reason: "x"}}}
	b := []StreamEvent{}
	if err := CompareFixtures(a, b); err == nil || !strings.Contains(err.Error(), "length differs") {
		t.Errorf("expected length-differs, got: %v", err)
	}
}

func TestCompareFixturesReportsTypeMismatch(t *testing.T) {
	a := []StreamEvent{{Type: EventTypeStop, Stop: &StopPayload{Reason: "x"}}}
	b := []StreamEvent{{Type: EventTypeError, Error: &ErrorPayload{}}}
	if err := CompareFixtures(a, b); err == nil || !strings.Contains(err.Error(), "type differs") {
		t.Errorf("expected type-differs, got: %v", err)
	}
}

func TestCompareFixturesReportsValueMismatch(t *testing.T) {
	a := []StreamEvent{{Type: EventTypeContentDelta, ContentDelta: &ContentDeltaPayload{Delta: "aaa"}}}
	b := []StreamEvent{{Type: EventTypeContentDelta, ContentDelta: &ContentDeltaPayload{Delta: "bbb"}}}
	if err := CompareFixtures(a, b); err == nil || !strings.Contains(err.Error(), "payload differs") {
		t.Errorf("expected payload-differs, got: %v", err)
	}
}
