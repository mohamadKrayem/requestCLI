package httpfile

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parse(t *testing.T, src string) *File {
	t.Helper()
	file, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return file
}

func TestParseSingleRequest(t *testing.T) {
	file := parse(t, `GET https://example.com/users
X-Token: abc
Accept: application/json

{"name":"ada"}`)

	if len(file.Requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(file.Requests))
	}
	got := file.Requests[0]

	if got.Method != http.MethodGet {
		t.Errorf("Method = %q, want GET", got.Method)
	}
	if got.URL != "https://example.com/users" {
		t.Errorf("URL = %q", got.URL)
	}
	if len(got.Headers) != 2 {
		t.Fatalf("got %d headers, want 2", len(got.Headers))
	}
	if got.Headers[0] != (Header{Name: "X-Token", Value: "abc"}) {
		t.Errorf("Headers[0] = %+v", got.Headers[0])
	}
	if string(got.Body) != `{"name":"ada"}` {
		t.Errorf("Body = %q", got.Body)
	}
}

// ### separates requests and names the one that follows.
func TestParseMultipleRequestsAndNames(t *testing.T) {
	file := parse(t, `### Create a user
POST https://example.com/users

{"name":"ada"}

### List users
GET https://example.com/users
`)

	if len(file.Requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(file.Requests))
	}
	if file.Requests[0].Name != "Create a user" {
		t.Errorf("Requests[0].Name = %q", file.Requests[0].Name)
	}
	if file.Requests[1].Name != "List users" {
		t.Errorf("Requests[1].Name = %q", file.Requests[1].Name)
	}
	if file.Requests[0].Method != http.MethodPost {
		t.Errorf("Requests[0].Method = %q, want POST", file.Requests[0].Method)
	}
}

// "# @name x" is the explicit form, and must win over the separator text.
func TestParseNameDirectiveOverridesSeparator(t *testing.T) {
	file := parse(t, `### ignored
# @name createUser
POST https://example.com/users
`)

	if file.Requests[0].Name != "createUser" {
		t.Errorf("Name = %q, want createUser", file.Requests[0].Name)
	}
}

// A bare URL is a GET, which is what the dialect specifies.
func TestParseBareURLIsGet(t *testing.T) {
	file := parse(t, "https://example.com/health\n")

	if file.Requests[0].Method != http.MethodGet {
		t.Errorf("Method = %q, want GET", file.Requests[0].Method)
	}
	if file.Requests[0].URL != "https://example.com/health" {
		t.Errorf("URL = %q", file.Requests[0].URL)
	}
}

// A trailing HTTP version is part of the grammar and carries nothing we act on.
func TestParseAcceptsAnHTTPVersion(t *testing.T) {
	file := parse(t, "GET https://example.com/ HTTP/1.1\n")

	if file.Requests[0].URL != "https://example.com/" {
		t.Errorf("URL = %q, want the version stripped", file.Requests[0].URL)
	}
}

func TestParseCollectsFileVars(t *testing.T) {
	file := parse(t, `@base = https://example.com
@token = abc123

GET {{base}}/users
`)

	if len(file.Vars) != 2 {
		t.Fatalf("got %d vars, want 2", len(file.Vars))
	}
	if file.Vars[0].Name != "base" || file.Vars[0].Value != "https://example.com" {
		t.Errorf("Vars[0] = %+v", file.Vars[0])
	}
}

// Comments are skipped, but a "#" inside a body is data.
func TestParseCommentsAreSkippedButBodyIsLiteral(t *testing.T) {
	file := parse(t, `# a comment
// another comment
POST https://example.com/
Content-Type: application/json

{"note":"# not a comment","path":"//also not"}
`)

	if len(file.Requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(file.Requests))
	}
	if !strings.Contains(string(file.Requests[0].Body), "# not a comment") {
		t.Errorf("Body = %q, want the # preserved", file.Requests[0].Body)
	}
	if !strings.Contains(string(file.Requests[0].Body), "//also not") {
		t.Errorf("Body = %q, want the // preserved", file.Requests[0].Body)
	}
}

// A blank line inside a body belongs to the body.
func TestParseKeepsBlankLinesInsideABody(t *testing.T) {
	file := parse(t, `POST https://example.com/

line one

line three
`)

	if string(file.Requests[0].Body) != "line one\n\nline three" {
		t.Errorf("Body = %q", file.Requests[0].Body)
	}
}

// "< ./path" is a body read from a file.
func TestParseBodyFromFileReference(t *testing.T) {
	file := parse(t, `POST https://example.com/

< ./payload.json
`)

	if file.Requests[0].BodyFile != "./payload.json" {
		t.Errorf("BodyFile = %q", file.Requests[0].BodyFile)
	}
	if len(file.Requests[0].Body) != 0 {
		t.Errorf("Body = %q, want empty until read", file.Requests[0].Body)
	}
}

// Duplicate headers are legitimate and the file is the record of intent.
func TestParsePreservesDuplicateHeaders(t *testing.T) {
	file := parse(t, `GET https://example.com/
Accept: application/json
Accept: text/plain
`)

	if len(file.Requests[0].Headers) != 2 {
		t.Fatalf("got %d headers, want both kept", len(file.Requests[0].Headers))
	}
}

// A header value may contain a colon — a URL in a Referer, a port.
func TestParseHeaderValueMayContainColons(t *testing.T) {
	file := parse(t, `GET https://example.com/
Referer: https://example.com:8443/page
`)

	if got := file.Requests[0].Headers[0].Value; got != "https://example.com:8443/page" {
		t.Errorf("Value = %q", got)
	}
}

// A separator ends a request even mid-body: ### is absolutely reserved.
func TestParseSeparatorEndsABody(t *testing.T) {
	file := parse(t, `POST https://example.com/a

first body
###
POST https://example.com/b

second body
`)

	if len(file.Requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(file.Requests))
	}
	if string(file.Requests[0].Body) != "first body" {
		t.Errorf("Requests[0].Body = %q", file.Requests[0].Body)
	}
}

// An annotation this slice does not implement must not break the file, which
// is the property that makes the format extensible without forking it.
func TestParseIgnoresUnknownRequestDirectives(t *testing.T) {
	file := parse(t, `GET https://example.com/
# @ignore body.timestamp
# @snapshot
`)

	if len(file.Requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(file.Requests))
	}
}

func TestParseEmptyInput(t *testing.T) {
	file := parse(t, "")
	if len(file.Requests) != 0 {
		t.Errorf("got %d requests, want 0", len(file.Requests))
	}
}

// An error has to say which line, or the reader has to go hunting.
func TestParseErrorsCarryTheLine(t *testing.T) {
	for name, tc := range map[string]struct {
		src      string
		wantLine int
		wantMsg  string
	}{
		"malformed header": {
			src:      "GET https://example.com/\nthis is not a header\n",
			wantLine: 2,
			wantMsg:  "Name: value",
		},
		"empty header name": {
			src:      "GET https://example.com/\n: orphaned\n",
			wantLine: 2,
			wantMsg:  "header name is empty",
		},
		"junk after url": {
			src:      "GET https://example.com/ nonsense\n",
			wantLine: 1,
			wantMsg:  "METHOD URL",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.src))
			if err == nil {
				t.Fatal("expected an error")
			}
			var parseErr *ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error is %T, want *ParseError", err)
			}
			if parseErr.Line != tc.wantLine {
				t.Errorf("Line = %d, want %d", parseErr.Line, tc.wantLine)
			}
			if !strings.Contains(parseErr.Msg, tc.wantMsg) {
				t.Errorf("Msg = %q, want it to mention %q", parseErr.Msg, tc.wantMsg)
			}
		})
	}
}

func TestParseFileRecordsItsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.http")
	if err := os.WriteFile(path, []byte("GET https://example.com/\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if file.Dir != dir {
		t.Errorf("Dir = %q, want %q", file.Dir, dir)
	}
}

// A parse failure from a file reports file:line:, the form editors and
// terminals turn into a clickable jump.
func TestParseFileErrorIsAClickablePosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.http")
	if err := os.WriteFile(path, []byte("GET https://example.com/\nbad header\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), path+":2:") {
		t.Errorf("error = %q, want it to start with %q", err, path+":2:")
	}
}

// Parsed from a reader there is no file to name, so the position degrades to
// the line alone rather than printing an empty path.
func TestParseErrorWithoutAFileNamesOnlyTheLine(t *testing.T) {
	_, err := Parse(strings.NewReader("GET https://example.com/\nbad header\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.HasPrefix(err.Error(), "line 2:") {
		t.Errorf("error = %q, want it to start with \"line 2:\"", err)
	}
}

func TestPosition(t *testing.T) {
	if got := Position("api.http", 7); got != "api.http:7:" {
		t.Errorf("Position = %q", got)
	}
	if got := Position("", 7); got != "line 7:" {
		t.Errorf("Position = %q", got)
	}
}
