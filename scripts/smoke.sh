#!/usr/bin/env bash
#
# End-to-end smoke test for requestCLI.
#
# Builds the binary, starts the local echoserver fixture, runs every scenario
# from TESTING.md and asserts on the output. Exits non-zero if anything fails.
#
#   ./scripts/smoke.sh
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
BIN="$WORK/requestCLI"
HTTP="localhost:8080"
HTTPS="https://localhost:8443"

PASS=0
FAIL=0

cleanup() {
  [[ -n "${SERVER_PID:-}" ]] && kill "$SERVER_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

# check <name> <expected-substring> <command...>
check() {
  local name="$1" expect="$2"; shift 2
  local out
  out="$("$@" 2>&1)"
  if [[ "$out" == *"$expect"* ]]; then
    printf '  \033[32mPASS\033[0m %s\n' "$name"
    PASS=$((PASS + 1))
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$name"
    printf '        expected to contain: %s\n' "$expect"
    printf '        got: %s\n' "$(echo "$out" | head -3)"
    FAIL=$((FAIL + 1))
  fi
}

# check_not <name> <unexpected-substring> <command...>
check_not() {
  local name="$1" reject="$2"; shift 2
  local out
  out="$("$@" 2>&1)"
  if [[ "$out" != *"$reject"* ]]; then
    printf '  \033[32mPASS\033[0m %s\n' "$name"
    PASS=$((PASS + 1))
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$name"
    printf '        should NOT have contained: %s\n' "$reject"
    FAIL=$((FAIL + 1))
  fi
}

echo "==> Building"
go build -o "$BIN" "$ROOT" || exit 1

echo "==> Starting echoserver"
go run "$ROOT/scripts/echoserver" >"$WORK/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 30); do
  grep -q "https listening" "$WORK/server.log" 2>/dev/null && break
  sleep 0.5
done
if ! grep -q "https listening" "$WORK/server.log" 2>/dev/null; then
  echo "server failed to start:"; cat "$WORK/server.log"; exit 1
fi

mkdir -p "$WORK/fixtures"
echo "hello from a text file" >"$WORK/fixtures/letter.txt"

echo
echo "== HTTP methods =="
for m in get:GET post:POST put:PUT patch:PATCH del:DELETE options:OPTIONS trace:TRACE; do
  check "${m%%:*} sends ${m##*:}" "\"method\": \"${m##*:}\"" \
    "$BIN" "${m%%:*}" "$HTTP/" -B
done
check "delete alias works" '"method": "DELETE"' "$BIN" delete "$HTTP/" -B
check "conn alias works" "200 OK" "$BIN" conn "$HTTP/" -S

echo
echo "== Output selection =="
check     "status only shows status"   "200 OK"       "$BIN" get "$HTTP/json" -S
check_not "status only hides body"     '"name"'       "$BIN" get "$HTTP/json" -S
check     "headers only shows headers" "Content-Type" "$BIN" get "$HTTP/json" -H
check     "body only shows body"       '"name"'       "$BIN" get "$HTTP/json" -B
check_not "body only hides status"     "HTTP/1.1"     "$BIN" get "$HTTP/json" -B
check     "-H -B combine (headers)"    "Content-Type" "$BIN" get "$HTTP/text" -H -B
check     "-H -B combine (body)"       "plain text"   "$BIN" get "$HTTP/text" -H -B

echo
echo "== Request data =="
check "query params"            '"a": "1"'                   "$BIN" get "$HTTP/" -q a=1,b=two -B
check "custom headers"          '"X-Api-Token": "123"'       "$BIN" get "$HTTP/" -n X-API-Token=123 -B
check "cookies"                 '"session": "abc"'           "$BIN" get "$HTTP/" -c session=abc -B
check "basic auth"              '"authenticated": "yes"'     "$BIN" get "$HTTP/basic-auth" -a username=Mohamad,password=pass123 -B
check "json body on POST"       'name\":\"Mohamad'           "$BIN" post "$HTTP/" -b '{"name":"Mohamad"}' -B
check "json body on GET -> query" '"a": "1"'                 "$BIN" get "$HTTP/" -b '{"a":1}' -B
check "non-json body -> text"   '"Content-Type": "text/plain"' "$BIN" get "$HTTP/" -b 'hello' -B
check "url-encoded form"        'x-www-form-urlencoded'      "$BIN" post "$HTTP/" -f -b '{"k":"v"}' -B
check "form keeps -n headers"   '"X-Api-Token": "123"'       "$BIN" post "$HTTP/" -f -n X-API-Token=123 -b '{"k":"v"}' -B

echo
echo "== Multipart =="
check "multipart with a file" "letter.txt" \
  "$BIN" post "$HTTP/multipart" --multi -b "{\"@!letter\":\"$WORK/fixtures/letter.txt\",\"name\":\"Mohamad\"}" -B
check "multipart without files" '"name": "Mohamad"' \
  "$BIN" post "$HTTP/multipart" --multi -b '{"name":"Mohamad"}' -B
check "single-char field name" '"a": "1"' \
  "$BIN" post "$HTTP/multipart" --multi -b '{"a":1}' -B
check "missing file errors cleanly" "no such file" \
  "$BIN" post "$HTTP/multipart" --multi -b '{"@!x":"/does/not/exist"}'

echo
echo "== Redirects, compression, status =="
check "redirect not followed by default" "302"        "$BIN" get "$HTTP/redirect" -S
check "redirect followed with --redirect" "200 OK"    "$BIN" get "$HTTP/redirect" --redirect -S
check "gzip decoded"    "decoded correctly"           "$BIN" get "$HTTP/gzip" -B
check "deflate decoded" "decoded correctly"           "$BIN" get "$HTTP/deflate" -B
check "404 reported"    "404 Not Found"               "$BIN" get "$HTTP/status/404" -S
check "500 reported"    "500 Internal Server Error"   "$BIN" get "$HTTP/status/500" -S

echo
echo "== TLS =="
check "self-signed rejected by default" "certificate" "$BIN" get "$HTTPS/json" -S
check "self-signed accepted with -k"    '"name"'      "$BIN" get "$HTTPS/json" -B -k
check "request really used TLS"         '"tls": true' "$BIN" get "$HTTPS/" -B -k

echo
echo "== Timeouts =="
check "timeout is enforced" "Client.Timeout" \
  "$BIN" get "$HTTP/slow?seconds=5" --timeout 1s -S

echo
echo "== Multi-line stdin =="
check "multi-line body"      'a\":1'         bash -c "printf '{\n\n\"a\":1\n};\n' | '$BIN' post '$HTTP/' --body -B"
check "top-level array body" '[1,2,3]'       bash -c "printf '[\n1,\n2,\n3\n];\n' | '$BIN' post '$HTTP/' --body -B"
check "headers+body together (header)" '"X-Api-Token": "123"' \
  bash -c "printf '{\n\"X-API-Token\":\"123\"\n};\n{\n\"a\":1\n};\n' | '$BIN' put '$HTTP/' --headers --body -B"
check "headers+body together (body)" 'a\":1' \
  bash -c "printf '{\n\"X-API-Token\":\"123\"\n};\n{\n\"a\":1\n};\n' | '$BIN' put '$HTTP/' --headers --body -B"

echo
echo "== Error handling =="
check "no URL"            "accepts 1 arg(s), received 0" "$BIN" get
check "too many URLs"     "accepts 1 arg(s), received 2" "$BIN" get a.com b.com
check "unknown command"   "unknown command"              "$BIN" fetch example.com
check "connection refused" "connection refused"          "$BIN" get 127.0.0.1:1 --http
check "malformed url"     "no host"                      "$BIN" get "://nope"
check "malformed json"    "parsing json"                 "$BIN" post "$HTTP/" --multi -b '{bad'

echo
echo "== Exit codes =="
"$BIN" get "$HTTP/text" -S >/dev/null 2>&1
if [[ $? -eq 0 ]]; then
  printf '  \033[32mPASS\033[0m success exits 0\n'; PASS=$((PASS + 1))
else
  printf '  \033[31mFAIL\033[0m success exits 0\n'; FAIL=$((FAIL + 1))
fi
"$BIN" get 127.0.0.1:1 --http >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  printf '  \033[32mPASS\033[0m failure exits non-zero\n'; PASS=$((PASS + 1))
else
  printf '  \033[31mFAIL\033[0m failure exits non-zero\n'; FAIL=$((FAIL + 1))
fi

echo
echo "================================"
printf 'passed: %d   failed: %d\n' "$PASS" "$FAIL"
echo "================================"
[[ $FAIL -eq 0 ]]
