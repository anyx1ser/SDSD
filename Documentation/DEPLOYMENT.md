# Deployment Guide

## Scope

This guide is for deploying the current SDSD runtime (fanotify-based monitoring with per-UID ML state and SQLite persistence).

## 1. Build and Install

```bash
cd /path/to/SDSD
go build -o anomaly-detector
sudo install -m 0755 anomaly-detector /usr/local/bin/anomaly-detector
```

## 2. Create Runtime Directories

```bash
sudo mkdir -p /var/lib/sdsd
sudo mkdir -p /var/log/sdsd
```

## 3. Systemd Service

Create `/etc/systemd/system/sdsd.service`:

```ini
[Unit]
Description=SDSD Filesystem Anomaly Detector
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/sdsd
ExecStart=/usr/local/bin/anomaly-detector \
  -paths /home,/etc \
  -window 10s \
  -step 2s \
  -baseline 60 \
  -db /var/lib/sdsd/sdsd_baselines.db \
  -contamination 0.10 \
  -estimators 100 \
  -sample-size 256
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable sdsd
sudo systemctl start sdsd
sudo systemctl status sdsd
```

View logs:

```bash
sudo journalctl -u sdsd -f
```

## 4. Security Notes

- Root is required for fanotify.
- Restrict write access to `/var/lib/sdsd`.
- Forward journal alerts to your SIEM.
- Keep monitored paths narrow (`-paths`) to reduce noise.

## 5. Tuning Profiles

### Conservative

```text
-window 15s -step 3s -baseline 100
```

### Balanced (default-like)

```text
-window 10s -step 2s -baseline 60
```

### Fast Reaction

```text
-window 5s -step 1s -baseline 80
```

## 6. Backup and Recovery

Persist and back up:

- SQLite file from `-db`

To reset learning for all UIDs:

1. stop service
2. move or delete DB
3. restart service

Example:

```bash
sudo systemctl stop sdsd
sudo mv /var/lib/sdsd/sdsd_baselines.db /var/lib/sdsd/sdsd_baselines.db.bak
sudo systemctl start sdsd
```

## 7. Known Operational Behavior

- New UIDs begin in baseline collection.
- High-risk cold-start behavior can still emit alerts before training completes.
- Events outside configured `-paths` are dropped before detection.

