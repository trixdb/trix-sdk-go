package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Cross-language conformance: a fixture produced by the TypeScript SDK
// (trix-bots/src/sdk/fixtures.ts) must parse and round-trip identically
// here. This is the contract that lets retrix swap runtimes.
func TestCrossLanguageFixtureParses(t *testing.T) {
	path := filepath.Join("..", "testdata", "cross-lang-fixture.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	lines, err := ParseFixtureString(string(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("got %d lines; want 10", len(lines))
	}

	// Spot-check every variant we care about.
	want := []EventType{
		EventTypeMessageStart,
		EventTypeMemoryRetrieved,
		EventTypeContentDelta,
		EventTypeMemoryCited,
		EventTypeContentDelta,
		EventTypeToolUseStart,
		EventTypeMessageStop,
		EventTypeBudgetState,
		EventTypeCacheReport,
		EventTypeStop,
	}
	for i, w := range want {
		if lines[i].Event.Type != w {
			t.Errorf("line %d: type = %s; want %s", i, lines[i].Event.Type, w)
		}
	}

	// Spot-check payload values
	if lines[0].Event.MessageStart.Message.Model != "claude-opus-4-6" {
		t.Errorf("messageStart model wrong: %s", lines[0].Event.MessageStart.Message.Model)
	}
	if lines[1].Event.MemoryRetrieved.LatencyMs != 42 {
		t.Errorf("memoryRetrieved latency wrong: %d", lines[1].Event.MemoryRetrieved.LatencyMs)
	}
	if lines[3].Event.MemoryCited.MemoryID != "m1" {
		t.Errorf("memoryCited id wrong: %s", lines[3].Event.MemoryCited.MemoryID)
	}
	if lines[7].Event.BudgetState.Band != "ok" {
		t.Errorf("budgetState band wrong: %s", lines[7].Event.BudgetState.Band)
	}
	if !lines[8].Event.CacheReport.Hit {
		t.Errorf("cacheReport hit should be true")
	}
	if lines[9].Event.Stop.Reason != "clear" {
		t.Errorf("stop reason wrong: %s", lines[9].Event.Stop.Reason)
	}
}

// Round-trip: parse → marshal back → re-parse → compare.
// Verifies our MarshalJSON produces the same wire shape as the TS SDK.
func TestCrossLanguageFixtureRoundTrips(t *testing.T) {
	path := filepath.Join("..", "testdata", "cross-lang-fixture.ndjson")
	raw, _ := os.ReadFile(path)
	lines, _ := ParseFixtureString(string(raw))
	for i, line := range lines {
		out, err := json.Marshal(line.Event)
		if err != nil {
			t.Fatalf("marshal line %d: %v", i, err)
		}
		var parsed StreamEvent
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("re-parse line %d: %v\nbody=%s", i, err, out)
		}
		if parsed.Type != line.Event.Type {
			t.Errorf("line %d type drift: %s -> %s", i, line.Event.Type, parsed.Type)
		}
	}
}
