# SDSD

SDSD is a Linux filesystem anomaly detector written in Go.

It monitors file activity with fanotify, aggregates behavior in sliding windows, learns per-UID baselines, and raises alerts when behavior deviates or matches high-risk cold-start patterns.

## Current Detection Model

- Data source: Linux fanotify events (not audit log tailing)
- Scope: monitored path prefixes (default `/home,/etc`)
- Baseline: per UID
- Models: aggregate z-score + Isolation Forest
- Persistence: SQLite (`sdsd_baselines.db` by default)
- Cold-start protection: immediate alerts during baseline for risky behavior (including transfer tools such as `rsync`, `scp`, `sftp`, `rclone`)

## Quick Start

```bash
go build -o anomaly-detector
sudo ./anomaly-detector
```

Default startup settings:

- Paths: `/home,/etc`
- Window: `10s`
- Step: `2s`
- Baseline windows per UID: `60`

## Key Flags

```text
-paths string
    Comma-separated paths to monitor (default "/home,/etc")
-window duration
    Aggregation window size (default 10s)
-step duration
    Sliding-window step interval (default 2s)
-baseline int
    Windows to collect per UID before training (default 60)
-db string
    SQLite DB for baselines/history (default "sdsd_baselines.db")
-contamination float
    Isolation Forest contamination (default 0.1)
-estimators int
    Isolation Forest tree count (default 100)
-sample-size int
    Isolation Forest sample size per tree (default 256)
-verbose
    Enable verbose output
```

## What Is Alerted

Alert reasons include:

- mass file access
- recursive directory traversal
- archive creation detected
- abnormal entropy of accessed files
- burst filesystem activity
- suspicious transfer process activity
- strong baseline deviation

During baseline collection for new UIDs, additional immediate reasons are emitted for high-risk activity:

- suspicious transfer process activity during baseline
- privileged user burst activity during baseline
- extreme burst activity during baseline

## Notes

- Root privileges are required for fanotify.
- The monitor now performs strict path-prefix filtering before emitting events to detection.
- `parser.go` remains in the repository for legacy audit-line parsing references but is not in the active runtime pipeline.

## Documentation

See the documentation set in `Documentation/`:

- `Documentation/INDEX.md`
- `Documentation/QUICKSTART.md`
- `Documentation/README_AGENT.md`
- `Documentation/DEPLOYMENT.md`
- `Documentation/IMPLEMENTATION_SUMMARY.md`
- `Documentation/AUDIT_LOG_FORMAT.md`

