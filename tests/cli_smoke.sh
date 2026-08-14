#!/usr/bin/env bash
# CLI smoke test: builds the CLI-only binary, creates a fake QQ data root
# (docs/05 §6), runs `scan --json`, then executes the manifest with
# `clean --force` into a backup dir and verifies the redlines.
set -euo pipefail

HASH=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb1
QQ=10001
WORK=$(mktemp -d)
BIN="$WORK/ntqq-cleaner-cli"
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
# 各文件大小唯一：不触发内容哈希（同内容 ⇒ 同大小，无孪生候选）
echo aaaa > "$D/Pic/2023-01/Ori/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01.jpg"
echo bbbbb > "$D/Pic/2023-01/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg"
echo cccccc > "$D/Pic/2026-07/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg"
echo ddddddd > "$D/Pic/2026-07/OriTemp/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp"
# 内容重复对：同字节、不同名、不同月份（二次扫描必须识别）
printf 'identical-content-payload' > "$D/Pic/2023-01/Thumb/cccccccccccccccccccccccccccccccc01_720.jpg"
cp "$D/Pic/2023-01/Thumb/cccccccccccccccccccccccccccccccc01_720.jpg" \
   "$D/Pic/2026-07/Thumb/cccccccccccccccccccccccccccccccc02_720.jpg"
echo nope > "$D/mmkv/mmkv.default"       # must never appear
echo nope > "$D/nt_db/message.db"       # must never appear
touch -d "2023-01-15" "$D/Pic/2023-01/Ori"/*.jpg "$D/Pic/2023-01/Thumb"/*.jpg
touch -d "2026-07-10" "$D/Pic/2026-07/Thumb"/*.jpg
touch -d "2026-08-08" "$D/Pic/2026-07/OriTemp"/*.tmp  # 4 days old: temp → safe

echo "== scan (dry-run) =="
"$BIN" scan --root "$ROOT" --min-age-days 0 > "$WORK/scan.txt"
cat "$WORK/scan.txt"
grep -q "内容哈希" "$WORK/scan.txt" || { echo "FAIL: scan report missing 内容哈希 line"; exit 1; }
"$BIN" scan --root "$ROOT" --min-age-days 0 --json > "$WORK/manifest.json"

echo "== content hash second pass (real binary end-to-end) =="
HASH_COUNT=$(grep -o '"contentHash"' "$WORK/manifest.json" | wc -l | tr -d ' ')
[ "$HASH_COUNT" = "2" ] || { echo "FAIL: contentHash entries = $HASH_COUNT, want 2 (only the identical pair)"; exit 1; }
HASHES=$(grep -o '"contentHash": "[0-9a-f]\{64\}"' "$WORK/manifest.json" | sed 's/.*"\([0-9a-f]\{64\}\)"/\1/')
UNIQ=$(echo "$HASHES" | sort -u | wc -l | tr -d ' ')
[ "$UNIQ" = "1" ] || { echo "FAIL: identical pair must share one hash, got: $HASHES"; exit 1; }

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
