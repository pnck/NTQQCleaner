#!/usr/bin/env bash
# CLI smoke test: builds the CLI-only binary, creates a fake QQ data root
# (docs/05 §6), runs `scan --json`, then executes the manifest with
# `clean --force` into a backup dir and verifies the redlines.
set -euo pipefail

HASH=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb1
QQ=10001
WORK=$(mktemp -d)
BIN="$WORK/qq-cleaner-cli"
GO=$(command -v go)
echo "workdir: $WORK"

"$GO" build -o "$BIN" .

ROOT="$WORK/qq"
mkdir -p "$ROOT/global/nt_data/mmkv" "$ROOT/global/nt_data/Login"
printf 'k_/data/nt_qq_%s/nt_data/flashfransfer+%s_flash_transfer_document_cache_key' "$HASH" "$QQ" \
  > "$ROOT/global/nt_data/mmkv/mmkv.default"
touch "$ROOT/global/nt_data/Login/.$QQ"

D="$ROOT/nt_qq_$HASH/nt_data"
mkdir -p "$D/Pic/2023-01/Ori" "$D/Pic/2023-01/Thumb" "$D/Pic/2026-07/Thumb" "$D/Pic/2026-07/OriTemp" "$D/mmkv" "$D/nt_db"
echo aaaa > "$D/Pic/2023-01/Ori/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01.jpg"
echo bbbb > "$D/Pic/2023-01/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg"
echo cccc > "$D/Pic/2026-07/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg"
echo dddd > "$D/Pic/2026-07/OriTemp/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp"
echo nope > "$D/mmkv/mmkv.default"       # must never appear
echo nope > "$D/nt_db/message.db"       # must never appear
touch -d "2023-01-15" "$D/Pic/2023-01/Ori"/*.jpg "$D/Pic/2023-01/Thumb"/*.jpg
touch -d "2026-07-10" "$D/Pic/2026-07/Thumb"/*.jpg
touch -d "2026-08-08" "$D/Pic/2026-07/OriTemp"/*.tmp  # 4 days old: temp → safe

echo "== scan (dry-run) =="
"$BIN" scan --root "$ROOT" --min-age-days 0
"$BIN" scan --root "$ROOT" --min-age-days 0 --json > "$WORK/manifest.json"

echo "== scan must not have deleted anything =="
[ -f "$D/Pic/2026-07/OriTemp/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp" ] || { echo "FAIL: dry-run deleted a file"; exit 1; }

echo "== clean refuses without --force =="
if "$BIN" clean --file "$WORK/manifest.json"; then echo "FAIL: clean ran without --force"; exit 1; fi

echo "== clean with --force + backup =="
echo yes | "$BIN" clean --file "$WORK/manifest.json" --force --backup-dir "$WORK/backup" --audit-log "$WORK/audit.log"

[ ! -f "$D/Pic/2026-07/OriTemp/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp" ] || { echo "FAIL: temp not cleaned"; exit 1; }
[ -f "$WORK/backup/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp" ] || { echo "FAIL: no backup copy"; exit 1; }
[ -s "$WORK/audit.log" ] || { echo "FAIL: audit log empty"; exit 1; }
[ -f "$D/mmkv/mmkv.default" ] && [ -f "$D/nt_db/message.db" ] || { echo "FAIL: blacklisted path touched"; exit 1; }

echo "SMOKE OK (workdir kept for inspection: $WORK)"
