// Package formats handles parsing, normalizing and colorizing JSON documents.
package formats

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/neilotoole/jsoncolor"
)

// Json holds a JSON document as a string.
type Json string

// NewJson validates and normalizes a JSON document.
func NewJson(jsonInput string) (Json, error) {
	return removeNewLinesFromJSONString(jsonInput)
}

// ToJSON marshals a string map into a Json value.
func ToJSON(jsonAsMap map[string]string) (Json, error) {
	jsonString, err := json.Marshal(jsonAsMap)
	if err != nil {
		return "", fmt.Errorf("encoding map as json: %w", err)
	}
	return NewJson(string(jsonString))
}

// ToMap decodes the document into a map.
func (js *Json) ToMap() (map[string]any, error) {
	return ToMapOptionalJS(string(*js))
}

// ToMapOptionalJS decodes a JSON object into a map.
func ToMapOptionalJS(js string) (map[string]any, error) {
	var jsonMap map[string]any
	if err := json.Unmarshal([]byte(js), &jsonMap); err != nil {
		return nil, fmt.Errorf("parsing json object: %w", err)
	}
	return jsonMap, nil
}

// GetColorizedJSON renders the document indented, and colorized when stdout is a terminal.
func (js *Json) GetColorizedJSON() (string, error) {
	var buf bytes.Buffer
	jsonData := string(*js)

	enc := jsoncolor.NewEncoder(&buf)

	// Only colorize when stdout is a color terminal, so piped output stays clean.
	if jsoncolor.IsColorTerminal(colorable.NewColorableStdout()) {
		enc.SetColors(&jsoncolor.Colors{
			Null:   jsoncolor.Color("\x1b[32m"), // green
			Bool:   jsoncolor.Color("\x1b[36m"), // cyan
			String: jsoncolor.Color("\x1b[92m"), // bright green
			Number: jsoncolor.Color("\x1b[33m"), // yellow
			Key:    jsoncolor.Color("\x1b[94m"), // bright blue
		})
	}
	enc.SetIndent("", "  ")

	decoded, err := decode(jsonData)
	if err != nil {
		return "", err
	}

	if err := enc.Encode(decoded); err != nil {
		return "", fmt.Errorf("encoding json for display: %w", err)
	}
	return buf.String(), nil
}

// decode parses a document as either an array or an object, whichever it opens with.
func decode(jsonStr string) (any, error) {
	if isArray(jsonStr) {
		return toArrayOfMaps(jsonStr)
	}
	return ToMapOptionalJS(jsonStr)
}

func toArrayOfMaps(js string) ([]any, error) {
	var arrayOfMaps []any
	if err := json.Unmarshal([]byte(js), &arrayOfMaps); err != nil {
		return nil, fmt.Errorf("parsing json array: %w", err)
	}
	return arrayOfMaps, nil
}

// isArray reports whether the document's first non-space character opens an array.
func isArray(js string) bool {
	trimmed := strings.TrimSpace(js)
	return len(trimmed) > 0 && trimmed[0] == '['
}

func removeNewLinesFromJSONString(jsonStr string) (Json, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return "", errors.New("empty json input")
	}

	decoded, err := decode(jsonStr)
	if err != nil {
		return "", err
	}

	modifiedJSONStr, err := json.Marshal(removeNewLines(decoded))
	if err != nil {
		return "", fmt.Errorf("re-encoding json: %w", err)
	}
	return Json(modifiedJSONStr), nil
}

// removeNewLines strips newlines from every string value in the tree.
//
// It returns the cleaned value rather than mutating in place: a string reached
// through an `any` is a copy, so assigning to the loop variable would be a no-op.
func removeNewLines(jsonObj any) any {
	switch val := jsonObj.(type) {
	case string:
		return strings.ReplaceAll(val, "\n", "")
	case map[string]any:
		for k, v := range val {
			val[k] = removeNewLines(v)
		}
		return val
	case []any:
		for i, v := range val {
			val[i] = removeNewLines(v)
		}
		return val
	default:
		return jsonObj
	}
}
