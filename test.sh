#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR/gui"

test -z "$(gofmt -l .)" || { gofmt -l . >&2; echo "gofmt: files above need formatting" >&2; exit 1; }
go vet ./...
go tool mygo i18n check
go test ./...
