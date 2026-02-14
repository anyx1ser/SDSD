# Quick Start Guide - Anomaly Detector

## 30-Second Setup

### Prerequisites
- Linux system with `auditd` installed
- Go 1.21+ installed
- `sudo` access

### Installation

```bash
# 1. Build the agent
cd anomaly-detector
go build -o anomaly-detector

# 2. Run it
sudo ./anomaly-detector

# You should see:
# [INFO] Agent started. Press Ctrl+C to stop.
# [INFO] Learning baseline behavior...
```

Wait ~5 minutes for baseline learning (30 windows × 10 seconds).

## What to Expect

### During Baseline Learning
```
[INFO] Agent started. Press Ctrl+C to stop.
[INFO] Learning baseline behavior...
```
Agent collects statistics from normal system activity.

### After Baseline Complete
```
[STATS] FileAccess: mean=45.3, stddev=12.1
[STATS] UniqueFile: mean=23.5, stddev=5.2
[STATS] ReadCount: mean=28.1, stddev=8.3
[STATS] ExecCount: mean=3.2, stddev=1.1
```

### When Anomalies Detected
```
[ALERT] time=2026-02-14 15:30:45 window=42 score=3.15 reason=High file access rate (z=3.15, count=156)
        FileAccessCount=156 UniqueFiles=89 ReadOps=78 ExecOps=3
```

## Common Commands

```bash
# Run with default settings
sudo ./anomaly-detector

# Run verbose (shows all parsed events)
sudo ./anomaly-detector -verbose

# More sensitive detection
sudo ./anomaly-detector -threshold 1.5

# Less sensitive (fewer false positives)
sudo ./anomaly-detector -threshold 3.5

# Faster response (5 second windows)
sudo ./anomaly-detector -window 5s

# Stop the agent
Ctrl+C
```

## Testing the Agent

### Generate Test Activity

```bash
# In another terminal, generate file access patterns
while true; do
    cat /var/log/auth.log > /dev/null
    cat /etc/hosts > /dev/null
    cat /proc/cpuinfo > /dev/null
    sleep 0.1
done
```

Watch for alerts in the main agent window.

## Installation as Service

```bash
# Build
go build -o anomaly-detector

# Install binary
sudo cp anomaly-detector /usr/local/bin/

# Create service (see DEPLOYMENT.md for full config)
sudo tee /etc/systemd/system/anomaly-detector.service > /dev/null <<EOF
[Unit]
Description=Linux Anomaly Detection Agent
After=auditd.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/anomaly-detector
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable anomaly-detector
sudo systemctl start anomaly-detector

# Check status
sudo systemctl status anomaly-detector

# View logs
sudo journalctl -u anomaly-detector -f
```

## Tuning for Your Environment

### Problem: Too Many False Positives

```bash
# Increase threshold (less sensitive)
sudo ./anomaly-detector -threshold 3.0

# Or increase window size
sudo ./anomaly-detector -window 20s
```

### Problem: Missing Anomalies

```bash
# Decrease threshold (more sensitive)
sudo ./anomaly-detector -threshold 1.5

# Or decrease window size
sudo ./anomaly-detector -window 5s
```

### Problem: High CPU Usage

```bash
# Increase poll interval
sudo ./anomaly-detector -poll 2s

# Or increase window size
sudo ./anomaly-detector -window 20s
```

## Command-Line Options

```
-log string
    Path to audit log file (default "/var/log/audit/audit.log")

-window duration
    Aggregation window size (default 10s)

-baseline int
    Number of windows for baseline learning (default 30)

-threshold float
    Z-score threshold for anomaly detection (default 2.5)

-poll duration
    Poll interval for log file (default 500ms)

-verbose
    Enable verbose output for debugging
```

## Files Overview

```
anomaly-detector/
├── main.go              # Entry point, component orchestration
├── types.go             # Shared data structures  
├── reader.go            # Audit log tailing
├── parser.go            # Event parsing
├── aggregator.go        # Feature engineering
├── detector.go          # Anomaly detection
├── go.mod               # Go module definition
├── README_AGENT.md      # Full documentation
├── DEPLOYMENT.md        # Advanced deployment guide
├── QUICKSTART.md        # This file
├── Makefile             # Build automation
└── demo.sh              # Demo script
```

## Understanding Output

```
[INFO] message           - Informational messages
[STATS] statistics       - Baseline statistics (shown once)
[PARSE] event details    - Individual events (with -verbose)
[ALERT] anomaly detection- Anomalies detected
[WARN] warning message   - Warnings or issues
```

## Next Steps

1. **Read Full Documentation**: See [README_AGENT.md](README_AGENT.md)
2. **Review Architecture**: Check [README_AGENT.md#architecture](README_AGENT.md#architecture)
3. **Production Deployment**: Follow [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Tune for Your Environment**: See tuning sections above
5. **Set Up Service**: Configure systemd service for auto-start
6. **Monitor Alerts**: Set up SIEM integration or email alerts

## Troubleshooting

### "Cannot access audit log" Error
```bash
# Ensure running with sudo
sudo ./anomaly-detector

# Check auditd is running
sudo systemctl status auditd

# Verify log file exists and is readable
ls -la /var/log/audit/audit.log
```

### No Events Being Detected
```bash
# Enable verbose mode to see parsing
sudo ./anomaly-detector -verbose

# Add audit rules if missing
sudo auditctl -a always,exit -S open,openat,read,execve -k file_access

# Check audit log has events
sudo tail -20 /var/log/audit/audit.log
```

### Help & Documentation

```bash
# Get help on options
./anomaly-detector -h

# See full documentation
cat README_AGENT.md

# Check deployment guide
cat DEPLOYMENT.md
```

## Performance Expectations

| Metric | Value |
|--------|-------|
| Memory Usage | 10-50 MB |
| CPU Usage | <1% normal, <3% high load |
| Detection Latency | 10-20 seconds |
| Startup Time | <1 second |

## What Gets Detected

- **Rapid file access**: Spike in open/read syscalls (potential scanning)
- **Bulk data reading**: Multiple unique files accessed in short window
- **Unusual execution**: Sudden process execution patterns
- **Suspicious processes**: Anomalous behavior for specific users

## What Won't Be Detected (Yet)

- Network-based exfiltration
- In-memory malware
- Privilege escalation
- Kernel-level rootkits
- Activities using already-running processes

## Support & Issues

Check documentation:
1. README_AGENT.md - Comprehensive guide
2. DEPLOYMENT.md - Production setup
3. Code comments - Implementation details

---

**Ready to detect anomalies!** 🛡️

Questions? Check the full README_AGENT.md for comprehensive documentation.
