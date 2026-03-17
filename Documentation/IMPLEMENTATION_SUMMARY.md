# Implementation Summary

## Current State

SDSD is implemented as a streaming Go pipeline with:

- Linux fanotify event ingestion
- per-UID feature aggregation over sliding windows
- hybrid anomaly scoring (z-score + Isolation Forest)
- SQLite persistence for baseline continuity
- cold-start alerting for high-risk behavior during baseline

## Component Map

- `main.go`: flag parsing and pipeline bootstrap
- `reader.go`: fanotify monitor + strict path filtering
- `aggregator.go`: per-UID buffers and feature extraction
- `detector.go`: per-UID model lifecycle, scoring, alert generation
- `database.go`: SQLite schema and CRUD
- `isolation_forest.go`: IF model implementation/serialization
- `entropy.go`: Shannon entropy helpers
- `types.go`: shared structs

Legacy parser artifacts:

- `parser.go` remains in repo but is not active in main runtime pipeline.

## Detection Flow

1. Read fanotify event
2. Attribute event to UID
3. Aggregate into UID window features
4. If UID baseline exists: score with hybrid detector
5. If UID baseline collecting: apply cold-start safeguards, then continue training
6. Emit alerts and persist telemetry

## Notable Recent Changes

- Strict path-prefix filter before events are emitted from monitor
- Transfer-process signal (`rsync`, `rclone`, `scp`, `sftp`) added to feature vector
- Cold-start risk alerts added to avoid blind window for new UIDs

## Alert Reasons

Runtime can emit reasons including:

- mass file access
- recursive directory traversal
- archive creation detected
- abnormal entropy of accessed files
- burst filesystem activity
- suspicious transfer process activity
- strong baseline deviation

Cold-start reasons:

- suspicious transfer process activity during baseline
- privileged user burst activity during baseline
- extreme burst activity during baseline

## Data Persistence

SQLite tables:

- `baselines`
- `feature_vectors`
- `alerts`

This allows warm restarts and post-incident review.

