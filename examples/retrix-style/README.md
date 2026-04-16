# Retrix-style embedding example

The canonical pattern any downstream Go consumer (retrix, custom agents,
ops tooling) uses to embed the Trix Agent SDK.

## What this shows

- One `agent.Client` constructed at startup.
- A `Run` facade that flattens the streaming event surface into a
  `RunOutcome` struct (text, citations, budget, stop reason) — the shape
  most app code prefers.
- Real-server invocation via `TRIX_API_URL` + `TRIX_API_KEY`.
- Mocked SSE integration test in `embed_test.go` proves the embedding
  works end-to-end against the same wire format the live API uses.

## Run

```sh
TRIX_API_URL=https://api.trixdb.com \
TRIX_API_KEY=sk-trix-... \
TRIX_SPACE_ID=space-A \
go run ./examples/retrix-style "what is the answer?"
```

## Test

```sh
go test ./examples/retrix-style
```

## Why it's the test for retrix integration

Retrix doesn't live in this repo, but its embedding pattern is identical
to this example: import `github.com/devghost/trix-sdk-go/agent`, build a
`Client`, call `Query()`, fold the StreamEvents into your own outcome
type. The integration test covers every interaction surface a retrix-style
consumer would touch: auth header forwarding, error events, HTTP failures,
context cancellation, empty responses, full memory-native transcripts.

When/if retrix lands in the repo, it can either depend on this example
package directly or copy `EmbeddedAgent` verbatim — the contract is
stable.
