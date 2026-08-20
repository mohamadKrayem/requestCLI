package core

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fetch runs a handler and returns the Result of a GET against it.
func fetch(t *testing.T, handler http.HandlerFunc) *Result {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req := NewRequest(http.MethodGet, srv.URL)
	result, err := req.Send(SendOptions{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	return result
}

func TestResultCarriesStructuredData(t *testing.T) {
	result := fetch(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Marker", "here")
		w.Write([]byte("payload"))
	})

	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.Status == "" || result.Proto == "" {
		t.Errorf("Status/Proto missing: %q %q", result.Status, result.Proto)
	}
	// Headers must stay real headers, not a pre-rendered string: a TUI or a
	// diff tool cannot work with text that has already been formatted.
	if got := result.Headers.Get("X-Marker"); got != "here" {
		t.Errorf("Headers.Get(X-Marker) = %q, want here", got)
	}
	if string(result.Body) != "payload" {
		t.Errorf("Body = %q, want payload", result.Body)
	}
}

func TestMediaTypeStripsParameters(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"application/json; charset=utf-8", "application/json"},
		{"TEXT/HTML", "text/html"},
		{"application/json", "application/json"},
		{"", ""},
		// A malformed header should degrade, not blow up.
		{"application/json;;;", "application/json"},
	}

	for _, tt := range tests {
		result := &Result{Headers: http.Header{}}
		if tt.header != "" {
			result.Headers.Set("Content-Type", tt.header)
		}
		if got := result.MediaType(); got != tt.want {
			t.Errorf("MediaType(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestDecodesCompressedBodies(t *testing.T) {
	const payload = "the quick brown fox"

	tests := []struct {
		name     string
		encoding string
		compress func(*testing.T, string) []byte
	}{
		{
			name:     "gzip",
			encoding: "gzip",
			compress: func(t *testing.T, s string) []byte {
				var buf bytes.Buffer
				zw := gzip.NewWriter(&buf)
				if _, err := zw.Write([]byte(s)); err != nil {
					t.Fatalf("gzip write: %v", err)
				}
				if err := zw.Close(); err != nil {
					t.Fatalf("gzip close: %v", err)
				}
				return buf.Bytes()
			},
		},
		{
			name:     "deflate",
			encoding: "deflate",
			compress: func(t *testing.T, s string) []byte {
				var buf bytes.Buffer
				zw, err := flate.NewWriter(&buf, flate.DefaultCompression)
				if err != nil {
					t.Fatalf("flate writer: %v", err)
				}
				if _, err := zw.Write([]byte(s)); err != nil {
					t.Fatalf("flate write: %v", err)
				}
				if err := zw.Close(); err != nil {
					t.Fatalf("flate close: %v", err)
				}
				return buf.Bytes()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.compress(t, payload)
			result := fetch(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Content-Encoding", tt.encoding)
				w.Write(body)
			})

			if string(result.Body) != payload {
				t.Errorf("decoded body = %q, want %q", result.Body, payload)
			}
		})
	}
}

func TestEmptyBodyIsEmpty(t *testing.T) {
	result := fetch(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if len(result.Body) != 0 {
		t.Errorf("Body = %q, want empty", result.Body)
	}
}

func TestRepeatedHeadersAreKept(t *testing.T) {
	result := fetch(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("X-Multi", "one")
		w.Header().Add("X-Multi", "two")
		w.WriteHeader(http.StatusOK)
	})

	if got := result.Headers.Values("X-Multi"); len(got) != 2 {
		t.Errorf("X-Multi = %v, want both values", got)
	}
}
