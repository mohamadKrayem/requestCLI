package render

import (
	"strings"
	"testing"
)

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		mediaType string
		want      bool
	}{
		{name: "png by media type", body: []byte{0x89, 'P', 'N', 'G'}, mediaType: "image/png", want: true},
		{name: "octet-stream", body: []byte("anything"), mediaType: "application/octet-stream", want: true},
		{name: "pdf", body: []byte("%PDF-1.4"), mediaType: "application/pdf", want: true},
		{name: "plain text", body: []byte("hello"), mediaType: "text/plain", want: false},
		{name: "json", body: []byte(`{"a":1}`), mediaType: "application/json", want: false},
		{name: "problem+json", body: []byte(`{"a":1}`), mediaType: "application/problem+json", want: false},
		{name: "xml", body: []byte("<a/>"), mediaType: "application/xml", want: false},
		{name: "form encoded", body: []byte("a=1"), mediaType: "application/x-www-form-urlencoded", want: false},
		// No Content-Type at all: fall back to sniffing.
		{name: "sniffed text", body: []byte("plain words here"), want: false},
		{name: "sniffed nul bytes", body: []byte{'a', 0x00, 'b'}, want: true},
		{name: "empty body", body: nil, mediaType: "image/png", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinary(tt.body, tt.mediaType); got != tt.want {
				t.Errorf("isBinary(%q, %q) = %v, want %v", tt.body, tt.mediaType, got, tt.want)
			}
		})
	}
}

// Regression: a binary body was dumped raw and wrecked the terminal.
func TestBinaryBodyRendersANoticeInsteadOfBytes(t *testing.T) {
	body := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 2000)...)
	got := Render(result("image/png", string(body)), Options{ShowBody: true})

	if !strings.Contains(got, "binary data") {
		t.Errorf("output %q should describe the body, not print it", got)
	}
	if !strings.Contains(got, "image/png") {
		t.Errorf("output %q should name the media type", got)
	}
	if strings.Contains(got, "\x00") {
		t.Error("raw NUL bytes reached the output")
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{12400, "12.4 kB"},
		{2_500_000, "2.5 MB"},
	}

	for _, tt := range tests {
		if got := humanSize(tt.in); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
