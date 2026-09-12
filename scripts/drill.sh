#!/usr/bin/env sh
# drill.sh - 月度备份恢复演练（POSIX 版，CI/Linux/macOS 用）。Windows 用 scripts/drill.ps1。
# 全链路：建数据 → 快照 → 校验 → 删除 → 两阶段恢复 → 重启 → 验证。
set -eu
cd "$(dirname "$0")/.."
[ -x ./relationship ] || { echo "先运行 scripts/release.sh 生成 relationship"; exit 1; }

TMP="$(mktemp -d)"
mkdir -p "$TMP/backups"
PORT=8180
BASE="http://127.0.0.1:$PORT"
cat > "$TMP/config.yaml" <<EOF
server:
  host: 127.0.0.1
  port: $PORT
database:
  path: drill.db
llm:
  endpoint: http://127.0.0.1:1
  async_extract: false
  allow_remote: false
backup:
  enabled: false
  dir: backups
maintenance:
  audit_retention_days: 0
EOF

start_server() {
  ( cd "$TMP" && "$OLDPWD/relationship" config.yaml > out.log 2> err.log & echo $! > pid )
  i=0
  while [ $i -lt 60 ]; do
    sleep 0.5; i=$((i+1))
    kill -0 "$(cat "$TMP/pid")" 2>/dev/null || { cat "$TMP/out.log" "$TMP/err.log" >&2; exit 1; }
    code="$(curl -s --noproxy '*' -o /dev/null -w '%{http_code}' "$BASE/api/config" || true)"
    [ "$code" = "200" ] && return 0
  done
  echo "server did not start" >&2; exit 1
}

stop_server() { [ -f "$TMP/pid" ] && kill "$(cat "$TMP/pid")" 2>/dev/null || true; sleep 0.5; }

api() { # api METHOD PATH [JSON]
  m="$1"; p="$2"; j="$3"
  if [ -n "$j" ]; then
    curl -s --noproxy '*' -X "$m" -H 'Content-Type: application/json' -d "$j" -w '\n%{http_code}' "$BASE$p"
  else
    curl -s --noproxy '*' -X "$m" -w '\n%{http_code}' "$BASE$p"
  fi
}
check() { # check EXPECTED_CODE RESPONSE
  last_line="$(printf '%s' "$2" | tail -n 1)"
  [ "$last_line" = "$1" ] || { echo "expected $1 got $last_line: $2" >&2; exit 1; }
  printf '%s' "$2" | sed '$d'
}

echo "[1/6] 写入演练数据"
resp="$(api POST /api/persons '{"name":"恢复演练人物"}')"
body="$(check 201 "$resp")"
person_id="$(printf '%s' "$body" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"
resp="$(api POST /api/events "{\"person_id\":\"$person_id\",\"raw_text\":\"演练记录：备份恢复验证\",\"event_date\":\"2026-09-12\"}")"
body="$(check 201 "$resp")"
event_id="$(printf '%s' "$body" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"

echo "[2/6] 创建快照"
resp="$(api POST /api/backups '')"
body="$(check 201 "$resp")"
file="$(printf '%s' "$body" | grep -o '"name":"[^"]*"' | head -1 | cut -d'"' -f4)"

echo "[3/6] 校验快照"
resp="$(api POST /api/backups/validate "{\"file\":\"$file\"}")"
check 200 "$resp" >/dev/null

echo "[4/6] 删除数据（模拟灾难）"
# 删除记录本身：删人按设计是 SET NULL（共同记录保留），记录不会消失。
check 200 "$(api DELETE "/api/events/$event_id" '')" >/dev/null
check 404 "$(api GET "/api/events/$event_id" '')" >/dev/null

echo "[5/6] 暂存恢复并重启"
check 202 "$(api POST /api/backups/restore "{\"file\":\"$file\"}")" >/dev/null
stop_server
start_server

echo "[6/6] 验证恢复"
check 200 "$(api GET "/api/persons/$person_id" '')" >/dev/null
check 200 "$(api GET "/api/events/$event_id" '')" >/dev/null

stop_server
rm -rf "$TMP"
echo "DRILL PASSED - 备份恢复链路完整可用"
