package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateURL(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		forceHTTP bool
		query     map[string]string
		want      string
	}{
		{name: "defaults to https", in: "example.com", want: "https://example.com"},
		{name: "localhost defaults to http", in: "localhost:3000", want: "http://localhost:3000"},
		{name: "loopback ipv4 defaults to http", in: "127.0.0.1:8099", want: "http://127.0.0.1:8099"},
		{name: "loopback ipv6 defaults to http", in: "[::1]:8080", want: "http://[::1]:8080"},
		{name: "forceHTTP overrides https", in: "example.com", forceHTTP: true, want: "http://example.com"},
		{name: "explicit http kept", in: "http://example.com", want: "http://example.com"},
		{name: "explicit https kept", in: "https://example.com", want: "https://example.com"},
		{
			// Regression: a scheme inside the query string must not be mistaken
			// for the URL's own scheme.
			name: "scheme inside query is not a prefix",
			in:   "example.com/r?to=http://evil.com",
			want: "https://example.com/r?to=http://evil.com",
		},
		{name: "query params applied", in: "example.com", query: map[string]string{"a": "1"}, want: "https://example.com?a=1"},
		{
			name:  "query params merge with existing",
			in:    "example.com/?a=1",
			query: map[string]string{"b": "2"},
			want:  "https://example.com/?a=1&b=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateURL(tt.in, tt.forceHTTP, tt.query)
			if err != nil {
				t.Fatalf("GenerateURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("GenerateURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGenerateURLRejectsMalformedInput(t *testing.T) {
	// "://nope" parses to the non-empty host ":" and needs the Hostname check.
	for _, in := range []string{"", "   ", "://nope", "http://"} {
		if _, err := GenerateURL(in, false, nil); err == nil {
			t.Errorf("GenerateURL(%q) should have failed", in)
		}
	}
}

// Regression: query params used to be appended with a literal "?", producing
// "?a=1?b=2" when the URL already carried a query string.
func TestAddQueryStringMergesInsteadOfAppending(t *testing.T) {
	req := NewRequest(http.MethodGet, "http://127.0.0.1:8099/?a=1")

	if err := req.AddQueryString(map[string]any{"b": float64(2)}); err != nil {
		t.Fatalf("AddQueryString: %v", err)
	}

	want := "http://127.0.0.1:8099/?a=1&b=2"
	if req.URL != want {
		t.Errorf("URL = %q, want %q", req.URL, want)
	}
	if strings.Count(req.URL, "?") != 1 {
		t.Errorf("URL %q should contain exactly one '?'", req.URL)
	}
}

func TestToQueryValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "string", in: "hello", want: "hello"},
		{name: "bool", in: true, want: "true"},
		{name: "int", in: 42, want: "42"},
		// Regression: float32 fell through an empty switch case and was dropped.
		{name: "float32", in: float32(1.5), want: "1.5"},
		{name: "float64", in: float64(2.25), want: "2.25"},
		{name: "float64 integral", in: float64(3), want: "3"},
		{name: "nil", in: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toQueryValue(tt.in); got != tt.want {
				t.Errorf("toQueryValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Regression: this fallback was unreachable because the JSON parser called
// log.Fatal before returning its error.
func TestWithBodyFallsBackToTextPlain(t *testing.T) {
	req := NewRequest(http.MethodGet, "http://example.com/")

	if err := req.WithBody("hello", false, false); err != nil {
		t.Fatalf("WithBody: %v", err)
	}
	if req.Body != "hello" {
		t.Errorf("Body = %q, want %q", req.Body, "hello")
	}
	if got := req.Headers["Content-Type"]; got != "text/plain" {
		t.Errorf("Content-Type = %v, want text/plain", got)
	}
}

func TestWithBodyJSONOnGetBecomesQueryParams(t *testing.T) {
	req := NewRequest(http.MethodGet, "http://example.com/")

	if err := req.WithBody(`{"a":1}`, false, false); err != nil {
		t.Fatalf("WithBody: %v", err)
	}
	if req.Body != "" {
		t.Errorf("Body = %q, want empty (data belongs in the query)", req.Body)
	}
	if want := "http://example.com/?a=1"; req.URL != want {
		t.Errorf("URL = %q, want %q", req.URL, want)
	}
}

func TestWithBodyFormOnPost(t *testing.T) {
	req := NewRequest(http.MethodPost, "http://example.com/")

	if err := req.WithBody(`{"a":"1"}`, true, false); err != nil {
		t.Fatalf("WithBody: %v", err)
	}
	if req.Body != "a=1" {
		t.Errorf("Body = %q, want %q", req.Body, "a=1")
	}
	if got := req.Headers["Content-Type"]; got != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %v, want application/x-www-form-urlencoded", got)
	}
}

func TestWithBodyJSONOnPostIsSentVerbatim(t *testing.T) {
	req := NewRequest(http.MethodPost, "http://example.com/")

	if err := req.WithBody(`{"a":1}`, false, false); err != nil {
		t.Fatalf("WithBody: %v", err)
	}
	if req.Body != `{"a":1}` {
		t.Errorf("Body = %q, want %q", req.Body, `{"a":1}`)
	}
}

func TestWithHeadersMerges(t *testing.T) {
	req := NewRequest(http.MethodGet, "http://example.com/")
	req.WithHeadersMap(map[string]string{"X-First": "1"})

	if err := req.WithHeaders(`{"X-Second":"2"}`); err != nil {
		t.Fatalf("WithHeaders: %v", err)
	}

	if req.Headers["X-First"] != "1" {
		t.Error("WithHeaders dropped a previously set header instead of merging")
	}
	if req.Headers["X-Second"] != "2" {
		t.Error("WithHeaders did not add the new header")
	}
}

func TestSendUsesTheGivenMethod(t *testing.T) {
	// Regression: the trace subcommand used to send CONNECT.
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace,
	} {
		t.Run(method, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.Method
			}))
			defer srv.Close()

			req := NewRequest(method, srv.URL)
			if _, err := req.Send(SendOptions{}); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if got != method {
				t.Errorf("server saw %q, want %q", got, method)
			}
		})
	}
}

func TestSendOmitsContentTypeOnBodylessRequest(t *testing.T) {
	var contentType string
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType, seen = r.Header.Get("Content-Type"), true
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !seen {
		t.Fatal("server never received the request")
	}
	if contentType != "" {
		t.Errorf("bodyless GET sent Content-Type %q, want none", contentType)
	}
}

func TestSendSetsContentTypeWhenBodyPresent(t *testing.T) {
	var contentType, body string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()

	req := NewRequest(http.MethodPost, srv.URL)
	if err := req.WithBody(`{"a":1}`, false, false); err != nil {
		t.Fatalf("WithBody: %v", err)
	}
	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if body != `{"a":1}` {
		t.Errorf("body = %q, want %q", body, `{"a":1}`)
	}
}

func TestSendAppliesBasicAuthAndCookies(t *testing.T) {
	var user, pass, cookie string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
		if c, err := r.Cookie("session"); err == nil {
			cookie = c.Value
		}
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	req.BasicAuth.Username = "me"
	req.BasicAuth.Password = "secret"
	req.WithCookie("session", "abc123")

	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !ok || user != "me" || pass != "secret" {
		t.Errorf("basic auth = (%q, %q, %v), want (me, secret, true)", user, pass, ok)
	}
	if cookie != "abc123" {
		t.Errorf("cookie = %q, want abc123", cookie)
	}
}

func TestSendHonoursTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	start := time.Now()
	_, err := req.Send(SendOptions{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Errorf("timeout not enforced: took %v", elapsed)
	}
}

func TestSendDoesNotFollowRedirectsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/moved", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL+"/")
	result, err := req.Send(SendOptions{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want the 302 itself", result.StatusCode)
	}

	req = NewRequest(http.MethodGet, srv.URL+"/")
	result, err = req.Send(SendOptions{Redirect: true})
	if err != nil {
		t.Fatalf("Send with redirect: %v", err)
	}
	if result.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want the followed 418", result.StatusCode)
	}
}

// TLS verification must be on unless the caller opts out.
func TestSendVerifiesTLSByDefault(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	if _, err := req.Send(SendOptions{}); err == nil {
		t.Fatal("self-signed certificate was accepted; verification is not enabled by default")
	}

	req = NewRequest(http.MethodGet, srv.URL)
	if _, err := req.Send(SendOptions{Insecure: true}); err != nil {
		t.Fatalf("--insecure should accept a self-signed certificate, got: %v", err)
	}
}

// Regression guard for WithoutHeader: it must suppress the default even when
// the case used to unset it differs from the case used to set the default.
func TestWithoutHeaderSuppressesDefault(t *testing.T) {
	var got string
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, seen = r.Header.Get("User-Agent"), true
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	req.WithoutHeader("user-agent")

	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !seen {
		t.Fatal("server never received the request")
	}
	if got != "" {
		t.Errorf("User-Agent = %q, want none (suppressed by WithoutHeader)", got)
	}
}

// WithoutHeader must also remove a header explicitly set earlier, not merely
// suppress the default.
func TestWithoutHeaderRemovesAnAlreadySetHeader(t *testing.T) {
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ok = r.Header.Get("X-Token") != ""
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	req.WithHeader("X-Token", "abc")
	req.WithoutHeader("x-token") // different case, must still match

	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ok {
		t.Error("X-Token was sent despite WithoutHeader")
	}
}

// WithHeader must cancel an earlier WithoutHeader for the same key (in any
// case), so that setting a header after unsetting it actually sends it —
// the last operation the caller performed is what happens.
func TestWithHeaderCancelsAnEarlierUnset(t *testing.T) {
	var got string
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, seen = r.Header.Get("X-Token"), true
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	req.WithoutHeader("x-token")
	req.WithHeader("X-Token", "a")

	if _, err := req.Send(SendOptions{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !seen {
		t.Fatal("server never received the request")
	}
	if got != "a" {
		t.Errorf("X-Token = %q, want a (WithHeader must cancel the earlier unset)", got)
	}

	if req.Unset["X-Token"] {
		t.Error("Unset[X-Token] still true after WithHeader; WithHeader must clear it")
	}
}

func TestMergeQueryValuesOverridesSameKeyAndKeepsOthers(t *testing.T) {
	req := NewRequest(http.MethodGet, "http://example.com/?a=1&b=2")

	if err := req.MergeQueryValues(map[string][]string{"b": {"x", "y"}}); err != nil {
		t.Fatalf("MergeQueryValues: %v", err)
	}

	parsed, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse resulting URL: %v", err)
	}
	q := parsed.Query()
	if q.Get("a") != "1" {
		t.Errorf("a = %q, want 1 (untouched key must survive)", q.Get("a"))
	}
	if got := q["b"]; len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("b = %v, want [x y] (override replaces, repeats survive in order)", got)
	}
}

func TestSendRecordsTiming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
	}))
	defer srv.Close()

	req := NewRequest(http.MethodGet, srv.URL)
	result, err := req.Send(SendOptions{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.Timing.Total < 10*time.Millisecond {
		t.Errorf("Timing.Total = %v, want at least the 10ms the handler slept", result.Timing.Total)
	}
}
