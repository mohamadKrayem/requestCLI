# Request-CLI

Request-CLI, inspired by tools like httpie and curl, is designed to be beginner-friendly with strong JSON operations, making it easy to execute various HTTP requests. The app offers advanced features such as handling cookies, basic authentication, multipart file uploads, and formatted, colorized output.

## Installation

Make sure to have Go installed on your system. You can download it from [https://go.dev/doc/install](https://go.dev/doc/install). Verify the installation by running the following command in your terminal:

```shell
$ go version
```

To install Request-CLI:

```shell
$ go install github.com/mohamadkrayem/requestCLI@latest
```

Or build from a checkout:

```shell
$ git clone https://github.com/mohamadkrayem/requestCLI.git
$ cd requestCLI
$ make build
```

### Docker

```shell
$ docker build -t requestcli .
$ docker run --rm requestcli get example.com -S
```

## Usage

```shell
$ requestCLI get example.com
```

For more information:

```shell
$ requestCLI --help
$ requestCLI get --help
```

### Commands

| Command   | Method  | Aliases  |
| --------- | ------- | -------- |
| `get`     | GET     |          |
| `post`    | POST    |          |
| `put`     | PUT     |          |
| `patch`   | PATCH   |          |
| `del`     | DELETE  | `delete` |
| `head`    | HEAD    |          |
| `options` | OPTIONS |          |
| `trace`   | TRACE   |          |
| `connect` | CONNECT | `conn`   |

Every command takes exactly one URL.

### Flags

| Flag         | Short | Description                                                 |
| ------------ | ----- | ----------------------------------------------------------- |
| `--Nbody`    | `-b`  | Body as JSON on a single line.                              |
| `--body`     |       | Read a multi-line JSON body from stdin, terminated by `;`.  |
| `--Nheaders` | `-n`  | Headers as `key=value` pairs.                               |
| `--headers`  |       | Read multi-line JSON headers from stdin, terminated by `;`. |
| `--query`    | `-q`  | Query parameters as `key=value` pairs.                      |
| `--cookie`   | `-c`  | Cookies as `key=value` pairs.                               |
| `--auth`     | `-a`  | Basic auth, e.g. `username=me,password=secret`.             |
| `--form`     | `-f`  | Send a url-encoded form.                                    |
| `--multi`    |       | Send a multipart form (supports file uploads).              |
| `--printB`   | `-B`  | Print the response body.                                    |
| `--printH`   | `-H`  | Print the response headers.                                 |
| `--printS`   | `-S`  | Print the response status line.                             |
| `--redirect` |       | Follow redirects.                                           |
| `--http`     |       | Force plain HTTP instead of HTTPS.                          |
| `--insecure` | `-k`  | Skip TLS certificate verification (dangerous).              |
| `--timeout`  |       | Overall request timeout (default `30s`).                    |

The `-B`, `-H` and `-S` flags combine: `-H -B` prints headers and body. Passing
none of them prints everything.

### URL scheme

Scheme-less URLs default to **https**, except loopback hosts (`localhost`,
`127.0.0.1`, `[::1]`) which default to **http** so local development keeps
working. Pass `--http` to force plain HTTP for any host.

```shell
$ requestCLI get example.com          # -> https://example.com
$ requestCLI get localhost:3000       # -> http://localhost:3000
$ requestCLI get example.com --http   # -> http://example.com
```

### TLS

Certificate verification is **on** by default. Use `-k` / `--insecure` to talk
to a server with a self-signed certificate — this removes protection against
man-in-the-middle attacks, so use it only against hosts you control.

```shell
$ requestCLI get https://self-signed.local -k
```

## Examples

Simple headers and a JSON body:

```shell
$ requestCLI post example.com -n X-API-Token=123 -b='{"name":"Mohamad"}'
```

Multiple simple headers:

```shell
$ requestCLI post example.com -n X-API-Token-1=123 -n X-API-Token-2=456
```

Or separated by a comma:

```shell
$ requestCLI post example.com -n X-API-Token-1=123,X-API-Token-2=456
```

---

Complex headers and body — use `--headers` and `--body`, and terminate each
block with `;`:

```shell
$ requestCLI put example.com --headers --body
{
	"X-API-Token": 123
};
{
	"name":"Mohamad",
	"arrayOfNbs": [1, 2, 3],
	"nestedJS": { "w":"2" }
};
```

Write complete JSON, including the closing brace or bracket. Top-level arrays
are supported, and blank lines are ignored.

---

Cookies:

```shell
$ requestCLI get example.com -n X-API-Token=123 -c key=value
```

Basic authentication:

```shell
$ requestCLI get example.com --auth username=Mohamad,password=pass123
```

> **Note**: credentials passed this way land in your shell history and are
> visible in `ps`. Prefer a throwaway credential for anything sensitive.

Query string parameters:

```shell
$ requestCLI get example.com -q q=queryExample,per_page=1
```

Output selection:

```shell
$ requestCLI get example.com -H        # headers only
$ requestCLI get example.com -H -B     # headers and body
```

Other methods:

```shell
$ requestCLI del example.com
$ requestCLI post "http://example.com" -b='{"name":"Example"}'
$ requestCLI put example.com -b='{"name":"Example"}'
```

---

## Sending forms and files

Multipart form with files:

```shell
$ requestCLI post example.com/form --multi --body
{
	"@!image":"~/justForTesting/OIG.jpeg",
	"@!resume":"~/justForTesting/Mohamad_Krayem_2023_CV.docx",
	"@!letter":"~/justForTesting/letter.txt",
	"name":"Mohamad",
	"age":22
};
```

File fields must begin with `@!`; normal data fields carry no prefix.
Path handling:

- `~` — resolved against your home directory.
- `/` — used as-is.
- anything else — resolved against the current working directory.

---

Url-encoded form:

```shell
$ requestCLI post example.com -f --body
{
	"key1":"value",
	"key2":21
};
```

> **Note**: use `-b` for a single-line body or `--body` for a multi-line one.

---

## Development

```shell
$ make test     # go test -race ./...
$ make cover    # coverage summary
$ make vet
$ make lint     # requires golangci-lint
$ make build
```

## Testing

See [TESTING.md](TESTING.md) for the full guide, including manual scenarios.

```shell
$ make test     # unit and integration tests, no network needed
$ make smoke    # end-to-end run of the real binary against a local fixture
$ make server   # start the fixture server for manual testing
```

## Technologies Used

- Golang
- Cobra

## Future Features

- JWT authentication
- Digest authentication
- Proxies
- Client SSL certificates
- Downloading response bodies to a file

## License

See [LICENSE](LICENSE).
