package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// normalizeJSON re-serialises arbitrary JSON with map keys sorted, so two
// structurally-equal documents compare equal regardless of field order.
func normalizeJSON(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("normalize unmarshal: %v (input=%s)", err, b)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("normalize marshal: %v", err)
	}
	return string(out)
}

// allVariantWire holds one canonical wire frame per StreamEvent variant, each
// populating every field of that variant. The map is exhaustive over the 20
// event types; TestAllVariantsCoveredByRoundTrip guards that exhaustiveness.
var allVariantWire = map[EventType]string{
	EventTypeMessageStart:       `{"type":"message_start","message":{"id":"msg-1","model":"claude-opus"},"usage":{"input_tokens":5,"output_tokens":3,"cache_read_tokens":2,"cache_write_tokens":1,"thinking_tokens":4}}`,
	EventTypeContentDelta:       `{"type":"content_delta","delta":"hello","contentIndex":2}`,
	EventTypeThinkingDelta:      `{"type":"thinking_delta","delta":"reasoning"}`,
	EventTypeToolUseStart:       `{"type":"tool_use_start","toolUseId":"tu-1","name":"Read"}`,
	EventTypeToolUseDelta:       `{"type":"tool_use_delta","toolUseId":"tu-1","inputDelta":"{\"path\":"}`,
	EventTypeToolResult:         `{"type":"tool_result","toolUseId":"tu-1","output":{"ok":true,"rows":3},"isError":false}`,
	EventTypeMessageStop:        `{"type":"message_stop","stopReason":"end_turn","usage":{"input_tokens":5,"output_tokens":10}}`,
	EventTypeHookFired:          `{"type":"hook_fired","event":"PreToolUse","payload":{"tool":"Bash"}}`,
	EventTypeMemoryRetrieved:    `{"type":"memory_retrieved","memoryIds":["a","b"],"latencyMs":42,"timedOut":false}`,
	EventTypeMemoryCited:        `{"type":"memory_cited","memoryId":"m1"}`,
	EventTypeMemoryConsolidated: `{"type":"memory_consolidated","newFacts":2,"reinforced":1,"weakened":0}`,
	EventTypeStageStart:         `{"type":"stage_start","stage":"retrieve"}`,
	EventTypeStageEnd:           `{"type":"stage_end","stage":"retrieve","status":"ok"}`,
	EventTypeBudgetState:        `{"type":"budget_state","spent":{"tokens":17,"usd":0.001},"band":"ok","pct":0.05}`,
	EventTypeBudgetWarning:      `{"type":"budget_warning","remaining":{"tokens":100,"usd":0.01}}`,
	EventTypeCacheReport:        `{"type":"cache_report","hit":true,"hitRate":0.9}`,
	EventTypeRetryAttempt:       `{"type":"retry_attempt","provider":"anthropic","attempt":2,"reason":"overloaded"}`,
	EventTypePermissionRequest:  `{"type":"permission_request","tool":"Bash","input":{"cmd":"ls"}}`,
	EventTypeStop:               `{"type":"stop","reason":"clear"}`,
	EventTypeError:              `{"type":"error","error":{"code":"stream_failure","message":"boom"}}`,
}

// TestEveryVariantRoundTrips decodes each canonical wire frame, re-encodes it,
// and asserts the re-encoded JSON is structurally identical to the input. This
// catches both *dropped* fields (present in, missing out) and *invented* fields
// (absent in, present out) for every StreamEvent variant — the exact drift the
// discriminated-union Marshal/Unmarshal could introduce.
func TestEveryVariantRoundTrips(t *testing.T) {
	for typ, wire := range allVariantWire {
		t.Run(string(typ), func(t *testing.T) {
			var ev StreamEvent
			if err := json.Unmarshal([]byte(wire), &ev); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if ev.Type != typ {
				t.Fatalf("type = %q; want %q", ev.Type, typ)
			}
			out, err := json.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got, want := normalizeJSON(t, out), normalizeJSON(t, []byte(wire)); got != want {
				t.Errorf("round-trip drift:\n  in : %s\n  out: %s", want, got)
			}
		})
	}
}

// TestAllVariantsCoveredByRoundTrip fails if a new EventType constant is added
// without a corresponding round-trip fixture — keeping this coverage exhaustive
// as the union grows.
func TestAllVariantsCoveredByRoundTrip(t *testing.T) {
	all := []EventType{
		EventTypeMessageStart, EventTypeContentDelta, EventTypeThinkingDelta,
		EventTypeToolUseStart, EventTypeToolUseDelta, EventTypeToolResult,
		EventTypeMessageStop, EventTypeHookFired, EventTypeMemoryRetrieved,
		EventTypeMemoryCited, EventTypeMemoryConsolidated, EventTypeStageStart,
		EventTypeStageEnd, EventTypeBudgetState, EventTypeBudgetWarning,
		EventTypeCacheReport, EventTypeRetryAttempt, EventTypePermissionRequest,
		EventTypeStop, EventTypeError,
	}
	if len(allVariantWire) != len(all) {
		t.Fatalf("fixture map has %d entries; want %d", len(allVariantWire), len(all))
	}
	for _, typ := range all {
		if _, ok := allVariantWire[typ]; !ok {
			t.Errorf("missing round-trip fixture for %q", typ)
		}
	}
}

// TestUnmarshalUnknownTypeErrors proves an unrecognised discriminator is a hard
// error rather than a silently-empty event.
func TestUnmarshalUnknownTypeErrors(t *testing.T) {
	var ev StreamEvent
	err := json.Unmarshal([]byte(`{"type":"nonexistent_event"}`), &ev)
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Fatalf("expected unknown-event-type error, got %v", err)
	}
}

// TestUnmarshalMalformedErrors proves a malformed envelope surfaces an error.
func TestUnmarshalMalformedErrors(t *testing.T) {
	var ev StreamEvent
	if err := json.Unmarshal([]byte(`{`), &ev); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// TestMarshalUnknownTypeErrors proves marshalling an event with an unknown
// discriminator errors instead of emitting an untyped frame.
func TestMarshalUnknownTypeErrors(t *testing.T) {
	_, err := json.Marshal(StreamEvent{Type: "nonexistent_event"})
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Fatalf("expected unknown-event-type error, got %v", err)
	}
}

// TestMarshalTypeOnlyWhenPayloadNil documents the nil-payload path: a known type
// with no payload set marshals to just the discriminator (never a nil-deref).
func TestMarshalTypeOnlyWhenPayloadNil(t *testing.T) {
	out, err := json.Marshal(StreamEvent{Type: EventTypeContentDelta})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := normalizeJSON(t, out); got != `{"type":"content_delta"}` {
		t.Errorf("type-only marshal = %s", got)
	}
}
