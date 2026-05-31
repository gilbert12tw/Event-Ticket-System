#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "$ROOT_DIR/services/api"
go test -count=1 ./... -covermode=atomic -coverprofile=coverage.out
sed 's#^event-ticket-system/#services/api/#' coverage.out > coverage.sonar.out
