#!/usr/bin/env bash
# Starts a built lucarne binary and checks that it serves its probes, the
# embedded UI, its assets, a deep link and the contract endpoint, and no run
# without a source. Usage: smoke.sh ./lucarne
set -euo pipefail

readonly BINARY=${1:?usage: smoke.sh <lucarne binary>}
readonly ADDR=127.0.0.1:18080
readonly BASE="http://${ADDR}"
readonly RUN_LINK=/runs/example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b
readonly START_ATTEMPTS=50
readonly START_INTERVAL=0.1

"$BINARY" -listen "$ADDR" &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT

for _ in $(seq "$START_ATTEMPTS"); do
  curl -fsS "$BASE/healthz" >/dev/null 2>&1 && break
  sleep "$START_INTERVAL"
done

curl -fsS "$BASE/healthz" | grep -qx ok
curl -fsS "$BASE/readyz" | grep -qx ready
index=$(curl -fsS "$BASE$RUN_LINK")
asset=$(grep -o '/assets/[^"]*\.js' <<<"$index" | head -n 1)
[ -n "$asset" ] || { echo "the index names no script: the frontend is not embedded" >&2; exit 1; }
curl -fsS -o /dev/null "$BASE$asset"
curl -fsS "$BASE/api/v1/contract" | grep -q '"schema":"lucarne.transcript/v1"'
# Started without -namespace, it reads no cluster: run endpoints answer 501.
status=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1$RUN_LINK")
[ "$status" = 501 ] || { echo "run snapshot answered $status without a source, want 501" >&2; exit 1; }
status=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/runs")
[ "$status" = 501 ] || { echo "run list answered $status without a source, want 501" >&2; exit 1; }
echo "smoke: ok ($asset)"
