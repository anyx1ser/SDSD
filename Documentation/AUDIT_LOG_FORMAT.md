# Event and Field Reference

This project originally included audit-log parsing support. The active runtime path is now fanotify-based.

## Active Runtime Event Shape

The monitor emits `AuditEvent` structs with:

- timestamp
- UID
- process name
- file path
- event type (`open`, `read`, `close_write`, `execve`, ...)
- success

These events are generated from fanotify metadata and `/proc` lookups.

## Path Scoping

Configured `-paths` are normalized and strict path-prefix checks are applied before events are forwarded.

Implication:

- Activity outside configured targets is ignored by detection.

## Feature Mapping

Events are converted into windowed `FeatureVector` fields such as:

- access/read/write counts
- unique file and directory counts
- process entropy
- burst metrics
- archive and transfer process signals
- entropy-based filename/path diversity metrics

## Legacy Audit Parser Note

`parser.go` still contains parsing logic for historical audit lines (`SYSCALL`, `EXECVE`) but is not part of current main runtime execution.

If audit-log ingestion is re-enabled in future, this file can serve as the baseline parser implementation.

