// Package formats validates and normalizes JSON supplied on the command line.
//
// It is an input-side package only. Rendering a response never comes through
// here: decoding a payload in order to display it is what destroyed number
// precision, key order and duplicate keys. See render/json.go.
package formats

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// JSON holds a JSON document as a string.
type JSON string

// NewJSON validates a JSON document and compacts it onto a single line.
//
// It uses json.Compact rather than decoding and re-encoding, so number literals
// and key order are preserved exactly as typed. A large integer written by hand
// reaches the server unchanged instead of being rounded through float64.
func NewJSON(jsonInput string) (JSON, error) {
	trimmed := strings.TrimSpace(jsonInput)
	if trimmed == "" {
		return "", errors.New("empty json input")
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return "", fmt.Errorf("parsing json: %w", err)
	}
	return JSON(buf.String()), nil
}

// ToJSON marshals a string map into a JSON value.
func ToJSON(jsonAsMap map[string]string) (JSON, error) {
	jsonString, err := json.Marshal(jsonAsMap)
	if err != nil {
		return "", fmt.Errorf("encoding map as json: %w", err)
	}
	return NewJSON(string(jsonString))
}

// ToMap decodes the document into a map.
func (js *JSON) ToMap() (map[string]any, error) {
	return ToMapOptionalJS(string(*js))
}

// ToMapOptionalJS decodes a JSON object into a map.
//
// This is for values that genuinely have to become Go data — header names,
// form fields, query parameters — never for display.
func ToMapOptionalJS(js string) (map[string]any, error) {
	var jsonMap map[string]any
	if err := json.Unmarshal([]byte(js), &jsonMap); err != nil {
		return nil, fmt.Errorf("parsing json object: %w", err)
	}
	return jsonMap, nil
}
