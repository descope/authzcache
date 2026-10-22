#!/usr/bin/env bash
# Builds authzcache:loadtest — the fake-backend image used by the k6 stress test.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
out=build/loadtest/app

GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-arm64}" \
  go build -tags loadtest -ldflags="-w -s" -o "$out" ./cmd/authzcache

docker build -t authzcache:loadtest build/loadtest
rm -f "$out"
echo "built authzcache:loadtest"
