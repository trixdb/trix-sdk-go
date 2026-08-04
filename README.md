# trix-sdk-go

[![Go Reference](https://pkg.go.dev/badge/github.com/trixdb/trix-sdk-go.svg)](https://pkg.go.dev/github.com/trixdb/trix-sdk-go)

Go SDK for embedding the **Trix agent runtime**. It opens a streaming
agent-run session over Server-Sent Events and hands you typed
`StreamEvent`s. The event surface mirrors the TypeScript SDK at
`trix-bots/src/sdk/` (ADR-121), so a fixture recorded by either runtime
replays in the other.

## Scope

This is deliberately the **agent-runtime streaming client**, not a
general-purpose Trix API client. It covers the agent-run surface
(`POST /v1/agent/run`) and its typed event stream — nothing else. If you
need CRUD access to the rest of the Trix API (memory, chat, goals,
billing, …), use the Python, TypeScript, or C# SDKs. The WebSocket
transport (`GET /v1/agent/ws`) and the prompt/schema helpers the TS SDK
ships are **not yet implemented here** — see the tracking issue for that
decision.

## Install

```sh
go get github.com/trixdb/trix-sdk-go
```

Requires Go 1.22 or newer. The SDK has **no third-party dependencies** —
standard library only.

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
if err != nil {
    return err
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
// Once events is drained, surface any mid-stream error (non-blocking:
// errs stays open on success).
select {
case err := <-errs:
    if err != nil {
        return err
    }
default:
}
```

A fuller, `RunOutcome`-style facade (fold the stream into a struct of
text + citations + budget) is in [`examples/retrix-style`](examples/retrix-style).

## Authentication

Pass an API key; it is sent as `Authorization: Bearer <key>`:

```go
c := agent.NewClient(agent.ClientOptions{
    BaseURL: "https://api.trixdb.com",
    APIKey:  os.Getenv("TRIX_API_KEY"),
})
```

Transport posture is validated up front (the error surfaces from the
first `Query`):

- A **remote** `BaseURL` must be `https://` — plain HTTP is refused unless
  you set `AllowInsecure: true`.
- A **remote** `BaseURL` requires a non-empty `APIKey` (so you can't
  accidentally hit the API unauthenticated).
- Loopback hosts (`localhost`, `127.0.0.1`, `::1`) are exempt from both,
  for local development.

## The event stream

`StreamEvent` is a discriminated union: a `Type` discriminator plus
pointer-typed payload fields, exactly one of which is non-nil. Switch on
`ev.Type` and read the matching payload. The variants (kept in sync with
the TS SDK) cover the model stream (`message_start`, `content_delta`,
`thinking_delta`, `tool_use_start` / `tool_use_delta` / `tool_result`,
`message_stop`), memory (`memory_retrieved`, `memory_cited`,
`memory_consolidated`), pipeline (`stage_start` / `stage_end`), budget
(`budget_state`, `budget_warning`), cache (`cache_report`), retries
(`retry_attempt`), hooks (`hook_fired`), permissions
(`permission_request`), and lifecycle (`stop`, `error`).

## Cancellation & timeouts

Cancel the `context` you pass to `Query` to abort an in-flight stream —
the SDK closes the connection and reclaims its worker goroutines. The
default HTTP client bounds connection setup and time-to-first-byte, but
deliberately does **not** cap the long-lived stream body (that would abort
a healthy run). Supply your own `HTTPClient` to change that.

## Resume after a dropped stream

Every `StreamEvent` carries the SSE `id:` from its frame on `ev.ID`. Track
the last non-zero id you saw and pass it back as `QueryOptions.LastEventID`
to resume:

```go
var lastID int64
for ev := range events {
    lastID = ev.ID
    // …handle ev…
}
// reconnect after a drop:
events, errs, err = c.Query(ctx, agent.QueryOptions{
    SessionID:   "sess-1",
    SpaceID:     "space-A",
    UserText:    "hello",
    LastEventID: lastID, // sent as the Last-Event-ID header
})
```

The server replays only events after `LastEventID` (it skips frames with
`seq <= LastEventID`).

> **Caveat — resume is caller-controlled by design.** `POST /v1/agent/run`
> re-invokes the agent from scratch on reconnect and suppresses
> already-delivered events by sequence number; it is **not** a stateful
> resume of the original run. For a non-deterministic live LLM run, a
> second generation can diverge from the first, so the SDK does **not**
> auto-reconnect — it hands you the cursor and lets you decide. Resume is
> cleanest for replayable/idempotent run factories (e.g. fixture replay);
> for live runs, treat it as best-effort continuation.

## Error handling

`Query` reports failures on three paths:

1. **Synchronous** (third return value): bad configuration, or the request
   failed to open — including a non-2xx HTTP status.
2. **Asynchronous** (`errs` channel): a mid-stream transport or decode
   error. At most one error is delivered; `errs` stays open on a clean
   finish, so drain it non-blockingly after `events` closes.
3. **In-band** `error` events: an `EventTypeError` frame carries a
   `{ code, message }` payload from the runtime itself.

## Development

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

## See also

- [`examples/retrix-style`](examples/retrix-style) — the canonical embedding pattern.
- [CHANGELOG.md](CHANGELOG.md)
- [ADR-121](../trix-research/docs/decisions/planned/ADR-121-agent-sdk-surface-and-transport.md) — the transport/event contract.
