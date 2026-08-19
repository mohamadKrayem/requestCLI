#!/usr/bin/env bash
#
# End-to-end check of the container image.
#
# scripts/smoke.sh is the exhaustive suite and runs against a locally built
# binary; it stays that way because it needs local files for upload items and a
# closed local port for the transport-failure checks. This script asserts the
# far smaller set of things that can only break in the image: that the binary
# runs as a non-root user, that the CA trust store is present, that stdin still
# reaches the process, and that the version was stamped in at link time.
#
#   scripts/docker-smoke.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose)
PASS=0
FAIL=0

cleanup() { "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

check() {
  local name="$1" expect="$2"; shift 2
  local out
  if out="$("$@" 2>&1)" && [[ "$out" == *"$expect"* ]]; then
    printf '  \033[32mPASS\033[0m %s\n' "$name"
    PASS=$((PASS + 1))
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$name"
    printf '        expected to contain: %s\n' "$expect"
    printf '        got: %s\n' "$(printf '%s' "$out" | head -3)"
    FAIL=$((FAIL + 1))
  fi
}

echo "== Building images =="
VERSION="${VERSION:-dev}" "${COMPOSE[@]}" build >/dev/null
VERSION="${VERSION:-dev}" "${COMPOSE[@]}" up -d --wait echoserver

RQ=("${COMPOSE[@]}" run --rm -T rq)

echo
echo "== Container image =="
check "version was stamped at link time" "${VERSION:-dev}" \
  "${RQ[@]}" --version
check "runs as a non-root user" "uid=10001" \
  "${COMPOSE[@]}" run --rm -T --entrypoint id rq
check "ca-certificates present for default TLS verification" "ca-certificates.crt" \
  "${COMPOSE[@]}" run --rm -T --entrypoint ls rq /etc/ssl/certs/
check "requestCLI alias still resolves" "deprecated" \
  "${COMPOSE[@]}" run --rm -T --entrypoint requestCLI rq get echoserver:8080/ --http -S

echo
echo "== Requests against the fixture server =="
check "plain request"        '"method": "GET"'   "${RQ[@]}" get echoserver:8080/ --http -B
check "json body renders"    '"name": "Mohamad"' "${RQ[@]}" get echoserver:8080/json --http -B
check "request item body"    'ada'               "${RQ[@]}" post echoserver:8080/ --http -B name=ada
check "piped stdin body"     'piped'             sh -c \
  "printf '{\"piped\":true}' | ${COMPOSE[*]} run --rm -T rq post echoserver:8080/ --http -B"

echo
echo "================================"
printf 'passed: %d   failed: %d\n' "$PASS" "$FAIL"
echo "================================"
[[ $FAIL -eq 0 ]]
