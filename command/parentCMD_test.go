package command

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanRequest(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "object across several lines",
			in:   "{\n\"a\":1\n};\n",
			want: `{"a":1}`,
		},
		{
			// Regression: a blank line used to panic on strTest[len(strTest)-1].
			name: "blank lines are skipped",
			in:   "{\n\n\"a\":1\n\n};\n",
			want: `{"a":1}`,
		},
		{
			// Regression: a "}" was appended unconditionally, corrupting arrays.
			name: "top level array survives",
			in:   "[\n1,\n2\n];\n",
			want: `[1,2]`,
		},
		{
			name: "single line terminated inline",
			in:   "{\"a\":1};\n",
			want: `{"a":1}`,
		},
		{
			name: "indentation is trimmed",
			in:   "{\n    \"a\":1\n};\n",
			want: `{"a":1}`,
		},
		{
			name: "nested structure",
			in:   "{\n\"a\":{\n\"b\":[1,2]\n}\n};\n",
			want: `{"a":{"b":[1,2]}}`,
		},
		{
			name: "eof without terminator still returns what was read",
			in:   "{\"a\":1}",
			want: `{"a":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanRequest(newScanner(strings.NewReader(tt.in)))
			if err != nil {
				t.Fatalf("scanRequest: %v", err)
			}
			if got != tt.want {
				t.Errorf("scanRequest(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScanRequestRejectsEmptyInput(t *testing.T) {
	for _, in := range []string{"", "\n\n", "   \n"} {
		if _, err := scanRequest(newScanner(strings.NewReader(in))); err == nil {
			t.Errorf("scanRequest(%q) should have returned an error", in)
		}
	}
}

// Regression: --headers and --body together read two documents from one stream.
// A per-call scanner buffered past the first ';' and swallowed the second.
func TestScanRequestReadsTwoDocumentsFromOneStream(t *testing.T) {
	stream := "{\n\"X-API-Token\": 123\n};\n{\n\"name\":\"Mohamad\"\n};\n"
	scanner := newScanner(strings.NewReader(stream))

	first, err := scanRequest(scanner)
	if err != nil {
		t.Fatalf("first document: %v", err)
	}
	if first != `{"X-API-Token": 123}` {
		t.Errorf("first document = %q", first)
	}

	second, err := scanRequest(scanner)
	if err != nil {
		t.Fatalf("second document: %v", err)
	}
	if second != `{"name":"Mohamad"}` {
		t.Errorf("second document = %q", second)
	}
}

func TestRunRequiresURL(t *testing.T) {
	if err := Run("GET", nil, &Options{}); err == nil {
		t.Error("Run with no args should return an error, not panic")
	}
	if err := Run("GET", []string{}, &Options{}); err == nil {
		t.Error("Run with empty args should return an error, not panic")
	}
}

func TestRunRejectsInvalidURL(t *testing.T) {
	if err := Run("GET", []string{"://nope"}, &Options{}); err == nil {
		t.Error("Run should reject a malformed URL")
	}
}

type captured struct {
	method string
	uri    string
	token  string
	cookie string
	body   string
	auth   [2]string
}

func captureServer(t *testing.T, got *captured) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method = r.Method
		got.uri = r.URL.RequestURI()
		got.token = r.Header.Get("X-Token")
		got.body = string(b)
		if c, err := r.Cookie("session"); err == nil {
			got.cookie = c.Value
		}
		if u, p, ok := r.BasicAuth(); ok {
			got.auth = [2]string{u, p}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunSendsEverythingItWasGiven(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		HeadersJS:   map[string]string{"X-Token": "abc"},
		BodyJS:      `{"a":1}`,
		QueryParams: map[string]string{"q": "1"},
		Cookies:     map[string]string{"session": "xyz"},
		Auth:        map[string]string{"username": "me", "password": "secret"},
		ShowStatus:  true,
		Timeout:     5 * time.Second,
	}

	if err := Run(http.MethodPost, []string{srv.URL}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.uri != "/?q=1" {
		t.Errorf("uri = %q, want /?q=1", got.uri)
	}
	if got.token != "abc" {
		t.Errorf("X-Token = %q, want abc", got.token)
	}
	if got.cookie != "xyz" {
		t.Errorf("cookie = %q, want xyz", got.cookie)
	}
	if got.body != `{"a":1}` {
		t.Errorf("body = %q", got.body)
	}
	if got.auth != [2]string{"me", "secret"} {
		t.Errorf("basic auth = %v, want [me secret]", got.auth)
	}
}

// Regression: --form used to replace the header map, discarding -n headers.
func TestRunFormKeepsExplicitHeaders(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		HeadersJS:  map[string]string{"X-Token": "abc"},
		BodyJS:     `{"k":"v"}`,
		Form:       true,
		ShowStatus: true,
		Timeout:    5 * time.Second,
	}

	if err := Run(http.MethodPost, []string{srv.URL}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got.token != "abc" {
		t.Errorf("--form dropped the explicit header: X-Token = %q, want abc", got.token)
	}
	if got.body != "k=v" {
		t.Errorf("body = %q, want k=v", got.body)
	}
}

func TestRunMergesBothHeaderSources(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		HeadersJS:  map[string]string{"X-Other": "1"},
		Headersjs:  `{"X-Token":"abc"}`,
		ShowStatus: true,
		Timeout:    5 * time.Second,
	}

	if err := Run(http.MethodGet, []string{srv.URL}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.token != "abc" {
		t.Errorf("X-Token = %q, want abc", got.token)
	}
}

func TestRunPutsJSONBodyInQueryForGet(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		BodyJS:     `{"a":1}`,
		ShowStatus: true,
		Timeout:    5 * time.Second,
	}

	if err := Run(http.MethodGet, []string{srv.URL}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.uri != "/?a=1" {
		t.Errorf("uri = %q, want /?a=1", got.uri)
	}
	if got.body != "" {
		t.Errorf("body = %q, want empty for GET", got.body)
	}
}

// A header item wins over -n/--headers for the same key: it is the most
// explicit source and is applied last.
func TestRunHeaderItemOverridesNheaders(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		HeadersJS:  map[string]string{"X-Token": "from-flag"},
		ShowStatus: true,
		Timeout:    5 * time.Second,
	}

	if err := Run(http.MethodGet, []string{srv.URL, "X-Token:from-item"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.token != "from-item" {
		t.Errorf("X-Token = %q, want from-item (item overrides -n)", got.token)
	}
}

// A HeaderUnset item ("Key:") must remove a header set by -n and suppress the
// User-Agent default, regardless of the case used in the item.
func TestRunHeaderUnsetItemRemovesFlagHeaderAndDefault(t *testing.T) {
	var gotUA string
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA, seen = r.Header.Get("User-Agent"), true
	}))
	t.Cleanup(srv.Close)

	opts := &Options{
		HeadersJS:  map[string]string{"X-Token": "abc"},
		ShowStatus: true,
		Timeout:    5 * time.Second,
	}

	if err := Run(http.MethodGet, []string{srv.URL, "X-Token:", "user-agent:"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !seen {
		t.Fatal("server never received the request")
	}
	if gotUA != "" {
		t.Errorf("User-Agent = %q, want none (unset by a lowercase item)", gotUA)
	}
}

// A query item overrides -q for the same key, and a key -q set that no item
// touches survives unchanged.
func TestRunQueryItemOverridesDashQAndKeepsOtherKeys(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{
		QueryParams: map[string]string{"a": "1", "b": "2"},
		ShowStatus:  true,
		Timeout:     5 * time.Second,
	}

	if err := Run(http.MethodGet, []string{srv.URL, "b==override"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	parsed, err := url.Parse("http://x" + got.uri)
	if err != nil {
		t.Fatalf("parse request URI: %v", err)
	}
	q := parsed.Query()
	if q.Get("a") != "1" {
		t.Errorf("a = %q, want 1 (untouched by items)", q.Get("a"))
	}
	if q.Get("b") != "override" {
		t.Errorf("b = %q, want override (item wins over -q)", q.Get("b"))
	}
}

// Repeated query items for the same key all survive, in order.
func TestRunRepeatedQueryItemsAllSurvive(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}

	if err := Run(http.MethodGet, []string{srv.URL, "tag==a", "tag==b"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	parsed, err := url.Parse("http://x" + got.uri)
	if err != nil {
		t.Fatalf("parse request URI: %v", err)
	}
	tags := parsed.Query()["tag"]
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tag = %v, want [a b]", tags)
	}
}

// A malformed request item is rejected before the request is ever sent.
func TestRunRejectsAnInvalidRequestItem(t *testing.T) {
	if err := Run(http.MethodGet, []string{"http://example.com", "b.com"}, &Options{}); err == nil {
		t.Error("Run should reject a positional argument that is not a request item")
	}
}

// Regression: applyHeaderItems used to walk HeaderOps's grouped set/unset
// lists, which put every unset ahead of every set regardless of what the user
// actually typed last. "X-Token: X-Token:a" must end up SET, since the
// second item was written after the first.
func TestRunLaterHeaderItemBeatsAnEarlierUnsetForTheSameKey(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}

	if err := Run(http.MethodGet, []string{srv.URL, "X-Token:", "X-Token:a"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.token != "a" {
		t.Errorf("X-Token = %q, want a (a later set must beat an earlier unset)", got.token)
	}
}

// The opposite order must also hold: a later unset removes an earlier set.
func TestRunLaterHeaderUnsetBeatsAnEarlierSetForTheSameKey(t *testing.T) {
	var got captured
	srv := captureServer(t, &got)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}

	if err := Run(http.MethodGet, []string{srv.URL, "X-Token:a", "X-Token:"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.token != "" {
		t.Errorf("X-Token = %q, want empty (a later unset must beat an earlier set)", got.token)
	}
}

// Request items and -b are mutually exclusive: merging them would require
// decoding the user's hand-written JSON to splice item fields in, which
// reintroduces the key-reordering problem this feature exists to avoid.
func TestRunRejectsBodyItemsCombinedWithDashB(t *testing.T) {
	opts := &Options{Body: true, BodyJS: `{"a":1}`}
	err := Run(http.MethodPost, []string{"http://example.com", "name=Mo"}, opts)
	if err == nil {
		t.Fatal("Run should reject request items combined with -b")
	}
	want := "request items and --Nbody/--body both provide a body; use one or the other"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// Same conflict, with --body (BodyJS set but Body flag false — the case
// where the JSON came from stdin rather than -b) instead of -b.
func TestRunRejectsBodyItemsCombinedWithDashDashBody(t *testing.T) {
	opts := &Options{BodyJS: `{"a":1}`}
	err := Run(http.MethodPost, []string{"http://example.com", "age:=22"}, opts)
	if err == nil {
		t.Fatal("Run should reject request items combined with --body")
	}
	want := "request items and --Nbody/--body both provide a body; use one or the other"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// With no -f/--multi, body items produce a JSON object with the matching
// Content-Type, key order preserved.
func TestRunBodyItemsDefaultToJSON(t *testing.T) {
	var got captured
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
	}))
	t.Cleanup(srv.Close)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}
	if err := Run(http.MethodPost, []string{srv.URL, "b:=2", "a:=1"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if got.body != `{"b":2,"a":1}` {
		t.Errorf("body = %q, want the fields in written order, never sorted", got.body)
	}
}

// -f encodes body items as a url-encoded form.
func TestRunBodyItemsWithFormFlagEncodeAsForm(t *testing.T) {
	var got captured
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
	}))
	t.Cleanup(srv.Close)

	opts := &Options{Form: true, ShowStatus: true, Timeout: 5 * time.Second}
	if err := Run(http.MethodPost, []string{srv.URL, "name=Mo", "age:=22"}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", contentType)
	}
	if got.body != "age=22&name=Mo" {
		t.Errorf("body = %q, want age=22&name=Mo", got.body)
	}
}

// --multi encodes body items as multipart, and the server sees the file's
// real content, not just its path.
func TestRunBodyItemsWithMultiFlagEncodeAsMultipart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "me.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	var contentType string
	var form *multipart.Form
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("server: ParseMultipartForm: %v", err)
			return
		}
		form = r.MultipartForm
	}))
	t.Cleanup(srv.Close)

	opts := &Options{Multipart: true, ShowStatus: true, Timeout: 5 * time.Second}
	if err := Run(http.MethodPost, []string{srv.URL, "name=Mo", "avatar@" + path}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", contentType)
	}
	if got := form.Value["name"]; len(got) != 1 || got[0] != "Mo" {
		t.Errorf(`form field "name" = %v, want ["Mo"]`, got)
	}
	if len(form.File["avatar"]) != 1 {
		t.Fatalf("expected one uploaded file named avatar, got %v", form.File)
	}
}

// A key@file item implies multipart even with no flag at all.
func TestRunFileUploadImpliesMultipartWithoutAnyFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "me.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
	}))
	t.Cleanup(srv.Close)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}
	if err := Run(http.MethodPost, []string{srv.URL, "avatar@" + path}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data (implied by the file upload)", contentType)
	}
}

// A file upload combined with -f is an error, not a silently dropped file.
func TestRunFileUploadWithFormFlagIsAnError(t *testing.T) {
	opts := &Options{Form: true}
	err := Run(http.MethodPost, []string{"http://example.com", "avatar@./me.png"}, opts)
	if err == nil {
		t.Fatal("Run should reject a file upload combined with -f")
	}
	want := "a file upload cannot be sent as a url-encoded form; drop -f"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// A file upload on a verb that carries its data in the query string is an
// error, since there is nowhere for the file's bytes to go.
func TestRunFileUploadOnQueryVerbIsAnError(t *testing.T) {
	opts := &Options{}
	err := Run(http.MethodGet, []string{"http://example.com", "avatar@./me.png"}, opts)
	if err == nil {
		t.Fatal("Run should reject a file upload on GET")
	}
	want := "GET cannot send a file upload; use POST, PUT or PATCH"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// On a verb that carries its data in the query string, body-carrying items
// become query parameters instead: a string field, a raw number, a raw array
// kept as compact JSON text, and a file field contributing its content.
func TestRunBodyItemsOnQueryVerbBecomeQueryParams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bio.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	var got captured
	srv := captureServer(t, &got)

	opts := &Options{ShowStatus: true, Timeout: 5 * time.Second}
	if err := Run(http.MethodGet, []string{
		srv.URL, "name=Mo", "age:=22", "tags:=[1,2]", "bio=@" + path,
	}, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.body != "" {
		t.Errorf("body = %q, want empty for GET", got.body)
	}

	parsed, err := url.Parse("http://x" + got.uri)
	if err != nil {
		t.Fatalf("parse request URI: %v", err)
	}
	q := parsed.Query()
	if q.Get("name") != "Mo" {
		t.Errorf("name = %q, want Mo", q.Get("name"))
	}
	if q.Get("age") != "22" {
		t.Errorf("age = %q, want 22", q.Get("age"))
	}
	if q.Get("tags") != "[1,2]" {
		t.Errorf("tags = %q, want compact JSON text [1,2]", q.Get("tags"))
	}
	if q.Get("bio") != "hello" {
		t.Errorf("bio = %q, want the file's content", q.Get("bio"))
	}
}

func TestRunSurfacesConnectionErrors(t *testing.T) {
	opts := &Options{ShowStatus: true, Timeout: 2 * time.Second, HTTP: true}

	err := Run(http.MethodGet, []string{"127.0.0.1:1"}, opts)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error %q should name the target", err)
	}
}
