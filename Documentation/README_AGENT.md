# Linux Anomaly Detection Agent

A production-ready Blue Team anomaly detection agent written in Go that detects abnormal file access behavior and potential data exfiltration attempts using behavioral analysis and statistical machine learning.

## Features

- **Real-time Log Processing**: Efficiently tails `/var/log/audit/audit.log` with incremental reading
- **Behavioral Baseline**: Learns normal activity patterns during initial windows
- **Statistical Anomaly Detection**: Uses z-score analysis to detect deviations from baseline
- **Minimal Overhead**: Lightweight Go implementation suitable for endpoint agents
- **No External Dependencies**: Single binary, no database, no external services required
- **Production-Ready**: Handles log rotation, graceful shutdown, and edge cases

## Architecture

The agent is composed of modular, independent components:

```
┌─────────────┐
│  Log Reader │ → Tail audit.log efficiently with file seeking
└──────┬──────┘
       │ (raw lines)
       ↓
┌──────────────┐
│  Parser      │ → Extract events (open, read, execve)
└──────┬───────┘
       │ (structured events)
       ↓
┌──────────────┐
│  Aggregator  │ → Time-windowed feature vectors (10s windows)
└──────┬───────┘
       │ (feature vectors)
       ↓
┌──────────────┐
│  Detector    │ → Statistical anomaly detection (z-scores)
└──────┬───────┘
       │ (alerts)
       ↓
┌──────────────┐
│  Alerting    │ → Print high-fidelity alerts
└──────────────┘
```

## How It Works

### 1. **Log Reading** (`reader.go`)
- Opens `/var/log/audit/audit.log` and seeks to end (tail mode)
- Uses 64KB buffered reader for efficiency
- Handles file rotation by detecting size changes and reopening
- Polls at configurable interval (default: 500ms)

### 2. **Event Parsing** (`parser.go`)
- Extracts audit events: `SYSCALL` (open, openat, read) and `EXECVE`
- Parses: timestamp, UID, process name, file path
- Ignores malformed records gracefully
- Only processes successful operations

### 3. **Feature Aggregation** (`aggregator.go`)
- Groups events into 10-second windows
- Computes features per window:
  - `FileAccessCount`: Total file operations (open + read)
  - `UniqueFileCount`: Number of distinct files accessed
  - `ReadCount`: Read syscalls
  - `ExecCount`: Process executions
- Creates feature vectors for anomaly detection

### 4. **Anomaly Detection** (`detector.go`)
- **Baseline Learning**: Collects statistics from first N windows (default: 30)
- **Mean & StdDev**: Calculates per-feature baseline statistics
- **Z-Score Analysis**: For each window, computes z-score = |value - mean| / stddev
- **Threshold**: Alerts when max z-score exceeds threshold (default: 2.5)
- **Reason Extraction**: Identifies which feature(s) triggered the alert

### 5. **Alerting**
Formatted output:
```
[ALERT] time=2026-02-14 15:30:45 window=42 score=3.15 reason=High file access rate (z=3.15, count=156)
        FileAccessCount=156 UniqueFiles=89 ReadOps=78 ExecOps=3
```

## Syscall Event Types

### Detected Audit Events:
- **`open` / `openat`** (syscall 2, 257): File open operations - indicates potential scanning
- **`read`** (syscall 0): File read operations - bulk data reading patterns
- **`execve`**: Process execution - can indicate script execution for exfiltration

## Use Cases

1. **Data Exfiltration Detection**: Sudden spike in file reads and opens
2. **Unauthorized Access**: Unusual file access patterns by specific users
3. **Malware Activity**: Rapid execution of multiple processes
4. **Ransomware Behavior**: Systematic file access patterns on mountpoints

## Requirements

### System Requirements
- Linux system with `auditd` enabled and running
- Root or audit group privileges to read `/var/log/audit/audit.log`
- Go 1.21+ to build

### Installing auditd

**Ubuntu/Debian:**
```bash
sudo apt-get install auditd
sudo systemctl start auditd
sudo systemctl enable auditd
```

**RHEL/CentOS:**
```bash
sudo yum install audit
sudo systemctl start auditd
sudo systemctl enable auditd
```

### Verify auditd is running:
```bash
sudo service auditd status
sudo auditctl -l  # List current audit rules
```

## Building

```bash
# Navigate to the project directory
cd anomaly-detector

# Build the binary
go build -o anomaly-detector

# Or cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o anomaly-detector
```

## Usage

### Basic Usage (requires sudo)
```bash
sudo ./anomaly-detector
```

### With Custom Parameters
```bash
sudo ./anomaly-detector \
  -log /var/log/audit/audit.log \
  -window 10s \
  -baseline 30 \
  -threshold 2.5 \
  -poll 500ms \
  -verbose
```

### Flags

- `-log string`: Path to audit log (default: "/var/log/audit/audit.log")
- `-window duration`: Aggregation window size (default: 10s)
- `-baseline int`: Number of windows for baseline learning (default: 30)
- `-threshold float`: Z-score threshold for anomaly (default: 2.5)
- `-poll duration`: Poll interval for log file (default: 500ms)
- `-verbose`: Enable verbose output for debugging

### Example Run
```bash
$ sudo ./anomaly-detector -verbose
=== Linux Anomaly Detection Agent ===
Log file: /var/log/audit/audit.log
Window size: 10s
Baseline windows: 30
Z-score threshold: 2.5
Poll interval: 500ms
=====================================

[INFO] Agent started. Press Ctrl+C to stop.
[INFO] Learning baseline behavior...
[PARSE] cp: open on /etc/passwd
[PARSE] bash: read on /home/user/.bashrc
[STATS] FileAccess: mean=45.3, stddev=12.1
[STATS] UniqueFile: mean=23.5, stddev=5.2
[STATS] ReadCount: mean=28.1, stddev=8.3
[STATS] ExecCount: mean=3.2, stddev=1.1
[ALERT] time=2026-02-14 15:30:45 window=42 score=3.15 reason=High file access rate (z=3.15, count=156)
        FileAccessCount=156 UniqueFiles=89 ReadOps=78 ExecOps=3
```

## Output Format

### Info Messages
- `[INFO]`: Status and configuration
- `[STATS]`: Baseline statistics after learning phase

### Debug Messages (with -verbose)
- `[PARSE]`: Successfully parsed event
- `[WARN]`: Parse errors

### Alerts
- `[ALERT]`: Anomaly detected with score and reason
- Includes feature counts for investigation

## Performance Characteristics

- **Memory**: ~10-50 MB typical (buffered channels, baseline storage)
- **CPU**: <1% on normal activity, <3% during high audit load
- **I/O**: Minimal - single log file tail with 500ms polling
- **Latency**: Detection within one window period (10s default)

## Tuning for Your Environment

### Adjusting Sensitivity
**More Sensitive** (detect more anomalies):
```bash
sudo ./anomaly-detector -threshold 1.5 -window 5s
```

**Less Sensitive** (fewer false positives):
```bash
sudo ./anomaly-detector -threshold 3.5 -window 15s
```

### Adjusting Baseline Learning
**Faster baseline** (for quick testing):
```bash
sudo ./anomaly-detector -baseline 10
```

**Longer baseline** (for stability in varied environments):
```bash
sudo ./anomaly-detector -baseline 60
```

### High-Load Systems
Increase poll interval to reduce CPU:
```bash
sudo ./anomaly-detector -poll 1s
```

## Running as Service

### SystemD Service File
Create `/etc/systemd/system/anomaly-detector.service`:

```ini
[Unit]
Description=Linux Anomaly Detection Agent
After=auditd.service
Wants=auditd.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/anomaly-detector -threshold 2.5
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable anomaly-detector
sudo systemctl start anomaly-detector
sudo journalctl -u anomaly-detector -f
```

## Limitations & Future Enhancements

### Current Limitations
1. **Syscall Scope**: Only monitors open, read, execve (expandable to write, network, etc.)
2. **User Context**: Groups all users - could be per-user for more precision
3. **ML Model**: Uses simple z-score - could integrate more sophisticated ML
4. **Persistence**: Doesn't persist baseline across restarts
5. **False Positives**: Legitimate activity spikes may trigger alerts

### Possible Enhancements
- Per-user and per-process baselines
- Integration with SIEM systems (syslog, JSON output)
- Serialized baselines for training transfer
- More audit syscalls (write, socket, etc.)
- Network exfiltration detection
- Long Short-Term Memory (LSTM) or Isolation Forest models
- Rule-based whitelist for known safe processes

## Troubleshooting

### "Cannot access audit log" Error
```bash
# Ensure auditd is running
sudo systemctl status auditd

# Check file permissions
ls -la /var/log/audit/audit.log

# Run with sudo
sudo ./anomaly-detector
```

### No Events Being Parsed
1. Audit rules not enabled:
   ```bash
   sudo auditctl -a always,exit -S open,openat,read,execve -k file_access
   ```

2. Check audit log has content:
   ```bash
   sudo tail -20 /var/log/audit/audit.log
   ```

### High False Positive Rate
- Increase threshold: `-threshold 3.0`
- Increase baseline windows: `-baseline 60`
- Check for normal burst activity patterns

### CPU or Memory Issues
- Increase poll interval: `-poll 2s`
- Reduce window size: `-window 20s`
- Decrease buffer sizes in code (adjust channel capacities)

## Code Architecture

### File Structure
```
anomaly-detector/
├── main.go          # Entry point, component orchestration
├── types.go         # Shared data structures
├── reader.go        # Log file tailing
├── parser.go        # Audit event parsing
├── aggregator.go    # Feature engineering
├── detector.go      # Anomaly detection
├── go.mod           # Go module definition
└── README.md        # This file
```

### Key Design Decisions

1. **Goroutine-Based Architecture**: Each component runs independently with channel communication
2. **No Locks Required**: Channels provide thread-safe communication
3. **Streaming Processing**: Never loads entire log into memory
4. **Graceful Degradation**: Dropped channels never crash the system
5. **Zero External Dependencies**: Pure Go standard library

## Security Considerations

1. **Requires Root Access**: Audit logs require elevated privileges
2. **Process Isolation**: Run as dedicated user with minimal privileges
3. **Output Handling**: Alerts could leak sensitive paths - monitor carefully
4. **Log Protection**: Protect audit logs from unauthorized access
5. **Baseline Poisoning**: Ensure baseline learning happens in normal conditions

## References

- Linux Audit Framework: https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/chap-system_auditing
- Audit Syscalls: https://man7.org/linux/man-pages/man2/open.2.html
- Z-Score Anomaly Detection: https://en.wikipedia.org/wiki/Standard_score
- Go Concurrency: https://go.dev/blog/pipelines

## License

See LICENSE file

## Contributing

Contributions welcome! Areas for improvement:
- Additional audit syscalls
- Alternative ML models
- SIEM integration
- Performance optimizations
- Test coverage

---

**Version**: 1.0  
**Last Updated**: February 2026  
**Status**: Production-Ready MVP
