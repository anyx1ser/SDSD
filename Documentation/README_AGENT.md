# SDSD Agent Guide

## Overview

SDSD is a host-based anomaly detector focused on filesystem behavior.

Current implementation characteristics:

- fanotify event source
- per-UID sliding-window feature extraction
- hybrid anomaly scoring (z-score + Isolation Forest)
- SQLite persistence for feature vectors, alerts, and per-UID baselines
- cold-start risk alerting for new UIDs

## Active Architecture

```text
FanotifyMonitor
  -> Aggregator (per UID)
  -> AnomalyDetector (per UID state)
  -> Alerts + Database
```

## Why Per-UID Modeling

Different users and service accounts have different normal behavior. Per-UID baselines reduce cross-user noise and improve anomaly fidelity.

## Feature Signals

The detector uses aggregated features such as:

- total accesses, reads, writes, access rate
- unique files/directories and ratio uniqueness
- process diversity entropy
- temporal burst signal (`MaxEventsInShortInterval`)
- archive-like behavior
- entropy over extension/directory/filename patterns
- transfer process presence (`rsync`, `rclone`, `scp`, `sftp`)

## Alert Logic

An alert can trigger from:

1. Baseline deviation beyond threshold
2. Isolation Forest anomaly confidence
3. Reason-tag heuristics with sufficient final score
4. Cold-start guardrails during baseline collection

Representative reason tags:

- mass file access
- recursive directory traversal
- archive creation detected
- abnormal entropy of accessed files
- burst filesystem activity
- suspicious transfer process activity
- strong baseline deviation

Cold-start reason tags (before baseline is ready):

- suspicious transfer process activity during baseline
- privileged user burst activity during baseline
- extreme burst activity during baseline

## Runtime Options

```text
-paths             monitored path prefixes
-window            aggregation window duration
-step              sliding step duration
-baseline          windows required before per-UID training
-db                sqlite file path
-contamination     Isolation Forest contamination
-estimators        Isolation Forest tree count
-sample-size       Isolation Forest sample size
-verbose           verbose mode
```

## Persistence

SQLite stores:

- `baselines`: per-UID trained model and statistics
- `feature_vectors`: window-level telemetry
- `alerts`: emitted alerts with reason and score

This enables restart continuity and historical analysis.

## Operational Guidance

- Start with default settings first.
- Keep monitored paths minimal and purposeful.
- Review frequent reason tags and tune windows/baseline before changing model hyperparameters.
- Investigate any cold-start alert from privileged UIDs promptly.

## Legacy Note

`parser.go` and older audit-log documentation are kept for historical compatibility, but active runtime detection is fanotify-based.

