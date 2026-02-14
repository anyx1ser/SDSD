#!/bin/bash

# Demo script to test the anomaly detector
# This script generates synthetic audit events for testing

set -e

BINARY="./anomaly-detector"
LOG_FILE="/tmp/audit_demo.log"

echo "=== Anomaly Detector Demo Script ==="
echo ""

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo "Building binary first..."
    go build -o "$BINARY"
fi

echo "Creating demo audit log at $LOG_FILE..."
cat > "$LOG_FILE" << 'EOF'
type=SYSCALL msg=audit(1707901845.123): arch=c000003e syscall=2 success=yes exit=3 a0=0x7fffffff a1=0x0 a2=0x0 a3=0x0 items=1 ppid=1234 pid=5678 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts0 ses=1 comm="bash" exe="/bin/bash"
type=SYSCALL msg=audit(1707901845.234): arch=c000003e syscall=257 success=yes exit=4 a0=0xffffff9c a1=0x55555555 a2=0x0 a3=0x0 items=1 ppid=1234 pid=5678 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts0 ses=1 comm="cp" exe="/bin/cp" name="/etc/passwd"
type=SYSCALL msg=audit(1707901845.345): arch=c000003e syscall=0 success=yes exit=1024 a0=0x4 a1=0x7fffffff a2=0x1000 a3=0x0 items=0 ppid=1234 pid=5679 uid=1000 gid=1000 euid=1000 egid=1000 fsuid=1000 fsgid=1000 tty=pts0 ses=1 comm="cat" exe="/bin/cat"
type=EXECVE msg=audit(1707901845.456): argc=2 a0="/bin/ls" a1="-la" uid=1000
EOF

echo "Demo log created at $LOG_FILE"
echo ""
echo "Running detector with demo log (5 second demo)..."
echo ""

# Run detector with demo log
timeout 5s sudo "$BINARY" \
    -log "$LOG_FILE" \
    -window 2s \
    -baseline 2 \
    -threshold 1.5 \
    -poll 100ms \
    -verbose || true

echo ""
echo "Demo complete!"
echo ""
echo "To run on production audit log:"
echo "  sudo ./anomaly-detector"
echo ""
echo "To run with custom settings:"
echo "  sudo ./anomaly-detector -threshold 2.5 -baseline 30 -window 10s"
