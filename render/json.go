package render

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/pretty"
)

// jsonStyle keeps the palette the tool has always used.
var jsonStyle = &pretty.Style{
	Key:      [2]string{"\x1b[94m", ansiReset}, // bright blue
	String:   [2]string{"\x1b[92m", ansiReset}, // bright green
	Number:   [2]string{"\x1b[33m", ansiReset}, // yellow
	True:     [2]string{ansiCyan, ansiReset},
	False:    [2]string{ansiCyan, ansiReset},
	Null:     [2]string{"\x1b[32m", ansiReset}, // green
	Escape:   [2]string{"\x1b[92m", ansiReset},
	Brackets: [2]string{"", ""},
}

var jsonIndent = &pretty.Options{
	Width:    80,
	Indent:   "  ",
	SortKeys: false, // never reorder what the server sent
}

// isJSON reports whether a media type carries a JSON payload.
//
// The "+json" suffix matters: application/problem+json and
// application/vnd.api+json are both common and both JSON.
func isJSON(mediaType string) bool {
	return mediaType == "application/json" ||
		mediaType == "text/json" ||
		strings.HasSuffix(mediaType, "+json")
}

// renderJSON formats and colorizes a JSON payload without ever decoding it.
//
// It operates on the raw bytes, so number precision, key order and duplicate
// keys survive exactly as the server sent them. Decoding into map[string]any
// and re-encoding — the previous approach — silently destroyed all three.
//
// It reports false when the payload is not valid JSON, so the caller can fall
// back to showing it raw.
func renderJSON(body []byte, color bool) (string, bool) {
	if !json.Valid(body) {
		return "", false
	}

	out := pretty.PrettyOptions(body, jsonIndent)
	if color {
		out = pretty.Color(out, jsonStyle)
	}
	return string(out), true
}
