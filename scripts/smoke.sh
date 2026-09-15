#!/usr/bin/env bash
#
# End-to-end smoke test for rq.
#
# Builds the binary, starts the local echoserver fixture, runs every scenario
# from TESTING.md and asserts on the output. Exits non-zero if anything fails.
#
#   ./scripts/smoke.sh
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
BIN="$WORK/rq"
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
go build -o "$BIN" "$ROOT/cmd/rq" || exit 1

echo "==> Starting echoserver"
# Refuse to run against a fixture we did not start. A stale server left over
# from a previous run answers on the same ports with an older build, which
# produces confidently wrong results rather than an obvious failure.
#
# The probe is wrapped in `timeout` because a closed port does not always
# refuse the connection: WSL2 and hardened firewalls drop the SYN instead, so
# an unguarded /dev/tcp probe blocks for the kernel's full connect timeout —
# minutes before the suite even starts. A timeout exit means nothing answered,
# which is exactly the free-port case.
for port in 8080 8443; do
  if timeout 1 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null; then
    echo "port $port is already in use; stop the process holding it first" >&2
    exit 1
  fi
done

# Build the fixture and run the binary directly rather than `go run`: go run
# execs the compiled program as a *child*, so killing the pid we captured left
# the real server holding the ports after the script exited. That is what the
# port check above kept tripping over.
go build -o "$WORK/echoserver" "$ROOT/scripts/echoserver" || exit 1
"$WORK/echoserver" >"$WORK/server.log" 2>&1 &
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
check "timeout is enforced" "timed out after 1s" \
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
echo "== Piped stdin body =="
check "piped body reaches the server" 'name\":\"Mohamad' \
  bash -c "printf '{\"name\":\"Mohamad\"}' | '$BIN' post '$HTTP/' -B"
check "--ignore-stdin suppresses the pipe" '"body": ""' \
  bash -c "printf '{\"name\":\"Mohamad\"}' | '$BIN' post '$HTTP/' -B --ignore-stdin"

echo
echo "== -v/--verbose =="
check "-v shows the request line" "GET / HTTP/1.1"  "$BIN" get "$HTTP/" -v -S
check "-v shows a request header" "Accept:"          "$BIN" get "$HTTP/" -v -S

echo
echo "== .http files =="
HTTPFILE="$WORK/api.http"
cat > "$HTTPFILE" <<HTTPDOC
@base = http://$HTTP
@who = body-value-ada
@token = header-value-xyz

### Create a user
POST {{base}}/
Content-Type: application/json
X-Token: {{token}}

{"name":"{{who}}"}

### Health check
GET {{base}}/json
HTTPDOC

check "runs every request in the file" '"name": "Mohamad"' "$BIN" run "$HTTPFILE" --http -B
check "resolves a file variable in the url" '"path": "/"' "$BIN" run "$HTTPFILE" --name "Create a user" --http -B
check "resolves a variable in a header" "header-value-xyz" \
  "$BIN" run "$HTTPFILE" --name "Create a user" --http -B
check "resolves a variable in the body" "body-value-ada" \
  "$BIN" run "$HTTPFILE" --name "Create a user" --http -B
check "--var overrides a file variable" "grace-override" \
  "$BIN" run "$HTTPFILE" --name "Create a user" --var who=grace-override --http -B

# Headings only exist to tell several responses apart, and they go to stderr so
# a piped run carries bodies and nothing else.
check_not "headings stay off stdout" "###" \
  sh -c "$BIN run '$HTTPFILE' --http -B 2>/dev/null"

# A typo in --name should be a one-step fix, not a hunt.
check "an unknown --name lists what is available" "Health check" \
  "$BIN" run "$HTTPFILE" --name nope --http -S

# A body kept in its own file, which is how anyone holds a large payload
# outside the request document.
printf '{"marker":"body-from-file"}' > "$WORK/payload.json"
printf 'POST http://%s/\nContent-Type: application/json\n\n< ./payload.json\n' "$HTTP" > "$WORK/bodyfile.http"
check "reads a body from < ./file" "body-from-file" "$BIN" run "$WORK/bodyfile.http" --http -B

# Errors must say where. file:line: is what an editor turns into a jump.
printf 'GET http://%s/\nbad header line\n' "$HTTP" > "$WORK/broken.http"
check "a parse error reports file:line:" "broken.http:2:" "$BIN" run "$WORK/broken.http" --http -S
printf 'GET http://%s/{{missing}}\n' "$HTTP" > "$WORK/unknown.http"
check "an unknown variable is named" "unknown variable {{missing}}" \
  "$BIN" run "$WORK/unknown.http" --http -S

# A sequence stops at the first failure rather than cascading.
printf '### first\nGET http://%s/status/500\n\n### second\nGET http://%s/json\n' "$HTTP" "$HTTP" > "$WORK/failing.http"
check_exit "--check-status stops at the first failure" 5 \
  "$BIN" run "$WORK/failing.http" --http -S --check-status
check_not "and does not run what follows" "Mohamad" \
  "$BIN" run "$WORK/failing.http" --http -B --check-status

echo
echo "== Streaming =="
# text/event-stream is streamed without asking.
check "sse is auto-detected" "delta" "$BIN" get "$HTTP/sse?events=2" --http -B
check "sse event data is rendered" '"index":0' "$BIN" get "$HTTP/sse?events=2" --http -B
check "sse summary counts events" "2 events" "$BIN" get "$HTTP/sse?events=2" --http -B

# The summary goes to stderr so a pipe carries only events.
check_not "sse summary stays off stdout" "events ·" \
  sh -c "$BIN get '$HTTP/sse?events=2' --http -B 2>/dev/null"

# A keepalive comment is framing, not an event: it must not print a blank line
# and must not be counted.
check "sse keepalives are not counted" "2 events" \
  "$BIN" get "$HTTP/sse?events=2&keepalive=1" --http -B
check_not "sse keepalives are not rendered" ": keepalive" \
  "$BIN" get "$HTTP/sse?events=2&keepalive=1" --http -B

# --raw exists to show framing, so it must show what the parsed view hides.
check "sse --raw shows keepalive frames" ": keepalive" \
  "$BIN" get "$HTTP/sse?events=2&keepalive=1" --http -B --raw
check "sse --raw shows the data field" "data: " \
  "$BIN" get "$HTTP/sse?events=1" --http -B --raw
check_not "sse --raw drops the parsed marker" "●" \
  "$BIN" get "$HTTP/sse?events=1" --http -B --raw

# A stream cut off without its final blank line still shows the last frame.
check "sse shows an unterminated final frame" '"index":1' \
  "$BIN" get "$HTTP/sse?events=2&noterm=1" --http -B

# A gzip-encoded stream must decode. The incremental half of that claim is
# pinned by a unit test that holds the stream open; here we check the bytes
# come out right end to end.
check "sse decodes a gzipped stream" '"index":1' \
  "$BIN" get "$HTTP/sse?events=2&gzip=1" --http -B
check "sse gzipped stream is announced as such" "gzip" \
  "$BIN" get "$HTTP/sse?events=1&gzip=1" --http -H

# A frame with no event: field is "message" per the spec.
check "sse names an unnamed frame message" "message" \
  "$BIN" get "$HTTP/sse?events=1&name=" --http -B
check "sse passes a custom event name through" "content_block_delta" \
  "$BIN" get "$HTTP/sse?events=1&name=content_block_delta" --http -B

# --stream forces incremental rendering for a server that does not advertise
# the content type.
check "--stream forces streaming on any content type" "message" \
  "$BIN" get "$HTTP/text" --http -B --stream

# -v adds each event's offset from the start of the request.
#
# Two events with a delay, not one: a single event can land in under a
# millisecond, and formatDuration then renders it as microseconds. Asserting on
# "ms" against one event is a bet on the machine being slow, which CI lost. The
# second event here cannot arrive before the delay has passed.
check "-v adds per-event timing" "ms" \
  "$BIN" get "$HTTP/sse?events=2&delay=60ms" --http -B -v

# The status line still comes from the same renderer as a buffered response.
check "sse still prints a status line" "200 OK" "$BIN" get "$HTTP/sse?events=1" --http -S

# The regression this whole change exists to prevent: --timeout must bound the
# headers, not the life of the stream. A 1s timeout across a ~1.2s stream used
# to kill it at 1s.
check "--timeout does not kill a longer stream" "3 events" \
  "$BIN" get "$HTTP/sse?events=3&delay=400ms" --http -B --timeout 1s

echo
echo "== Error handling =="
check "no URL"            "requires at least 1 arg(s)"    "$BIN" get
# A second bare URL is no longer an arity error: extra args are request items,
# so it is rejected by the item grammar instead.
check "stray second URL"  "not a request item: \"b.com\"" "$BIN" get a.com b.com
check "unknown command"   "unknown command"              "$BIN" fetch example.com
# Some environments (WSL2, hardened firewalls) drop the SYN instead of refusing it,
# so the error text is not portable; assert only that the transport failure is
# reported with the request context. --timeout keeps a dropped SYN from costing 30s.
check "transport failure" "sending GET http://127.0.0.1:1" "$BIN" get 127.0.0.1:1 --http --timeout 2s
check "malformed url"     "no host"                      "$BIN" get "://nope"
check "malformed json"    "parsing json"                 "$BIN" post "$HTTP/" --multi -b '{bad'

echo
echo "== Request items: headers and query =="
check "header item"              '"X-Token": "abc"'   "$BIN" get "$HTTP/" -B "X-Token:abc"
check "header item overrides -n" '"X-Token": "item"'  "$BIN" get "$HTTP/" -B -n X-Token=flag "X-Token:item"
check "query item"               '"a": "1"'           "$BIN" get "$HTTP/" -B "a==1"
check "query item overrides -q"  '"a": "item"'        "$BIN" get "$HTTP/" -B -q a=flag "a==item"
check "other -q keys survive"    '"keep": "yes"'      "$BIN" get "$HTTP/" -B -q keep=yes "a==item"
check "repeated query items"     '"tag": "one,two"'   "$BIN" get "$HTTP/" -B "tag==one" "tag==two"
# A trailing colon removes a header the tool would otherwise send by default.
check_not "header item unsets a default" "User-Agent" "$BIN" get "$HTTP/" -B "User-Agent:"
check_not "unset is case-insensitive"    "User-Agent" "$BIN" get "$HTTP/" -B "user-agent:"
# Setting after unsetting must win: last thing written is what happens.
check "set after unset wins"     '"X-Token": "a"'     "$BIN" get "$HTTP/" -B "X-Token:" "X-Token:a"
# The URL is never item-parsed, so its own '=' and ':' are safe.
check "url query is not an item" '"b": "c"'           "$BIN" get "$HTTP/?b=c" -B
check "value may contain @"      '"email": "a@b.com"' "$BIN" get "$HTTP/" -B "email==a@b.com"

echo
echo "== Request items: body =="
# Each of the four body encodings is echoed back by the fixture, so a body
# built with the wrong bytes fails loudly here instead of merely "looking ok".
check "json item body (no flags)"        'name\":\"Mohamad' \
  "$BIN" post "$HTTP/" -B "name=Mohamad"
check "large integer survives via item"  "1234567890123456789" \
  "$BIN" post "$HTTP/" -B "id:=1234567890123456789"
# The query path must not mangle what the body path preserves: routing a raw
# value through map[string]any rounded it to ...800 on GET while POST was exact.
check "large integer survives on a query verb" "1234567890123456789" \
  "$BIN" get "$HTTP/" -B "id:=1234567890123456789"
check "raw bool on a query verb"         '"active": "true"' \
  "$BIN" get "$HTTP/" -B "active:=true"
check "raw field written verbatim"       'tags\":[1,2]' \
  "$BIN" post "$HTTP/" -B "tags:=[1,2]"
check "duplicate item keys: last wins"   'a\":\"2' \
  "$BIN" post "$HTTP/" -B "a=1" "a=2"
check_not "duplicate item keys: earlier value dropped" 'a\":\"1' \
  "$BIN" post "$HTTP/" -B "a=1" "a=2"
check "form item body with -f (header)"  "x-www-form-urlencoded" \
  "$BIN" post "$HTTP/" -f -B "name=Mohamad"
check "form item body with -f (fields)"  "name=Mohamad" \
  "$BIN" post "$HTTP/" -f -B "name=Mohamad"
check "key@file implies multipart"       "letter.txt" \
  "$BIN" post "$HTTP/multipart" -B "avatar@$WORK/fixtures/letter.txt" "name=Mohamad"
check "implied multipart keeps fields"   '"name": "Mohamad"' \
  "$BIN" post "$HTTP/multipart" -B "avatar@$WORK/fixtures/letter.txt" "name=Mohamad"
check "field item on GET becomes query"      '"name": "Mohamad"' \
  "$BIN" get "$HTTP/" -B "name=Mohamad"
check "raw array item on GET becomes query"  '"tags": "[1,2]"' \
  "$BIN" get "$HTTP/" -B "tags:=[1,2]"
check "items and -b/--body conflict" \
  "request items and --Nbody/--body both provide a body; use one or the other" \
  "$BIN" post "$HTTP/" "name=Mohamad" -b '{"x":1}'
check "file upload with -f is an error" \
  "a file upload cannot be sent as a url-encoded form; drop -f" \
  "$BIN" post "$HTTP/" -f "avatar@$WORK/fixtures/letter.txt"
check "file upload on GET is an error" \
  "GET cannot send a file upload; use POST, PUT or PATCH" \
  "$BIN" get "$HTTP/" "avatar@$WORK/fixtures/letter.txt"

echo
echo "== Output fidelity =="
# The display path must never decode the payload: doing so silently corrupted
# large integers, key order, newlines inside strings, and duplicate keys.
check "large integers survive"     "1234567890123456789" "$BIN" get "$HTTP/fidelity" -B
check "key order preserved"        "zebra"               "$BIN" get "$HTTP/fidelity" -B
check "newline inside string kept" 'line1\nline2'        "$BIN" get "$HTTP/fidelity" -B
check "duplicate keys shown"       '"dup"'               "$BIN" get "$HTTP/fidelity" -B

# Piped output must never carry ANSI escapes. $'\033[' is an escape sequence.
check_not "piped json has no ansi" $'\033[' "$BIN" get "$HTTP/json" -B
check_not "piped html has no ansi" $'\033[' "$BIN" get "$HTTP/html" -B
check_not "piped xml has no ansi"  $'\033[' "$BIN" get "$HTTP/xml" -B
check_not "NO_COLOR strips colour" $'\033[' env NO_COLOR=1 "$BIN" get "$HTTP/json" -B

check "xml is served"    "highlighted" "$BIN" get "$HTTP/xml" -B
check "yaml is served"   "highlighted" "$BIN" get "$HTTP/yaml" -B

# A binary body is described, not dumped.
check     "binary body described"  "binary data" "$BIN" get "$HTTP/binary" -B
check     "binary names its type"  "image/png"   "$BIN" get "$HTTP/binary" -B
check_not "binary bytes not dumped" "PNG"        "$BIN" get "$HTTP/binary" -B

echo
echo "== Exit codes =="
"$BIN" get "$HTTP/text" -S >/dev/null 2>&1
if [[ $? -eq 0 ]]; then
  printf '  \033[32mPASS\033[0m success exits 0\n'; PASS=$((PASS + 1))
else
  printf '  \033[31mFAIL\033[0m success exits 0\n'; FAIL=$((FAIL + 1))
fi
"$BIN" get 127.0.0.1:1 --http --timeout 2s >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  printf '  \033[32mPASS\033[0m failure exits non-zero\n'; PASS=$((PASS + 1))
else
  printf '  \033[31mFAIL\033[0m failure exits non-zero\n'; FAIL=$((FAIL + 1))
fi

# check_exit <name> <expected-code> <command...>
check_exit() {
  local name="$1" want="$2"; shift 2
  "$@" >/dev/null 2>&1
  local got=$?
  if [[ $got -eq $want ]]; then
    printf '  \033[32mPASS\033[0m %s\n' "$name"; PASS=$((PASS + 1))
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$name"
    printf '        expected exit %d, got %d\n' "$want" "$got"
    FAIL=$((FAIL + 1))
  fi
}

# Without --check-status, an HTTP error status still exits 0.
check_exit "no --check-status: 4xx still exits 0" 0 "$BIN" get "$HTTP/status/404"

check_exit "--check-status: success exits 0"        0 "$BIN" get "$HTTP/" --check-status
check_exit "--check-status: transport failure exits 2" 2 "$BIN" get 127.0.0.1:1 --http --timeout 2s --check-status
check_exit "--check-status: unfollowed 3xx exits 3" 3 "$BIN" get "$HTTP/redirect" --check-status
check_exit "--check-status: 4xx exits 4"            4 "$BIN" get "$HTTP/status/404" --check-status
check_exit "--check-status: 5xx exits 5"            5 "$BIN" get "$HTTP/status/500" --check-status

echo
echo "================================"
printf 'passed: %d   failed: %d\n' "$PASS" "$FAIL"
echo "================================"
[[ $FAIL -eq 0 ]]
