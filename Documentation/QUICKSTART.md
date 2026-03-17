# Quick Start Guide

## Prerequisites

- Linux host
- Go 1.21+
- Root privileges (`sudo`) for fanotify

No auditd configuration is required for the active runtime path.

## Build and Run

```bash
cd /path/to/SDSD
go build -o anomaly-detector
sudo ./anomaly-detector
```

Expected startup output includes:

```text
=== SDSD — Isolation Forest Anomaly Detection Agent ===
Monitor paths  : /home, /etc
Window size    : 10s
Window step    : 2s
Baseline windows/UID : 60
...
[INFO] Agent started. Press Ctrl+C to stop.
```

## Generate Test Activity

In a second terminal:

```bash
while true; do
    cat /etc/hosts > /dev/null
    cat /home/$USER/.bashrc > /dev/null 2>&1 || true
    sleep 0.1
done
```

To simulate transfer-tool behavior:

```bash
rsync -a /etc/ /tmp/sdsd-rsync-test --delete --exclude='*.tmp'
```

## Core Flags

```bash
sudo ./anomaly-detector \
  -paths /home,/etc \
  -window 10s \
  -step 2s \
  -baseline 60 \
  -db sdsd_baselines.db \
  -contamination 0.10 \
  -estimators 100 \
  -sample-size 256
```

## Alert Semantics

Reasons are comma-separated and can include multiple tags in one alert.

Common examples:

- burst filesystem activity
- mass file access
- suspicious transfer process activity

For new UIDs (during baseline), cold-start alerts can still trigger immediately when behavior is high-risk.

## Troubleshooting

### No events

- Verify you started with root privileges.
- Verify the paths passed to `-paths` exist.
- Generate activity inside monitored paths.

### Too many alerts

- Increase `-baseline` (for more stable per-UID baseline)
- Increase `-window`
- Lower sensitivity by increasing `-contamination` carefully only if you understand tradeoffs

### Path confusion

The runtime now drops events outside configured paths before detection.

