#!/usr/bin/env bash
set -euo pipefail

AWS_BIN=${AWS_BIN:-aws}
ALLOW_ROOT_BOOTSTRAP=${PHASE3_ALLOW_ROOT_BOOTSTRAP:-false}

die() {
  printf '[phase3-aws-identity] error: %s\n' "$*" >&2
  exit 1
}

command -v "$AWS_BIN" >/dev/null 2>&1 || die "missing aws CLI"

arn=$("$AWS_BIN" sts get-caller-identity --query Arn --output text)
case "$arn" in
  arn:aws:iam::*:root)
    if [ "$ALLOW_ROOT_BOOTSTRAP" = "true" ]; then
      printf '[phase3-aws-identity] root identity allowed only for explicit bootstrap: %s\n' "$arn"
      exit 0
    fi
    die "refusing to use root identity for deployment: $arn"
    ;;
esac

printf '[phase3-aws-identity] using non-root identity: %s\n' "$arn"
