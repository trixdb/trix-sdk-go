# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

No version has been tagged yet; everything below is unreleased and slated for
the first `v0.1.0` pre-release.

## [Unreleased]

### Added

- `agent.Client` streaming client for `POST /v1/agent/run` — `Query` returns a
  typed `StreamEvent` channel, an error channel, and a synchronous open error.
- Full `StreamEvent` discriminated-union type surface mirroring the TypeScript
  SDK (`trix-bots/src/sdk/types.ts`, ADR-121): model stream, memory, pipeline,
  budget, cache, retry, hook, permission, and lifecycle events.
- SSE frame parser (`ReadSSEFrames`) and typed decoder (`DecodeFrameToEvent`),
  with heartbeat/comment-line handling and multi-line `data:` joining.
- SSE resume: each `StreamEvent` surfaces its frame `id:` on `ev.ID`; pass the
  last id back as `QueryOptions.LastEventID` (sent as the `Last-Event-ID`
  header) to resume after a dropped stream.
- Cross-language fixture conformance tests: a TS-generated NDJSON fixture parses
  and round-trips identically in Go.
- Transport-posture validation in `NewClient`: HTTPS enforced for remote hosts
  (override with `AllowInsecure`), API key required for remote hosts, loopback
  hosts exempt for local development.
- `examples/retrix-style`: a `RunOutcome`-style embedding facade plus an
  httptest integration test covering the full memory-native transcript.

### Security / Robustness

- Bounded HTTP connection-setup and response-header timeouts without capping the
  long-lived SSE body.
- Bounded SSE frame size (8 MiB) and error-body reads (64 KiB) to cap the memory
  a hostile or buggy server can force the client to buffer.
- Context-cancellation closes the connection and reclaims both worker goroutines
  on every exit path (no goroutine or connection leaks).

[Unreleased]: https://github.com/trixdb/trix-sdk-go/commits/main
