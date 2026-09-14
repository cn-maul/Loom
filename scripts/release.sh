#!/usr/bin/env sh
# release.sh - 发布管线（POSIX 版，CI/Linux/macOS 用）：vet、测试、前端构建、后端构建、迁移冒烟。
# Windows 用户请用 scripts/release.ps1。
set -eu
cd "$(dirname "$0")/.."

echo "=== 1/6 go vet ==="
go vet ./...

echo "=== 2/6 go test ==="
go test ./...

echo "=== 3/6 frontend build ==="
( cd web
  [ -d node_modules ] || npm install
  npm run build )

echo "=== 4/6 go build ==="
rm -rf internal/frontend/dist
cp -r web/dist internal/frontend/dist
go build -o relationship ./cmd/server

echo "=== 5/6 migration + smoke ==="
TMP="$(mktemp -d)"
PORT=8178
cat > "$TMP/config.yaml" <<EOF
server:
  host: 127.0.0.1
  port: $PORT
database:
  path: smoke.db
llm:
  endpoint: http://127.0.0.1:1
  async_extract: false
  allow_remote: false
backup:
  enabled: false
maintenance:
  audit_retention_days: 0
EOF
( cd "$TMP" && "$OLDPWD/relationship" config.yaml > out.log 2> err.log & echo $! > pid ) || true
BASE="http://127.0.0.1:$PORT"
ready=0
i=0
while [ $i -lt 60 ]; do
  sleep 0.5
  i=$((i+1))
  [ -f "$TMP/pid" ] || break
  kill -0 "$(cat "$TMP/pid")" 2>/dev/null || break
  code="$(curl -s --noproxy '*' -o "$TMP/body.json" -w '%{http_code}' "$BASE/api/config" || true)"
  if [ "$code" = "200" ]; then ready=1; break; fi
done
if [ "$ready" != "1" ]; then
  echo "server did not become ready; logs:" >&2
  cat "$TMP/out.log" "$TMP/err.log" >&2 || true
  exit 1
fi

curl -s --noproxy '*' -D - -o /dev/null "$BASE/api/config" | grep -qi "x-api-version: 1" \
  || { echo "missing X-API-Version header"; exit 1; }

# Failures under /api answer in the error envelope, whether Echo rejected a
# missing row (handler path) or an unknown route (router path). Neither may
# fall through to the SPA shell.
code="$(curl -s --noproxy '*' -o "$TMP/404.json" -w '%{http_code}' "$BASE/api/persons/does-not-exist")"
grep -q '"ok":false' "$TMP/404.json" && [ "$code" = "404" ] \
  || { echo "404 envelope broken"; exit 1; }

code="$(curl -s --noproxy '*' -o "$TMP/unrouted.json" -w '%{http_code}' "$BASE/api/no-such-endpoint")"
grep -q '"ok":false' "$TMP/unrouted.json" && [ "$code" = "404" ] \
  || { echo "unrouted /api path did not answer 404 JSON"; exit 1; }

if [ -f "$TMP/pid" ]; then kill "$(cat "$TMP/pid")" 2>/dev/null || true; fi
rm -rf "$TMP"

echo "=== 6/6 result ==="
echo "RELEASE PIPELINE PASSED: relationship"
