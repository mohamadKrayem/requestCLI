# Testing rq

There are three ways to test this project, from cheapest to most thorough:

| Layer            | Command              | What it covers                                  |
| ---------------- | --------------------- | ----------------------------------------------- |
| Unit/integration | `make test`          | Every package, no network needed                |
| End-to-end smoke | `make smoke`         | The real binary against a local server          |
| Manual scenarios | see below            | Exploring behaviour by hand                     |

Every command and expected output in this document was run against the local
fixture server and reflects real output.

---

## 1. Automated tests

```shell
$ make test        # go test -race ./...
$ make cover       # coverage summary
```

Tests use `httptest`, so nothing leaves your machine. Each bug found in the code
review has a regression test marked with a `Regression:` comment — search for
that string to see what a given test is protecting against.

Run a single package or test:

```shell
$ go test ./core/ -run TestGenerateURL -v
$ go test ./command/ -run TestScanRequest -v
$ go test ./reqitem/ -run TestParse -v
```

## 2. End-to-end smoke test

```shell
$ make smoke
```

This builds `rq`, starts the fixture server, runs 100+ assertions covering
every flag, subcommand, request-item form and exit code, and prints a pass/fail
summary. It cleans up after itself. Use this as the "did I break anything"
check before committing.

## 3. Manual scenarios

Start the fixture server in one terminal:

```shell
$ make server
# http  listening on http://127.0.0.1:8080
# https listening on https://127.0.0.1:8443 (self-signed)
```

Build the CLI in another:

```shell
$ make build
```

`make build` produces `rq`, which the scenarios below use.

The catch-all endpoint echoes your request back as JSON, so you can see exactly
what was sent. Available endpoints:

| Endpoint         | Purpose                                     |
| ---------------- | ------------------------------------------- |
| `/`              | Echoes method, query, headers, cookies, body |
| `/json`          | A nested JSON document                      |
| `/html`          | An HTML document                            |
| `/text`          | A plain-text document                       |
| `/gzip`          | gzip-encoded JSON                           |
| `/deflate`       | deflate-encoded JSON                        |
| `/redirect`      | 302 to `/moved`                             |
| `/slow?seconds=5`| Sleeps before responding                    |
| `/sse`           | Streams server-sent events. See Scenario U for its query parameters |
| `/status/<code>` | Returns that status code                    |
| `/basic-auth`    | Requires Basic Auth                         |
| `/multipart`     | Parses a multipart form and reports it      |
| `/fidelity`      | JSON with a 64-bit integer, unsorted keys, a newline inside a string, and a duplicate key |
| `/binary`        | A PNG header followed by NUL bytes          |
| `/xml`           | An XML document                             |
| `/yaml`          | A YAML document                             |

---

### Scenario A — HTTP methods

Confirm each subcommand sends its own verb. The echo endpoint reports it back.

```shell
$ ./rq get     localhost:8080/ -B | grep method     # "method": "GET"
$ ./rq post    localhost:8080/ -B | grep method     # "method": "POST"
$ ./rq put     localhost:8080/ -B | grep method     # "method": "PUT"
$ ./rq patch   localhost:8080/ -B | grep method     # "method": "PATCH"
$ ./rq del     localhost:8080/ -B | grep method     # "method": "DELETE"
$ ./rq options localhost:8080/ -B | grep method     # "method": "OPTIONS"
$ ./rq trace   localhost:8080/ -B | grep method     # "method": "TRACE"
```

Aliases:

```shell
$ ./rq delete localhost:8080/ -B | grep method      # "method": "DELETE"
$ ./rq conn   localhost:8080/ -S                    # 200 OK
```

> **Note**: `head` returns no body by design — HEAD responses have none. Use
> `-H` to see its headers.

### Scenario B — Output selection

```shell
$ ./rq get localhost:8080/json -S      # status line only
$ ./rq get localhost:8080/json -H      # headers only
$ ./rq get localhost:8080/json -B      # body only
$ ./rq get localhost:8080/json -H -B   # headers and body, no status
$ ./rq get localhost:8080/json         # everything
```

Expected for `-S`:

```
HTTP/1.1 200 OK
```

The three flags combine. Passing none of them prints everything.

### Scenario C — URL scheme and TLS

Scheme-less URLs default to https, except loopback hosts:

```shell
$ ./rq get example.com -S            # -> https://example.com
$ ./rq get localhost:8080/text -S    # -> http://localhost:8080
$ ./rq get example.com --http -S     # -> http://example.com
```

Certificate verification is on by default:

```shell
$ ./rq get https://localhost:8443/json -S
Error: sending GET https://localhost:8443/json: Get "https://localhost:8443/json":
tls: failed to verify certificate: x509: certificate signed by unknown authority

$ ./rq get https://localhost:8443/json -B -k     # succeeds
$ ./rq get https://localhost:8443/ -B -k | grep tls
  "tls": true
```

That first failure is the correct result — the fixture uses a self-signed
certificate. If it ever *succeeds* without `-k`, verification has regressed.

### Scenario D — Headers

```shell
$ ./rq get localhost:8080/ -n X-API-Token=123 -B | grep X-Api
      "X-Api-Token": "123",

$ ./rq get localhost:8080/ -n A=1 -n B=2 -B          # repeated flag
$ ./rq get localhost:8080/ -n A=1,B=2 -B             # comma-separated
```

Multi-line headers from stdin, terminated by `;`:

```shell
$ ./rq get localhost:8080/ --headers -B
{
	"X-API-Token": "123"
};
```

### Scenario E — Request bodies

```shell
# JSON body on a method that carries one
$ ./rq post localhost:8080/ -b '{"name":"Mohamad"}' -B | grep -A2 body
  "body": {
    "name": "Mohamad"
  },

# JSON body on GET becomes query parameters instead
$ ./rq get localhost:8080/ -b '{"a":1}' -B | grep -A2 query
  "query": {
    "a": "1"
  },

# A body that is not JSON is sent verbatim as text/plain, on any verb
$ ./rq get localhost:8080/ -b 'hello' -B | grep -E 'body|text/plain'
  "body": "hello",
    "Content-Type": "text/plain",
$ ./rq post localhost:8080/ -b 'hello' -B | grep -E 'body|text/plain'
  "body": "hello",
    "Content-Type": "text/plain",

# An explicit Content-Type is never overwritten by that fallback
$ ./rq post localhost:8080/ -b 'hello' -B Content-Type:text/csv | grep -i content-type
    "Content-Type": "text/csv",
```

Multi-line body from stdin:

```shell
$ ./rq post localhost:8080/ --body -B
{
	"name":"Mohamad",
	"arrayOfNbs":[1,2,3],
	"nested":{"w":"2"}
};
```

Top-level arrays work, and blank lines are ignored:

```shell
$ printf '[\n1,\n2,\n3\n];\n' | ./rq post localhost:8080/ --body -B | grep body
  "body": [1, 2, 3],
```

Headers and body together — two documents, one stream:

```shell
$ ./rq put localhost:8080/ --headers --body -B
{
	"X-API-Token":"123"
};
{
	"name":"Mohamad"
};
```

### Scenario F — Forms

```shell
$ ./rq post localhost:8080/ -f -b '{"key1":"value","key2":21}' -B
  "body": "key1=value&key2=21",
    "Content-Type": "application/x-www-form-urlencoded",
```

`-f` must not discard explicit headers:

```shell
$ ./rq post localhost:8080/ -f -n X-API-Token=123 -b '{"k":"v"}' -B | grep X-Api
      "X-Api-Token": "123",
```

### Scenario G — Multipart and file uploads

```shell
$ echo "hello from a text file" > /tmp/letter.txt

$ ./rq post localhost:8080/multipart --multi \
    -b '{"@!letter":"/tmp/letter.txt","name":"Mohamad","age":22}' -B
{
  "fields": {
    "age": "22",
    "name": "Mohamad"
  },
  "files": {
    "letter": "letter.txt (23 bytes)"
  }
}
```

Path forms — `~` expands to your home directory, `/` is absolute, anything else
is relative to the working directory:

```shell
$ ./rq post localhost:8080/multipart --multi -b '{"@!f":"~/notes.txt"}' -B
$ ./rq post localhost:8080/multipart --multi -b '{"@!f":"letter.txt"}' -B
```

A form with no files must still be valid:

```shell
$ ./rq post localhost:8080/multipart --multi -b '{"name":"Mohamad"}' -B
```

A missing file should fail cleanly, not panic:

```shell
$ ./rq post localhost:8080/multipart --multi -b '{"@!x":"/nope.txt"}'
Error: opening /nope.txt: open /nope.txt: no such file or directory
```

### Scenario H — Query parameters, cookies, auth

```shell
$ ./rq get localhost:8080/ -q a=1,b=two -B | grep -A3 query
  "query": {
    "a": "1",
    "b": "two"
  },

$ ./rq get localhost:8080/ -c session=abc,theme=dark -B | grep -A3 cookies
  "cookies": {
    "session": "abc",
    "theme": "dark"
  },

$ ./rq get localhost:8080/basic-auth -a username=Mohamad,password=pass123 -B
{
  "authenticated": "yes",
  "password": "pass123",
  "username": "Mohamad"
}

$ ./rq get localhost:8080/basic-auth -S
HTTP/1.1 401 Unauthorized
```

Combining `-q` with a form body must not produce a malformed URL:

```shell
$ ./rq get localhost:8080/ -q a=1 -f -b '{"b":2}' -B | grep -A3 query
  "query": {
    "a": "1",
    "b": "2"
  },
```

The server should log `uri=/?a=1&b=2` — with `&`, not a second `?`.

### Scenario I — Redirects

```shell
$ ./rq get localhost:8080/redirect -S
HTTP/1.1 302 Found

$ ./rq get localhost:8080/redirect --redirect -S
HTTP/1.1 200 OK
```

### Scenario J — Compression

```shell
$ ./rq get localhost:8080/gzip -B
{
  "encoding": "gzip",
  "message": "decoded correctly"
}

$ ./rq get localhost:8080/deflate -B
```

> **Note**: brotli decoding cannot be exercised by the local fixture — the
> `dsnet/compress` dependency decodes brotli but cannot produce it. Test it
> against a real CDN-backed site, e.g. `./rq get https://www.cloudflare.com -S`.

### Scenario K — Timeouts

```shell
$ time ./rq get "localhost:8080/slow?seconds=5" --timeout 1s -S
Error: sending GET http://localhost:8080/slow?seconds=5: ... (Client.Timeout exceeded ...)
real    0m1.026s
```

It must return in about a second, not five. Without `--timeout` the default is
30s.

### Scenario L — Status codes

```shell
$ ./rq get localhost:8080/status/201 -S    # HTTP/1.1 201 Created
$ ./rq get localhost:8080/status/404 -S    # HTTP/1.1 404 Not Found
$ ./rq get localhost:8080/status/500 -S    # HTTP/1.1 500 Internal Server Error
```

Note that an HTTP error status is not by itself a CLI failure without
`--check-status` — see Scenario Q below.

### Scenario M — Content-type rendering

```shell
$ ./rq get localhost:8080/json -B    # pretty-printed, colorized JSON
$ ./rq get localhost:8080/html -B    # syntax-highlighted HTML
$ ./rq get localhost:8080/xml -B     # syntax-highlighted XML
$ ./rq get localhost:8080/yaml -B    # syntax-highlighted YAML
$ ./rq get localhost:8080/text -B    # plain
$ ./rq get localhost:8080/binary -B  # a notice, not raw bytes
```

Try a different theme:

```shell
$ ./rq get localhost:8080/xml -B --style github
```

Piping should produce clean, uncolored output, for every content type:

```shell
$ ./rq get localhost:8080/json -B | cat
$ ./rq get localhost:8080/html -B | cat
$ NO_COLOR=1 ./rq get localhost:8080/json -B
```

### Scenario M2 — Output fidelity

The body shown must be byte-for-byte what the server sent. `/fidelity` returns
a payload built to break a display path that decodes and re-encodes:

```shell
$ ./rq get localhost:8080/fidelity -B
```

Check all four:

- `id` reads `1234567890123456789` exactly, not `...800`. Anything routed
  through `float64` loses precision past 53 bits.
- `zebra` still comes before `apple`. Keys are never sorted.
- `note` still contains `\n`. Newlines inside string *data* are not stripped.
- Both `dup` keys are shown rather than one silently winning.

### Scenario N — Error handling

None of these should panic; each should print one clear line and exit non-zero.

```shell
$ ./rq put
Error: requires at least 1 arg(s), only received 0

$ ./rq get a.com b.com
Error: not a request item: "b.com" (expected key=value, key:=value, key==value, Key:value, key@file or key=@file)

$ ./rq fetch example.com
Error: unknown command "fetch" for "rq"

$ ./rq get 127.0.0.1:1 --http
Error: sending GET http://127.0.0.1:1: ... connect: connection refused

$ ./rq get '://nope'
Error: invalid URL "https://://nope": no host

$ ./rq post localhost:8080/ --multi -b '{bad'
Error: --multi needs a json object as the body: parsing json object: ...
```

Exit codes:

```shell
$ ./rq get localhost:8080/text -S >/dev/null; echo $?   # 0
$ ./rq get 127.0.0.1:1 --http   >/dev/null 2>&1; echo $?   # 1
```

### Scenario O — Regression checks

These are the defects found in the code review. Each should now behave
correctly rather than crashing.

```shell
# 1. Missing URL on put/head/options/patch (used to panic)
$ ./rq put; ./rq head; ./rq options; ./rq patch

# 2. Blank line inside --body (used to panic)
$ printf '{\n\n"a":1\n};\n' | ./rq post localhost:8080/ --body -S

# 3. Multipart field name shorter than two characters (used to panic)
$ ./rq post localhost:8080/multipart --multi -b '{"a":1}' -B

# 4. Non-JSON body (used to exit via log.Fatal)
$ ./rq get localhost:8080/ -b 'hello' -B

# 5. trace used to send CONNECT
$ ./rq trace localhost:8080/ -B | grep method     # "method": "TRACE"

# 6. Query params used to produce ?a=1?b=2
$ ./rq get localhost:8080/ -q a=1 -f -b '{"b":2}' -B

# 7. -f used to wipe -n headers
$ ./rq post localhost:8080/ -f -n X-API-Token=123 -b '{"k":"v"}' -B

# 8. --headers --body together used to drop the body
$ printf '{\n"X-API-Token":"1"\n};\n{\n"a":1\n};\n' | \
    ./rq put localhost:8080/ --headers --body -B

# 9. Connection errors used to print a pointer pair
$ ./rq get 127.0.0.1:1 --http

# 10. Top-level array bodies used to be corrupted
$ printf '[\n1,\n2\n];\n' | ./rq post localhost:8080/ --body -B
```

### Scenario P — Request items

The primary way to build a request, in place of `-n`/`-q`/`-b`:

```shell
# Header and query items
$ ./rq get localhost:8080/ 'X-Token:abc' -B | grep -i token
    "X-Token": "abc"
$ ./rq get localhost:8080/ 'page==2' 'per_page==10' -B | grep -A3 query
  "query": {
    "page": "2",
    "per_page": "10"
  },

# Unset a default header with a trailing colon
$ ./rq get localhost:8080/ 'User-Agent:' -B | grep -i user-agent   # nothing

# JSON body items (no flags): field, raw field, form/multipart
$ ./rq post localhost:8080/ name=Mohamad age:=22 -B | grep -A3 body
  "body": {
    "name": "Mohamad",
    "age": 22
  },
$ ./rq post localhost:8080/ -f name=Mohamad -B | grep body
  "body": "name=Mohamad",
$ ./rq post localhost:8080/multipart avatar@/tmp/letter.txt name=Mohamad -B
{
  "fields": { "name": "Mohamad" },
  "files": { "avatar": "letter.txt (23 bytes)" }
}

# A file upload implies multipart even without --multi
$ ./rq post localhost:8080/multipart avatar@/tmp/letter.txt -B

# Body items on GET become query parameters, like -b does today
$ ./rq get localhost:8080/ name=Mohamad -B | grep -A2 query
  "query": {
    "name": "Mohamad"
  },

# Items conflict with -b/--body: pick one
$ ./rq post localhost:8080/ name=Mohamad -b '{"x":1}'
Error: request items and --Nbody/--body both provide a body; use one or the other

# A file upload cannot be sent as a form
$ ./rq post localhost:8080/ -f avatar@/tmp/letter.txt
Error: a file upload cannot be sent as a url-encoded form; drop -f
```

### Scenario Q — Piped stdin body and --ignore-stdin

```shell
$ echo '{"name":"Mohamad"}' | ./rq post localhost:8080/ -B | grep -A2 body
  "body": {
    "name": "Mohamad"
  },

$ echo '{"name":"Mohamad"}' | ./rq post localhost:8080/ -B --ignore-stdin | grep '"body"'
  "body": "",

$ ./rq post localhost:8080/ -B < /dev/null | grep '"body"'   # empty, not an error
  "body": "",
```

An explicit body source (`-b`, `--body`, or a body-carrying request item)
always wins over piped stdin, so stdin is never read in that case.

### Scenario R — -v / --verbose

```shell
$ ./rq get localhost:8080/json -v -S
GET /json HTTP/1.1
Host:   localhost:8080
Accept:   */*
Accept-Encoding:   gzip, deflate, br
User-Agent:   rq/1.2.0

HTTP/1.1 200 OK
```

`-v` shows the request that was actually sent — including headers filled in by
defaults — followed by a blank line and the response, on stdout.

### Scenario U — streaming and server-sent events

The fixture's `/sse` endpoint streams events:

| Parameter | Effect |
| --------- | ------ |
| `events=N` | how many events to send (default 3, max 100) |
| `delay=D` | pause between events (default 10ms, max 5s) |
| `keepalive=1` | prepend a comment frame, which must not render as an event |
| `noterm=1` | omit the blank line after the final event |
| `gzip=1` | gzip-encode the stream, flushed per frame |
| `name=NAME` | set the `event:` field; `name=` sends frames with none |

```shell
$ ./rq get "localhost:8080/sse?events=3" --http -B
● delta  {"index":0,"text":"chunk 0"}
● delta  {"index":1,"text":"chunk 1"}
● delta  {"index":2,"text":"chunk 2"}
3 events · 84 B · first 2ms · total 22ms · 131.3 events/s
```

Confirm it is genuinely incremental rather than buffered — the offsets should
step by roughly the delay, not all land together:

```shell
$ ./rq get "localhost:8080/sse?events=4&delay=300ms" --http -B -v
● delta 1ms  {"index":0,"text":"chunk 0"}
● delta 302ms  {"index":1,"text":"chunk 1"}
● delta 603ms  {"index":2,"text":"chunk 2"}
● delta 904ms  {"index":3,"text":"chunk 3"}
```

Keepalives are framing, so they are neither rendered nor counted — but `--raw`
shows them:

```shell
$ ./rq get "localhost:8080/sse?events=2&keepalive=1" --http -B --raw
: keepalive
event: delta
data: {"index":0,"text":"chunk 0"}
event: delta
data: {"index":1,"text":"chunk 1"}
```

The summary is on stderr, so a pipe sees only events:

```shell
$ ./rq get "localhost:8080/sse?events=2" --http -B 2>/dev/null
● delta  {"index":0,"text":"chunk 0"}
● delta  {"index":1,"text":"chunk 1"}
```

`--timeout` bounds the headers, not the stream. This must print all three
events and take about 1.2s, not stop after one second:

```shell
$ ./rq get "localhost:8080/sse?events=3&delay=400ms" --http -B --timeout 1s
```

A gzip-encoded stream must decode *and* stay incremental — the offsets should
still step by the delay rather than arriving together. A compressor that is not
flushed per frame would hold them back:

```shell
$ ./rq get "localhost:8080/sse?events=4&delay=300ms&gzip=1" --http -B -v
● delta 2ms  {"index":0,"text":"chunk 0"}
● delta 303ms  {"index":1,"text":"chunk 1"}
● delta 604ms  {"index":2,"text":"chunk 2"}
● delta 904ms  {"index":3,"text":"chunk 3"}
```

A frame with no `event:` field is named `message`, per the SSE spec:

```shell
$ ./rq get "localhost:8080/sse?events=2&name=" --http -B
● message  {"index":0,"text":"chunk 0"}
● message  {"index":1,"text":"chunk 1"}
```

Ctrl-C keeps what arrived, prints the summary, and exits 130:

```shell
$ ./rq get "localhost:8080/sse?events=50&delay=200ms" --http -B
^C
5 events · 140 B · first 1ms · total 988ms · 5.1 events/s
$ echo $?
130
```

### Scenario S — --check-status and exit codes

```shell
$ ./rq get localhost:8080/status/404 --check-status; echo $?    # 4
$ ./rq get localhost:8080/status/500 --check-status; echo $?    # 5
$ ./rq get localhost:8080/redirect --check-status; echo $?      # 3 (not followed)
$ ./rq get 127.0.0.1:1 --http --check-status; echo $?           # 2 (transport failure)
$ ./rq get localhost:8080/status/404; echo $?                   # 0 — without the flag, still 0
```

---

## Testing against real sites

The fixture covers everything except brotli. For a sanity check against the
open internet:

```shell
$ ./rq get example.com -S
$ ./rq get https://api.github.com/repos/golang/go -B | head -20
$ ./rq get https://httpbin.org/gzip -B
$ ./rq post https://httpbin.org/post -b '{"hello":"world"}' -B
$ ./rq post https://httpbin.org/post hello=world -B
```

## Adding a test

When you fix a bug, add a regression test next to the others and mark it:

```go
// Regression: <what went wrong before>.
func TestSomething(t *testing.T) { ... }
```

Then add a line to `scripts/smoke.sh` if it is observable from the CLI.
