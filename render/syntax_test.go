package render

import (
	"strings"
	"testing"
)

func TestLexerFor(t *testing.T) {
	tests := []struct {
		mediaType string
		want      string
		found     bool
	}{
		{"text/html", "html", true},
		{"application/xml", "xml", true},
		{"application/yaml", "yaml", true},
		{"text/x-yaml", "yaml", true},
		{"application/javascript", "javascript", true},
		{"text/css", "css", true},
		{"application/toml", "toml", true},
		{"text/markdown", "markdown", true},
		// Structured-syntax suffix, not in the map by name.
		{"application/atom+xml", "xml", true},
		// JSON never goes through chroma; it has a lossless fast path.
		{"application/json", "", false},
		{"text/plain", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got, ok := lexerFor(tt.mediaType)
		if ok != tt.found || got != tt.want {
			t.Errorf("lexerFor(%q) = (%q, %v), want (%q, %v)", tt.mediaType, got, ok, tt.want, tt.found)
		}
	}
}

func TestIsJSON(t *testing.T) {
	yes := []string{"application/json", "text/json", "application/problem+json", "application/vnd.api+json"}
	no := []string{"text/plain", "application/xml", "", "application/jsonish"}

	for _, mediaType := range yes {
		if !isJSON(mediaType) {
			t.Errorf("isJSON(%q) = false, want true", mediaType)
		}
	}
	for _, mediaType := range no {
		if isJSON(mediaType) {
			t.Errorf("isJSON(%q) = true, want false", mediaType)
		}
	}
}

// An unknown chroma style must not fail the command; highlighting is cosmetic.
func TestHighlightFallsBackOnBadStyle(t *testing.T) {
	body := []byte("<p>hi</p>")
	got := highlight(body, "html", Options{Color: true, Style: "no-such-style-exists"})

	if !strings.Contains(got, "hi") {
		t.Errorf("output %q lost the body", got)
	}
}
