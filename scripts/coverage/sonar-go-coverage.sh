#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_COVERAGE_MIN="${BACKEND_COVERAGE_MIN:-50}"
PROFILE_TMP="$(mktemp "${TMPDIR:-/tmp}/cets-go-coverage.XXXXXX")"
trap 'rm -f "$PROFILE_TMP"' EXIT

cd "$ROOT_DIR/services/api"
go test -count=1 ./... -covermode=atomic -coverprofile="$PROFILE_TMP"
cp "$PROFILE_TMP" coverage.out
sed 's#^event-ticket-system/#services/api/#' "$PROFILE_TMP" > coverage.sonar.out

coverage="$(go tool cover -func="$PROFILE_TMP" | awk '/^total:/ {gsub("%", "", $3); print $3}')"
awk -v coverage="$coverage" -v minimum="$BACKEND_COVERAGE_MIN" 'BEGIN {
	if ((coverage + 0) < (minimum + 0)) {
		printf "backend coverage %.2f%% is below required %.2f%%\n", coverage, minimum > "/dev/stderr"
		print "set TEST_DATABASE_URL, and REDIS_URL when Redis-backed reservation coverage is expected" > "/dev/stderr"
		exit 1
	}
	printf "backend coverage %.2f%% meets required %.2f%%\n", coverage, minimum
}'
