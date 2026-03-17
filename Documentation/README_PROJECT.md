# SDSD Project Overview

SDSD is a Linux filesystem anomaly detector for Blue Team workflows.

## Highlights

- fanotify-based telemetry
- strict monitored-path filtering
- per-UID behavior modeling
- Isolation Forest + statistical deviation scoring
- SQLite persistence of baselines, feature vectors, and alerts
- cold-start protection for new UIDs

## Build and Run

```bash
go build -o anomaly-detector
sudo ./anomaly-detector
```

## Current CLI Surface

```text
-paths, -window, -step, -baseline, -db,
-contamination, -estimators, -sample-size, -verbose
```

## Security Relevance

The agent is designed to surface suspicious filesystem behavior such as:

- high-rate bursts
- broad directory traversal
- archive-and-read patterns
- transfer-tool based collection behavior

## Documentation

- `INDEX.md`
- `QUICKSTART.md`
- `README_AGENT.md`
- `DEPLOYMENT.md`
- `IMPLEMENTATION_SUMMARY.md`

