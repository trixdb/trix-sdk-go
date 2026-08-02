// Package agent is the Trix agent SDK for Go.
//
// The event surface mirrors trix-bots/src/sdk/types.ts (ADR-121) so a
// recorded fixture from either runtime can be replayed in the other.
//
// Go lacks tagged unions, so StreamEvent is a discriminated struct: one
// Type field plus pointer-typed payload fields, exactly one of which is
// populated per event. JSON marshalling preserves the wire format the TS
// SDK emits.
package agent

import (
	"encoding/json"
	"fmt"
)

// EventType is the discriminator on a StreamEvent.
type EventType string

// StreamEvent variant constants — kept in sync with TypeScript SDK.
const (
	EventTypeMessageStart       EventType = "message_start"
	EventTypeContentDelta       EventType = "content_delta"
	EventTypeThinkingDelta      EventType = "thinking_delta"
	EventTypeToolUseStart       EventType = "tool_use_start"
	EventTypeToolUseDelta       EventType = "tool_use_delta"
	EventTypeToolResult         EventType = "tool_result"
	EventTypeMessageStop        EventType = "message_stop"
	EventTypeHookFired          EventType = "hook_fired"
	EventTypeMemoryRetrieved    EventType = "memory_retrieved"
	EventTypeMemoryCited        EventType = "memory_cited"
	EventTypeMemoryConsolidated EventType = "memory_consolidated"
	EventTypeStageStart         EventType = "stage_start"
	EventTypeStageEnd           EventType = "stage_end"
	EventTypeBudgetState        EventType = "budget_state"
	EventTypeBudgetWarning      EventType = "budget_warning"
	EventTypeCacheReport        EventType = "cache_report"
	EventTypeRetryAttempt       EventType = "retry_attempt"
	EventTypePermissionRequest  EventType = "permission_request"
	EventTypeStop               EventType = "stop"
	EventTypeError              EventType = "error"
)

// Usage matches SdkUsage in TS.
type Usage struct {
	InputTokens      int  `json:"input_tokens"`
	OutputTokens     int  `json:"output_tokens"`
	CacheReadTokens  *int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int `json:"cache_write_tokens,omitempty"`
	ThinkingTokens   *int `json:"thinking_tokens,omitempty"`
}

// AssistantMessageRef is the message identity the SDK reports.
type AssistantMessageRef struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

// StreamEvent is the discriminated union. Marshals to/from the same JSON
// shape as the TypeScript StreamEvent.
type StreamEvent struct {
	Type EventType `json:"type"`

	// ID is the SSE event id from the frame's `id:` line — the resume cursor,
	// not part of the JSON payload (the custom Marshal/Unmarshal never touch it).
	// Track the last non-zero ID you receive and pass it back as
	// QueryOptions.LastEventID to resume after a dropped stream. Zero means the
	// frame carried no id.
	ID int64 `json:"-"`

	// Mutually-exclusive payloads. Exactly one is non-nil per event.
	MessageStart       *MessageStartPayload       `json:"-"`
	ContentDelta       *ContentDeltaPayload       `json:"-"`
	ThinkingDelta      *ThinkingDeltaPayload      `json:"-"`
	ToolUseStart       *ToolUseStartPayload       `json:"-"`
	ToolUseDelta       *ToolUseDeltaPayload       `json:"-"`
	ToolResult         *ToolResultPayload         `json:"-"`
	MessageStop        *MessageStopPayload        `json:"-"`
	HookFired          *HookFiredPayload          `json:"-"`
	MemoryRetrieved    *MemoryRetrievedPayload    `json:"-"`
	MemoryCited        *MemoryCitedPayload        `json:"-"`
	MemoryConsolidated *MemoryConsolidatedPayload `json:"-"`
	StageStart         *StageStartPayload         `json:"-"`
	StageEnd           *StageEndPayload           `json:"-"`
	BudgetState        *BudgetStatePayload        `json:"-"`
	BudgetWarning      *BudgetWarningPayload      `json:"-"`
	CacheReport        *CacheReportPayload        `json:"-"`
	RetryAttempt       *RetryAttemptPayload       `json:"-"`
	PermissionRequest  *PermissionRequestPayload  `json:"-"`
	Stop               *StopPayload               `json:"-"`
	Error              *ErrorPayload              `json:"-"`
}

// MessageStartPayload mirrors the TS message_start event.
type MessageStartPayload struct {
	Message AssistantMessageRef `json:"message"`
	Usage   Usage               `json:"usage"`
}

// ContentDeltaPayload mirrors the TS content_delta event.
type ContentDeltaPayload struct {
	Delta        string `json:"delta"`
	ContentIndex int    `json:"contentIndex"`
}

// ThinkingDeltaPayload mirrors the TS thinking_delta event.
type ThinkingDeltaPayload struct {
	Delta string `json:"delta"`
}

// ToolUseStartPayload mirrors the TS tool_use_start event.
type ToolUseStartPayload struct {
	ToolUseID string `json:"toolUseId"`
	Name      string `json:"name"`
}

// ToolUseDeltaPayload mirrors the TS tool_use_delta event.
type ToolUseDeltaPayload struct {
	ToolUseID  string `json:"toolUseId"`
	InputDelta string `json:"inputDelta"`
}

// ToolResultPayload mirrors the TS tool_result event.
type ToolResultPayload struct {
	ToolUseID string          `json:"toolUseId"`
	Output    json.RawMessage `json:"output"`
	IsError   *bool           `json:"isError,omitempty"`
}

// MessageStopPayload mirrors the TS message_stop event.
type MessageStopPayload struct {
	StopReason string `json:"stopReason"`
	Usage      Usage  `json:"usage"`
}

// HookFiredPayload mirrors the TS hook_fired event.
type HookFiredPayload struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// MemoryRetrievedPayload mirrors the TS memory_retrieved event.
type MemoryRetrievedPayload struct {
	MemoryIDs []string `json:"memoryIds"`
	LatencyMs int      `json:"latencyMs"`
	TimedOut  bool     `json:"timedOut"`
}

// MemoryCitedPayload mirrors the TS memory_cited event.
type MemoryCitedPayload struct {
	MemoryID string `json:"memoryId"`
}

// MemoryConsolidatedPayload mirrors the TS memory_consolidated event.
type MemoryConsolidatedPayload struct {
	NewFacts   int `json:"newFacts"`
	Reinforced int `json:"reinforced"`
	Weakened   int `json:"weakened"`
}

// StageStartPayload mirrors the TS stage_start event.
type StageStartPayload struct {
	Stage string `json:"stage"`
}

// StageEndPayload mirrors the TS stage_end event.
type StageEndPayload struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
}

// BudgetSpent mirrors the inner spent object.
type BudgetSpent struct {
	Tokens int     `json:"tokens"`
	USD    float64 `json:"usd"`
}

// BudgetStatePayload mirrors the TS budget_state event.
type BudgetStatePayload struct {
	Spent BudgetSpent `json:"spent"`
	Band  string      `json:"band"`
	Pct   float64     `json:"pct"`
}

// BudgetWarningPayload mirrors the TS budget_warning event.
type BudgetWarningPayload struct {
	Remaining BudgetSpent `json:"remaining"`
}

// CacheReportPayload mirrors the TS cache_report event.
type CacheReportPayload struct {
	Hit     bool    `json:"hit"`
	HitRate float64 `json:"hitRate"`
}

// RetryAttemptPayload mirrors the TS retry_attempt event.
type RetryAttemptPayload struct {
	Provider string `json:"provider"`
	Attempt  int    `json:"attempt"`
	Reason   string `json:"reason"`
}

// PermissionRequestPayload mirrors the TS permission_request event.
type PermissionRequestPayload struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// StopPayload mirrors the TS stop event.
type StopPayload struct {
	Reason string `json:"reason"`
}

// ErrorPayload mirrors the TS error event.
type ErrorPayload struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// MarshalJSON renders an event in the wire format consumed by the TS SDK.
func (s StreamEvent) MarshalJSON() ([]byte, error) {
	payload, err := s.payload()
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return json.Marshal(map[string]any{"type": s.Type})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// Inject the type discriminator into the payload object.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	t, _ := json.Marshal(s.Type)
	m["type"] = t
	return json.Marshal(m)
}

// UnmarshalJSON parses an event from the TS wire format.
func (s *StreamEvent) UnmarshalJSON(data []byte) error {
	var head struct {
		Type EventType `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	s.Type = head.Type
	switch head.Type {
	case EventTypeMessageStart:
		s.MessageStart = new(MessageStartPayload)
		return json.Unmarshal(data, s.MessageStart)
	case EventTypeContentDelta:
		s.ContentDelta = new(ContentDeltaPayload)
		return json.Unmarshal(data, s.ContentDelta)
	case EventTypeThinkingDelta:
		s.ThinkingDelta = new(ThinkingDeltaPayload)
		return json.Unmarshal(data, s.ThinkingDelta)
	case EventTypeToolUseStart:
		s.ToolUseStart = new(ToolUseStartPayload)
		return json.Unmarshal(data, s.ToolUseStart)
	case EventTypeToolUseDelta:
		s.ToolUseDelta = new(ToolUseDeltaPayload)
		return json.Unmarshal(data, s.ToolUseDelta)
	case EventTypeToolResult:
		s.ToolResult = new(ToolResultPayload)
		return json.Unmarshal(data, s.ToolResult)
	case EventTypeMessageStop:
		s.MessageStop = new(MessageStopPayload)
		return json.Unmarshal(data, s.MessageStop)
	case EventTypeHookFired:
		s.HookFired = new(HookFiredPayload)
		return json.Unmarshal(data, s.HookFired)
	case EventTypeMemoryRetrieved:
		s.MemoryRetrieved = new(MemoryRetrievedPayload)
		return json.Unmarshal(data, s.MemoryRetrieved)
	case EventTypeMemoryCited:
		s.MemoryCited = new(MemoryCitedPayload)
		return json.Unmarshal(data, s.MemoryCited)
	case EventTypeMemoryConsolidated:
		s.MemoryConsolidated = new(MemoryConsolidatedPayload)
		return json.Unmarshal(data, s.MemoryConsolidated)
	case EventTypeStageStart:
		s.StageStart = new(StageStartPayload)
		return json.Unmarshal(data, s.StageStart)
	case EventTypeStageEnd:
		s.StageEnd = new(StageEndPayload)
		return json.Unmarshal(data, s.StageEnd)
	case EventTypeBudgetState:
		s.BudgetState = new(BudgetStatePayload)
		return json.Unmarshal(data, s.BudgetState)
	case EventTypeBudgetWarning:
		s.BudgetWarning = new(BudgetWarningPayload)
		return json.Unmarshal(data, s.BudgetWarning)
	case EventTypeCacheReport:
		s.CacheReport = new(CacheReportPayload)
		return json.Unmarshal(data, s.CacheReport)
	case EventTypeRetryAttempt:
		s.RetryAttempt = new(RetryAttemptPayload)
		return json.Unmarshal(data, s.RetryAttempt)
	case EventTypePermissionRequest:
		s.PermissionRequest = new(PermissionRequestPayload)
		return json.Unmarshal(data, s.PermissionRequest)
	case EventTypeStop:
		s.Stop = new(StopPayload)
		return json.Unmarshal(data, s.Stop)
	case EventTypeError:
		s.Error = new(ErrorPayload)
		return json.Unmarshal(data, s.Error)
	default:
		return fmt.Errorf("unknown event type %q", head.Type)
	}
}

// payload returns the populated payload for the event's discriminator.
// Returns (nil, nil) when no payload is set (legitimate for events whose
// shape is just the type discriminator — there are none today).
func (s StreamEvent) payload() (any, error) {
	switch s.Type {
	case EventTypeMessageStart:
		return s.MessageStart, nil
	case EventTypeContentDelta:
		return s.ContentDelta, nil
	case EventTypeThinkingDelta:
		return s.ThinkingDelta, nil
	case EventTypeToolUseStart:
		return s.ToolUseStart, nil
	case EventTypeToolUseDelta:
		return s.ToolUseDelta, nil
	case EventTypeToolResult:
		return s.ToolResult, nil
	case EventTypeMessageStop:
		return s.MessageStop, nil
	case EventTypeHookFired:
		return s.HookFired, nil
	case EventTypeMemoryRetrieved:
		return s.MemoryRetrieved, nil
	case EventTypeMemoryCited:
		return s.MemoryCited, nil
	case EventTypeMemoryConsolidated:
		return s.MemoryConsolidated, nil
	case EventTypeStageStart:
		return s.StageStart, nil
	case EventTypeStageEnd:
		return s.StageEnd, nil
	case EventTypeBudgetState:
		return s.BudgetState, nil
	case EventTypeBudgetWarning:
		return s.BudgetWarning, nil
	case EventTypeCacheReport:
		return s.CacheReport, nil
	case EventTypeRetryAttempt:
		return s.RetryAttempt, nil
	case EventTypePermissionRequest:
		return s.PermissionRequest, nil
	case EventTypeStop:
		return s.Stop, nil
	case EventTypeError:
		return s.Error, nil
	default:
		return nil, fmt.Errorf("unknown event type %q", s.Type)
	}
}
