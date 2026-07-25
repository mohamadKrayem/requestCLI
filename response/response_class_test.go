package response

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newResponse runs a handler and returns the raw *http.Response for rendering.
func newResponse(t *testing.T, handler http.HandlerFunc) *http.Response {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("requesting test server: %v", err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestNewResponseShowsEverythingWhenNoFlagsGiven(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Marker", "here")
		w.Write([]byte("payload"))
	})

	got, err := NewResponse(res, false, false, false)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	if got.Status == "" {
		t.Error("status missing")
	}
	if !strings.Contains(got.Headers, "X-Marker") {
		t.Error("headers missing")
	}
	if !strings.Contains(got.Body, "payload") {
		t.Error("body missing")
	}
}

// The three print flags are a selection set, so they combine.
func TestNewResponseCombinesSelectionFlags(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Marker", "here")
		w.Write([]byte("payload"))
	})

	got, err := NewResponse(res, false, true, true)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	if got.Status != "" {
		t.Error("status was not requested but is present")
	}
	if !strings.Contains(got.Headers, "X-Marker") {
		t.Error("headers were requested but are missing")
	}
	if !strings.Contains(got.Body, "payload") {
		t.Error("body was requested but is missing")
	}
}

func TestNewResponseStatusOnly(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("payload"))
	})

	got, err := NewResponse(res, true, false, false)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	if got.Status == "" {
		t.Error("status missing")
	}
	if got.Headers != "" || got.Body != "" {
		t.Errorf("only the status was requested, got headers=%q body=%q", got.Headers, got.Body)
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
			res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Content-Encoding", tt.encoding)
				w.Write(body)
			})

			got, err := NewResponse(res, false, false, true)
			if err != nil {
				t.Fatalf("NewResponse: %v", err)
			}
			if !strings.Contains(got.Body, payload) {
				t.Errorf("decoded body = %q, want it to contain %q", got.Body, payload)
			}
		})
	}
}

// A Content-Type of application/json with an unparseable payload should still
// render, rather than failing the whole command.
func TestInvalidJSONBodyFallsBackToRaw(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("this is not json"))
	})

	got, err := NewResponse(res, false, false, true)
	if err != nil {
		t.Fatalf("NewResponse should not fail on a malformed json body: %v", err)
	}
	if !strings.Contains(got.Body, "this is not json") {
		t.Errorf("body = %q, want the raw payload", got.Body)
	}
}

func TestEmptyBodyRendersEmpty(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	got, err := NewResponse(res, false, false, true)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	if got.Body != "" {
		t.Errorf("body = %q, want empty", got.Body)
	}
}

func TestRepeatedHeadersAreAllRendered(t *testing.T) {
	res := newResponse(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "one")
		w.Header().Add("X-Multi", "two")
		w.WriteHeader(http.StatusOK)
	})

	got, err := NewResponse(res, false, true, false)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	for _, want := range []string{"one", "two"} {
		if !strings.Contains(got.Headers, want) {
			t.Errorf("headers %q missing value %q", got.Headers, want)
		}
	}
}
