# Implementation milestones

1. **Foundation** — React control plane, Go API, PostgreSQL, Docker Compose, development login.
2. **Scenario catalog** — YAML schema/parser, database migration/seed, dashboard APIs.
3. **Runtime manager** — Docker interface, labelled isolated networks, limits, readiness checks, teardown.
4. **Terminal** — xterm.js, Go WebSocket proxy, PTY attach, authorization and event collection.
5. **First real scenario** — Broken Nginx Upstream, fault injection, independent health validation.
6. **Completion loop** — hints, submissions, deterministic scoring, timeline, results.
7. **Operational hardening** — reset, expiry/reaper, structured logs, metrics, audit events.
8. **Scenario library** — disk, pool, permissions, and memory incidents.
9. **Quality** — unit/integration/E2E tests, CI, authoring guide, ADRs, local developer workflow.

The current repository implements Milestone 1’s frontend control-plane foundation. It intentionally does not claim to provide the runtime plane until the Go and Docker services exist.
