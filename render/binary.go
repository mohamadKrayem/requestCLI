package render

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

// sniffLen matches what http.DetectContentType reads.
const sniffLen = 512

// isBinary reports whether a body would wreck the terminal if printed.
//
// Dumping raw bytes for an image or a tarball leaves the terminal in a broken
// state, so we show a notice instead — the same thing HTTPie does.
func isBinary(body []byte, mediaType string) bool {
	if len(body) == 0 {
		return false
	}
	if mediaType != "" {
		return !isTextual(mediaType)
	}

	// No Content-Type at all: sniff. A NUL byte is the strongest single signal,
	// and DetectContentType covers the rest.
	sniff := body
	if len(sniff) > sniffLen {
		sniff = sniff[:sniffLen]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return true
	}
	return !isTextual(mediaTypeOf(http.DetectContentType(sniff)))
}

// isTextual reports whether a media type is meant to be read as text.
func isTextual(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	if isJSON(mediaType) {
		return true
	}
	if strings.HasSuffix(mediaType, "+xml") || strings.HasSuffix(mediaType, "+yaml") {
		return true
	}
	switch mediaType {
	case "application/xml", "application/javascript", "application/x-javascript",
		"application/yaml", "application/x-yaml", "application/toml",
		"application/sql", "application/graphql",
		"application/x-www-form-urlencoded", "application/x-ndjson":
		return true
	}
	return false
}

// mediaTypeOf strips any parameters from a Content-Type value.
func mediaTypeOf(contentType string) string {
	value, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(value))
}

// binaryNotice describes a body instead of printing it.
func binaryNotice(body []byte, mediaType string) string {
	if mediaType == "" {
		sniff := body
		if len(sniff) > sniffLen {
			sniff = sniff[:sniffLen]
		}
		mediaType = mediaTypeOf(http.DetectContentType(sniff))
	}
	return fmt.Sprintf("[binary data: %s, %s — not shown]\n", humanSize(len(body)), mediaType)
}

// humanSize renders a byte count in the units people actually read.
func humanSize(n int) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := int64(n) / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
