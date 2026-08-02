# trix-sdk-go

Go SDK for embedding the Trix agent runtime. Mirrors the TypeScript SDK at
`trix-bots/src/sdk/` so consumers can target either runtime with the same
event surface.

## Quick start

```go
import "github.com/trixdb/trix-sdk-go/agent"

c := agent.NewClient(agent.ClientOptions{
    BaseURL: "http://localhost:3739",
})
// Query returns three values: the event channel, an error channel, and a
// synchronous error (bad config, or the request failed to open).
events, errs, err := c.Query(ctx, agent.QueryOptions{
    SessionID: "sess-1",
    SpaceID:   "space-A",
    UserText:  "hello",
})
if err != nil { return err }
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
// Once events is drained, surface any mid-stream error (non-blocking:
// errs stays open on success).
select {
case err := <-errs:
    if err != nil { return err }
default:
}
```

See [ADR-121](../trix-research/docs/decisions/planned/ADR-121-agent-sdk-surface-and-transport.md)
for the contract.
