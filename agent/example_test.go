package agent_test

import (
	"context"
	"fmt"

	"github.com/trixdb/trix-sdk-go/agent"
)

// Example demonstrates the canonical streaming loop: open a Query, switch on
// each event's Type to read the matching payload, then drain the error channel
// once the event channel closes.
func Example() {
	c := agent.NewClient(agent.ClientOptions{
		BaseURL: "http://localhost:3739",
	})

	events, errs, err := c.Query(context.Background(), agent.QueryOptions{
		SessionID: "sess-1",
		SpaceID:   "space-A",
		UserText:  "hello",
	})
	if err != nil {
		fmt.Println("open:", err)
		return
	}

	for ev := range events {
		switch ev.Type {
		case agent.EventTypeContentDelta:
			fmt.Print(ev.ContentDelta.Delta)
		case agent.EventTypeMemoryCited:
			fmt.Printf("[cited %s]\n", ev.MemoryCited.MemoryID)
		case agent.EventTypeStop:
			// stream is ending; the events channel closes next
		}
	}

	// Surface a mid-stream error, if any. errs stays open on a clean finish,
	// so this select is non-blocking.
	select {
	case err := <-errs:
		if err != nil {
			fmt.Println("stream:", err)
		}
	default:
	}
}
