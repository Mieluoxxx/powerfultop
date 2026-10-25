#!/bin/bash
# Builds the root fan helper into resources/, where `mygo build` bundles it under
# Contents/Resources/bin. Run it before `go tool mygo build`.
set -euo pipefail

GUI_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$GUI_DIR/resources/darwin-arm64/bin/PowerfulTopFanHelper"

mkdir -p "$(dirname "$OUT")"
cd "$GUI_DIR"
# Static Go binary: the helper runs as root and must not load anything from the user's environment.
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/fanhelper
echo "Built $OUT"
