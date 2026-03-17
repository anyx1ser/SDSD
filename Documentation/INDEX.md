# SDSD Documentation Index

This documentation reflects the current implementation (fanotify + per-UID ML + SQLite persistence).

## Read in This Order

1. `QUICKSTART.md` - get running quickly
2. `README_AGENT.md` - complete runtime behavior and tuning
3. `DEPLOYMENT.md` - production/service setup
4. `IMPLEMENTATION_SUMMARY.md` - architecture and design choices
5. `AUDIT_LOG_FORMAT.md` - event/field mapping reference (current + legacy parser note)
6. `DELIVERABLES.md` - repository deliverables snapshot
7. `COMPLETION_SUMMARY.md` - high-level project status

## Runtime Pipeline

```text
fanotify monitor -> per-UID sliding window aggregator -> hybrid detector -> alert output + SQLite
```

## Important Current Facts

- The agent does not tail `/var/log/audit/audit.log` in the active pipeline.
- Paths are configured with `-paths` and are now strictly filtered before events enter detection.
- Baselines are learned per UID and persisted in SQLite.
- New UIDs are protected by cold-start risk alerts before baseline completion.

