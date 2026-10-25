#!/bin/bash
# Fails if PowerfulTop shows a content window at launch (the "flash").
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="$ROOT/build/PowerfulTop.app"
BIN="$APP/Contents/MacOS/PowerfulTop"

cd "$ROOT"
bash build.sh >/tmp/powerfultop-smoke-build.log 2>&1

killall PowerfulTop >/dev/null 2>&1 || true
sleep 0.2

set +e
"$BIN" --smoke-launch >/tmp/powerfultop-smoke-launch.log 2>&1
status=$?
set -e

cat /tmp/powerfultop-smoke-launch.log >&2 || true
exit "$status"
