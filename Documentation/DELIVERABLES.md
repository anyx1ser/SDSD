# Deliverables

## Code

Core runtime files:

- `main.go`
- `reader.go`
- `aggregator.go`
- `detector.go`
- `database.go`
- `isolation_forest.go`
- `types.go`
- `entropy.go`

Supporting/legacy/reference:

- `parser.go` (legacy parser reference)
- tests (`*_test.go`)
- scripts (`demo.sh`, `verify_build.sh`)

## Runtime Capabilities

- fanotify monitoring with root privileges
- strict monitored-path prefix filtering
- per-UID sliding-window telemetry
- hybrid anomaly scoring (z-score + Isolation Forest)
- cold-start high-risk detection before baseline completion
- alert persistence and feature history in SQLite

## Documentation Set

- `INDEX.md`
- `QUICKSTART.md`
- `README_AGENT.md`
- `README_PROJECT.md`
- `DEPLOYMENT.md`
- `IMPLEMENTATION_SUMMARY.md`
- `AUDIT_LOG_FORMAT.md`
- `COMPLETION_SUMMARY.md`

## Validation

- unit tests via `go test ./...`
- feature and detector tests include transfer-process and cold-start logic

