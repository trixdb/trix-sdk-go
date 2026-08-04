package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBudgetWarningUnspecifiedStaysUnspecified is a regression test for a
// field-fidelity bug: the TS runtime emits budget_warning.remaining as
// `{ tokens?: number; usd?: number }`, so an unspecified field is *absent* on
// the wire (`{"remaining":{}}`). The old Go type used BudgetSpent (non-optional
// int/float), which both (a) re-marshalled the absent fields as `tokens:0,usd:0`
// and (b) made a genuine "0 remaining" (budget exhausted) indistinguishable from
// "not reported". Pointers restore the distinction.
func TestBudgetWarningUnspecifiedStaysUnspecified(t *testing.T) {
	const wire = `{"type":"budget_warning","remaining":{}}`
	var ev StreamEvent
	if err := json.Unmarshal([]byte(wire), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Type != EventTypeBudgetWarning || ev.BudgetWarning == nil {
		t.Fatalf("did not decode as budget_warning: %+v", ev)
	}
	if ev.BudgetWarning.Remaining.Tokens != nil {
		t.Errorf("Tokens = %v; want nil (unspecified)", *ev.BudgetWarning.Remaining.Tokens)
	}
	if ev.BudgetWarning.Remaining.USD != nil {
		t.Errorf("USD = %v; want nil (unspecified)", *ev.BudgetWarning.Remaining.USD)
	}

	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Absent fields must not be re-invented as zeros.
	if strings.Contains(string(out), `"tokens"`) || strings.Contains(string(out), `"usd"`) {
		t.Errorf("re-marshal invented zero fields: %s", out)
	}
}

// TestBudgetWarningZeroIsDistinctFromUnspecified proves a real zero survives
// round-trip and is not silently dropped by omitempty on the pointer.
func TestBudgetWarningZeroIsDistinctFromUnspecified(t *testing.T) {
	const wire = `{"type":"budget_warning","remaining":{"tokens":0,"usd":0}}`
	var ev StreamEvent
	if err := json.Unmarshal([]byte(wire), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	r := ev.BudgetWarning.Remaining
	if r.Tokens == nil || *r.Tokens != 0 {
		t.Errorf("Tokens = %v; want a non-nil 0 (exhausted budget must not read as unspecified)", r.Tokens)
	}
	if r.USD == nil || *r.USD != 0 {
		t.Errorf("USD = %v; want a non-nil 0", r.USD)
	}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"tokens":0`) || !strings.Contains(string(out), `"usd":0`) {
		t.Errorf("explicit zero dropped on re-marshal: %s", out)
	}
}

// TestBudgetWarningPopulatedRoundTrips covers the ordinary populated case.
func TestBudgetWarningPopulatedRoundTrips(t *testing.T) {
	const wire = `{"type":"budget_warning","remaining":{"tokens":1200,"usd":0.42}}`
	var ev StreamEvent
	if err := json.Unmarshal([]byte(wire), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	r := ev.BudgetWarning.Remaining
	if r.Tokens == nil || *r.Tokens != 1200 {
		t.Errorf("Tokens = %v; want 1200", r.Tokens)
	}
	if r.USD == nil || *r.USD != 0.42 {
		t.Errorf("USD = %v; want 0.42", r.USD)
	}
}
