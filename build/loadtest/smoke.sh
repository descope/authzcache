#!/usr/bin/env bash
# Proves both paths: a fresh tuple is a 10-50ms backend miss, the same tuple again is a fast hit.
set -euo pipefail

NET="${NET:-authzcache-loadtest_default}"
URL=http://authzcache:8189/v1/mgmt/fga/check
AUTH="Authorization: Bearer P2loadtest0000000000000000000:loadtest-key"
BODY='{"tuples":[{"resource":"doc-smoke-'"$RANDOM$RANDOM"'","resourceType":"doc","relation":"owner","target":"user-smoke","targetType":"user"}]}'

docker run --rm --network "$NET" curlimages/curl:latest -s \
  -H "$AUTH" -H 'Content-Type: application/json' -d "$BODY" \
  -w '\nMISS http=%{http_code} time=%{time_total}s\n' "$URL"

docker run --rm --network "$NET" curlimages/curl:latest -s \
  -H "$AUTH" -H 'Content-Type: application/json' -d "$BODY" \
  -w '\nHIT  http=%{http_code} time=%{time_total}s\n' "$URL"
