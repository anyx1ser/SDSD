#!/usr/bin/env bash
set -euo pipefail

# Test-only workload generator for SDSD:
# 1) create many files in nested directories
# 2) read them in quick succession
# 3) create a compressed archive

WORKDIR="${1:-/tmp/sdsd_archive_trigger}"
FILE_COUNT="${2:-2500}"
SUBDIR_COUNT="${3:-30}"

if ! [[ "$FILE_COUNT" =~ ^[0-9]+$ ]] || [ "$FILE_COUNT" -lt 1 ]; then
  echo "FILE_COUNT must be a positive integer"
  exit 1
fi

if ! [[ "$SUBDIR_COUNT" =~ ^[0-9]+$ ]] || [ "$SUBDIR_COUNT" -lt 1 ]; then
  echo "SUBDIR_COUNT must be a positive integer"
  exit 1
fi

SRC_DIR="$WORKDIR/source"
OUT_DIR="$WORKDIR/out"
STAMP="$(date +%Y%m%d_%H%M%S)"
ARCHIVE_PATH="$OUT_DIR/collection_${STAMP}.tar.gz"

echo "[+] Preparing test data under: $WORKDIR"
rm -rf "$WORKDIR"
mkdir -p "$SRC_DIR" "$OUT_DIR"

echo "[+] Creating $FILE_COUNT files across $SUBDIR_COUNT directories..."
for i in $(seq 1 "$FILE_COUNT"); do
  d1=$((i % SUBDIR_COUNT))
  d2=$((i % 7))
  case $((i % 4)) in
    0) ext="txt" ;;
    1) ext="log" ;;
    2) ext="csv" ;;
    3) ext="conf" ;;
  esac

  target_dir="$SRC_DIR/dir_${d1}/set_${d2}"
  mkdir -p "$target_dir"
  file_path="$target_dir/file_${i}.${ext}"

  {
    echo "record=$i"
    echo "created_at=$(date +%s)"
    echo "host=$(hostname)"
    head -c 256 /dev/urandom | base64
  } > "$file_path"
done

echo "[+] Reading files quickly to simulate harvesting..."
find "$SRC_DIR" -type f -print0 | xargs -0 cat >/dev/null

echo "[+] Creating archive: $ARCHIVE_PATH"
tar -czf "$ARCHIVE_PATH" -C "$SRC_DIR" .

echo "[+] Done"
echo "    Source files : $FILE_COUNT"
echo "    Archive      : $ARCHIVE_PATH"
echo ""
echo "Usage examples:"
echo "  $0"
echo "  $0 /tmp/sdsd_archive_trigger 4000 40"
