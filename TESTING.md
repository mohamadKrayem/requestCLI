# Testing requestCLI

There are three ways to test this project, from cheapest to most thorough:

| Layer            | Command              | What it covers                                  |
| ---------------- | -------------------- | ----------------------------------------------- |
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
$ go test ./requests/ -run TestGenerateUrl -v
$ go test ./command/ -run TestScanRequest -v
```

## 2. End-to-end smoke test

```shell
$ make smoke
```

This builds the binary, starts the fixture server, runs 51 assertions covering
every flag and subcommand, and prints a pass/fail summary. It cleans up after
itself. Use this as the "did I break anything" check before committing.

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
| `/status/<code>` | Returns that status code                    |
| `/basic-auth`    | Requires Basic Auth                         |
| `/multipart`     | Parses a multipart form and reports it      |

---

### Scenario A — HTTP methods

Confirm each subcommand sends its own verb. The echo endpoint reports it back.

```shell
$ ./requestCLI get     localhost:8080/ -B | grep method     # "method": "GET"
$ ./requestCLI post    localhost:8080/ -B | grep method     # "method": "POST"
$ ./requestCLI put     localhost:8080/ -B | grep method     # "method": "PUT"
$ ./requestCLI patch   localhost:8080/ -B | grep method     # "method": "PATCH"
$ ./requestCLI del     localhost:8080/ -B | grep method     # "method": "DELETE"
$ ./requestCLI options localhost:8080/ -B | grep method     # "method": "OPTIONS"
$ ./requestCLI trace   localhost:8080/ -B | grep method     # "method": "TRACE"
```

Aliases:

```shell
$ ./requestCLI delete localhost:8080/ -B | grep method      # "method": "DELETE"
$ ./requestCLI conn   localhost:8080/ -S                    # 200 OK
```

> **Note**: `head` returns no body by design — HEAD responses have none. Use
> `-H` to see its headers.

### Scenario B — Output selection

```shell
$ ./requestCLI get localhost:8080/json -S      # status line only
$ ./requestCLI get localhost:8080/json -H      # headers only
$ ./requestCLI get localhost:8080/json -B      # body only
$ ./requestCLI get localhost:8080/json -H -B   # headers and body, no status
$ ./requestCLI get localhost:8080/json         # everything
```

Expected for `-S`:

```
HTTP/1.1 200 OK
```

The three flags combine. Passing none of them prints everything.

### Scenario C — URL scheme and TLS

Scheme-less URLs default to https, except loopback hosts:

```shell
$ ./requestCLI get example.com -S            # -> https://example.com
$ ./requestCLI get localhost:8080/text -S    # -> http://localhost:8080
$ ./requestCLI get example.com --http -S     # -> http://example.com
```

Certificate verification is on by default:

```shell
$ ./requestCLI get https://localhost:8443/json -S
Error: sending GET https://localhost:8443/json: Get "https://localhost:8443/json":
tls: failed to verify certificate: x509: certificate signed by unknown authority

$ ./requestCLI get https://localhost:8443/json -B -k     # succeeds
$ ./requestCLI get https://localhost:8443/ -B -k | grep tls
  "tls": true
```

That first failure is the correct result — the fixture uses a self-signed
certificate. If it ever *succeeds* without `-k`, verification has regressed.

### Scenario D — Headers

```shell
$ ./requestCLI get localhost:8080/ -n X-API-Token=123 -B | grep X-Api
      "X-Api-Token": "123",

$ ./requestCLI get localhost:8080/ -n A=1 -n B=2 -B          # repeated flag
$ ./requestCLI get localhost:8080/ -n A=1,B=2 -B             # comma-separated
```

Multi-line headers from stdin, terminated by `;`:

```shell
$ ./requestCLI get localhost:8080/ --headers -B
{
	"X-API-Token": "123"
};
```

### Scenario E — Request bodies

```shell
# JSON body on a method that carries one
$ ./requestCLI post localhost:8080/ -b '{"name":"Mohamad"}' -B | grep body
  "body": "{\"name\":\"Mohamad\"}",

# JSON body on GET becomes query parameters instead
$ ./requestCLI get localhost:8080/ -b '{"a":1}' -B | grep -A2 query
  "query": {
    "a": "1"
  },

# A body that is not JSON is sent verbatim as text/plain
$ ./requestCLI get localhost:8080/ -b 'hello' -B | grep -E 'body|text/plain'
  "body": "hello",
    "Content-Type": "text/plain",
```

Multi-line body from stdin:

```shell
$ ./requestCLI post localhost:8080/ --body -B
{
	"name":"Mohamad",
	"arrayOfNbs":[1,2,3],
	"nested":{"w":"2"}
};
```

Top-level arrays work, and blank lines are ignored:

```shell
$ printf '[\n1,\n2,\n3\n];\n' | ./requestCLI post localhost:8080/ --body -B | grep body
  "body": "[1,2,3]",
```

Headers and body together — two documents, one stream:

```shell
$ ./requestCLI put localhost:8080/ --headers --body -B
{
	"X-API-Token":"123"
};
{
	"name":"Mohamad"
};
```

### Scenario F — Forms

```shell
$ ./requestCLI post localhost:8080/ -f -b '{"key1":"value","key2":21}' -B
  "body": "key1=value&key2=21",
    "Content-Type": "application/x-www-form-urlencoded",
```

`-f` must not discard explicit headers:

```shell
$ ./requestCLI post localhost:8080/ -f -n X-API-Token=123 -b '{"k":"v"}' -B | grep X-Api
      "X-Api-Token": "123",
```

### Scenario G — Multipart and file uploads

```shell
$ echo "hello from a text file" > /tmp/letter.txt

$ ./requestCLI post localhost:8080/multipart --multi \
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
$ ./requestCLI post localhost:8080/multipart --multi -b '{"@!f":"~/notes.txt"}' -B
$ ./requestCLI post localhost:8080/multipart --multi -b '{"@!f":"letter.txt"}' -B
```

A form with no files must still be valid:

```shell
$ ./requestCLI post localhost:8080/multipart --multi -b '{"name":"Mohamad"}' -B
```

A missing file should fail cleanly, not panic:

```shell
$ ./requestCLI post localhost:8080/multipart --multi -b '{"@!x":"/nope.txt"}'
Error: opening /nope.txt: open /nope.txt: no such file or directory
```

### Scenario H — Query parameters, cookies, auth

```shell
$ ./requestCLI get localhost:8080/ -q a=1,b=two -B | grep -A3 query
  "query": {
    "a": "1",
    "b": "two"
  },

$ ./requestCLI get localhost:8080/ -c session=abc,theme=dark -B | grep -A3 cookies
  "cookies": {
    "session": "abc",
    "theme": "dark"
  },

$ ./requestCLI get localhost:8080/basic-auth -a username=Mohamad,password=pass123 -B
{
  "authenticated": "yes",
  "password": "pass123",
  "username": "Mohamad"
}

$ ./requestCLI get localhost:8080/basic-auth -S
HTTP/1.1 401 Unauthorized
```

Combining `-q` with a form body must not produce a malformed URL:

```shell
$ ./requestCLI get localhost:8080/ -q a=1 -f -b '{"b":2}' -B | grep -A3 query
  "query": {
    "a": "1",
    "b": "2"
  },
```

The server should log `uri=/?a=1&b=2` — with `&`, not a second `?`.

### Scenario I — Redirects

```shell
$ ./requestCLI get localhost:8080/redirect -S
HTTP/1.1 302 Found

$ ./requestCLI get localhost:8080/redirect --redirect -S
HTTP/1.1 200 OK
```

### Scenario J — Compression

```shell
$ ./requestCLI get localhost:8080/gzip -B
{
  "encoding": "gzip",
  "message": "decoded correctly"
}

$ ./requestCLI get localhost:8080/deflate -B
```

> **Note**: brotli decoding cannot be exercised by the local fixture — the
> `dsnet/compress` dependency decodes brotli but cannot produce it. Test it
> against a real CDN-backed site, e.g. `./requestCLI get https://www.cloudflare.com -S`.

### Scenario K — Timeouts

```shell
$ time ./requestCLI get "localhost:8080/slow?seconds=5" --timeout 1s -S
Error: sending GET http://localhost:8080/slow?seconds=5: ... (Client.Timeout exceeded ...)
real    0m1.026s
```

It must return in about a second, not five. Without `--timeout` the default is
30s.

### Scenario L — Status codes

```shell
$ ./requestCLI get localhost:8080/status/201 -S    # HTTP/1.1 201 Created
$ ./requestCLI get localhost:8080/status/404 -S    # HTTP/1.1 404 Not Found
$ ./requestCLI get localhost:8080/status/500 -S    # HTTP/1.1 500 Internal Server Error
```

Note that an HTTP error status is not a CLI failure — the request succeeded.
The exit code is 0.

### Scenario M — Content-type rendering

```shell
$ ./requestCLI get localhost:8080/json -B    # pretty-printed, colorized JSON
$ ./requestCLI get localhost:8080/html -B    # syntax-highlighted HTML
$ ./requestCLI get localhost:8080/text -B    # plain
```

Piping should produce clean, uncolored output:

```shell
$ ./requestCLI get localhost:8080/json -B | cat
```

### Scenario N — Error handling

None of these should panic; each should print one clear line and exit non-zero.

```shell
$ ./requestCLI put
Error: accepts 1 arg(s), received 0

$ ./requestCLI get a.com b.com
Error: accepts 1 arg(s), received 2

$ ./requestCLI fetch example.com
Error: unknown command "fetch" for "requestCLI"

$ ./requestCLI get 127.0.0.1:1 --http
Error: sending GET http://127.0.0.1:1: ... connect: connection refused

$ ./requestCLI get '://nope'
Error: invalid URL "https://://nope": no host

$ ./requestCLI post localhost:8080/ --multi -b '{bad'
Error: --multi needs a json object as the body: parsing json object: ...
```

Exit codes:

```shell
$ ./requestCLI get localhost:8080/text -S >/dev/null; echo $?   # 0
$ ./requestCLI get 127.0.0.1:1 --http   >/dev/null 2>&1; echo $?   # 1
```

### Scenario O — Regression checks

These are the defects found in the code review. Each should now behave
correctly rather than crashing.

```shell
# 1. Missing URL on put/head/options/patch (used to panic)
$ ./requestCLI put; ./requestCLI head; ./requestCLI options; ./requestCLI patch

# 2. Blank line inside --body (used to panic)
$ printf '{\n\n"a":1\n};\n' | ./requestCLI post localhost:8080/ --body -S

# 3. Multipart field name shorter than two characters (used to panic)
$ ./requestCLI post localhost:8080/multipart --multi -b '{"a":1}' -B

# 4. Non-JSON body (used to exit via log.Fatal)
$ ./requestCLI get localhost:8080/ -b 'hello' -B

# 5. trace used to send CONNECT
$ ./requestCLI trace localhost:8080/ -B | grep method     # "method": "TRACE"

# 6. Query params used to produce ?a=1?b=2
$ ./requestCLI get localhost:8080/ -q a=1 -f -b '{"b":2}' -B

# 7. -f used to wipe -n headers
$ ./requestCLI post localhost:8080/ -f -n X-API-Token=123 -b '{"k":"v"}' -B

# 8. --headers --body together used to drop the body
$ printf '{\n"X-API-Token":"1"\n};\n{\n"a":1\n};\n' | \
    ./requestCLI put localhost:8080/ --headers --body -B

# 9. Connection errors used to print a pointer pair
$ ./requestCLI get 127.0.0.1:1 --http

# 10. Top-level array bodies used to be corrupted
$ printf '[\n1,\n2\n];\n' | ./requestCLI post localhost:8080/ --body -B
```

---

## Testing against real sites

The fixture covers everything except brotli. For a sanity check against the
open internet:

```shell
$ ./requestCLI get example.com -S
$ ./requestCLI get https://api.github.com/repos/golang/go -B | head -20
$ ./requestCLI get https://httpbin.org/gzip -B
$ ./requestCLI post https://httpbin.org/post -b '{"hello":"world"}' -B
```

## Adding a test

When you fix a bug, add a regression test next to the others and mark it:

```go
// Regression: <what went wrong before>.
func TestSomething(t *testing.T) { ... }
```

Then add a line to `scripts/smoke.sh` if it is observable from the CLI.
