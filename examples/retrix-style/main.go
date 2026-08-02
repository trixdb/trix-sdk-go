// Package main is a Retrix-style embedding example.
//
// Demonstrates the canonical pattern any downstream Go consumer (retrix,
// custom agents, ops tooling) uses to embed the Trix Agent SDK:
//
//  1. Construct an `agent.Client` once at startup.
//  2. Per request, call `Query()` to open a streaming session.
//  3. Pattern-match on event types, drive UI/storage from the deltas.
//  4. Track outcomes (citations, budget bands) for analytics.
//
// Run against a real trix-api by setting TRIX_API_URL + TRIX_API_KEY.
// The accompanying `embed_test.go` proves the same flow works against a
// mocked SSE server.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/trixdb/trix-sdk-go/agent"
)

// RunOutcome captures everything a downstream consumer typically needs from
// a single agent run: the assistant's text, citations for follow-ups, and
// numeric outcomes for telemetry.
type RunOutcome struct {
	AssistantText  string
	CitedMemoryIDs []string
	BudgetSpent    agent.BudgetSpent
	StopReason     string
}

// EmbeddedAgent wraps the SDK with a Retrix-shaped API: one `Run` call
// returns a fully-realised RunOutcome instead of a raw event stream.
// Streaming consumers can still use `agent.Client` directly; this is the
// shape most app code prefers.
type EmbeddedAgent struct {
	client *agent.Client
	out    io.Writer
}

// NewEmbeddedAgent constructs a Retrix-style facade.
func NewEmbeddedAgent(baseURL, apiKey string, out io.Writer) *EmbeddedAgent {
	return &EmbeddedAgent{
		client: agent.NewClient(agent.ClientOptions{
			BaseURL: baseURL,
			APIKey:  apiKey,
		}),
		out: out,
	}
}

// Run drives a single agent turn end-to-end, returning the outcome once the
// stream stops. ctx cancellation aborts the underlying SSE connection.
func (e *EmbeddedAgent) Run(ctx context.Context, opts agent.QueryOptions) (RunOutcome, error) {
	out := RunOutcome{}
	events, errs, err := e.client.Query(ctx, opts)
	if err != nil {
		return out, fmt.Errorf("query: %w", err)
	}
	var assistant strings.Builder
	for ev := range events {
		switch ev.Type {
		case agent.EventTypeContentDelta:
			assistant.WriteString(ev.ContentDelta.Delta)
		case agent.EventTypeMemoryCited:
			out.CitedMemoryIDs = append(out.CitedMemoryIDs, ev.MemoryCited.MemoryID)
		case agent.EventTypeBudgetState:
			out.BudgetSpent = ev.BudgetState.Spent
		case agent.EventTypeStop:
			out.StopReason = ev.Stop.Reason
		case agent.EventTypeError:
			return out, fmt.Errorf("agent error: %s — %s", ev.Error.Error.Code, ev.Error.Error.Message)
		}
	}
	// Drain any async errors surfaced after the channel closes.
	select {
	case e := <-errs:
		if e != nil {
			return out, e
		}
	default:
	}
	out.AssistantText = assistant.String()
	return out, nil
}

func main() {
	baseURL := os.Getenv("TRIX_API_URL")
	if baseURL == "" {
		baseURL = "https://api.trixdb.com"
	}
	apiKey := os.Getenv("TRIX_API_KEY")
	prompt := os.Args[len(os.Args)-1]
	if len(os.Args) < 2 || strings.HasPrefix(prompt, "-") {
		fmt.Fprintln(os.Stderr, "usage: retrix-style <prompt>")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	a := NewEmbeddedAgent(baseURL, apiKey, os.Stdout)
	outcome, err := a.Run(ctx, agent.QueryOptions{
		SpaceID:  os.Getenv("TRIX_SPACE_ID"),
		UserText: prompt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(outcome.AssistantText)
	if len(outcome.CitedMemoryIDs) > 0 {
		fmt.Fprintf(os.Stderr, "[cited %d memories: %s]\n", len(outcome.CitedMemoryIDs), strings.Join(outcome.CitedMemoryIDs, ", "))
	}
	fmt.Fprintf(os.Stderr, "[stop: %s; spent %d tokens / $%.4f]\n", outcome.StopReason, outcome.BudgetSpent.Tokens, outcome.BudgetSpent.USD)
}
