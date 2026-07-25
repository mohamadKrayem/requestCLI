package command

import (
	"io"
	"net/http"
	"net/http/httptest"
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
