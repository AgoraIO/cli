# CLI telemetry implementation plan

Goal: Implement the approved final design locally across CLI and SSP.
Architecture: Snapshot collection policy per operation; submit allowlisted NDJSON to a detached sender with bounded queues. Observe creation at createProject, then forward strict events through the existing collector.
Tech stack: Go/Cobra, Bun/Hono/Zod.

- [x] Remove consent and implement a private, independent best-effort notice marker; test JSON, quiet, opt-outs and live MCP configuration.
- [x] Add creation observations and validated app_id at the shared createProject boundary; pass operations through shell, MCP, Recipe and Quickstart resolution.
- [x] Replace in-process HTTP with one lazy detached sender per operation, 20-event queues, 100 ms local drain, three-second HTTP/EOF budgets and two child slots per parent; test slow receivers, overflow, recovery and real subprocess exit.
- [x] Extend SSP schema and command/event matching; remove PostHog uuid/event_id mapping; preserve existing limits and deduplication.
- [x] Update help, generated command docs, protocol and operational docs; run Go tests/race/lint, Bun tests/typecheck/lint and local CLI→SSP→mock PostHog verification.
- [x] Record platform/runtime and real PostHog verification limits. Production activation and deployment are outside this implementation.
