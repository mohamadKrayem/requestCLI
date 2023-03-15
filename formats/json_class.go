package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mattn/go-colorable"
	"github.com/neilotoole/jsoncolor"
	"strings"
)

type Json string

func NewJson(jsonInput string) (Json, error) {
	jsonString, err := removeNewlinesFromJSONString(jsonInput)
	if err != nil {
		return Json(""), err
	}

	return jsonString, nil
}

func (js *Json) ToMap() (map[string]interface{}, error) {
	var jsonMap map[string]interface{}

	if err := json.Unmarshal([]byte(*js), &jsonMap); err != nil {
		fmt.Println(err)
		return nil, err
	}

	return jsonMap, nil
}

func (js *Json) GetColorizedJSON() (string, error) {
	var buf bytes.Buffer
	var jsonData []byte = []byte(*js)

	// Create a new encoder that writes to the buffer
	enc := jsoncolor.NewEncoder(&buf)

	// Check if stdout is a color terminal
	if jsoncolor.IsColorTerminal(colorable.NewColorableStdout()) {
		// Set the colors for the encoder
		clrs := &jsoncolor.Colors{
			Null:   jsoncolor.Color("\x1b[32m"), // Green
			Bool:   jsoncolor.Color("\x1b[36m"), // Cyan
			String: jsoncolor.Color("\x1b[92m"), // Magenta
			Number: jsoncolor.Color("\x1b[33m"), // Yellow
			Key:    jsoncolor.Color("\x1b[94m"), // Red
		}
		// Apply the colors to the encoder
		enc.SetColors(clrs)
	}

	// Unmarshal the JSON data
	jsonMap, err := toMapOptionalJS(string(jsonData))
	if err != nil {
		return "", err
	}

	enc.SetIndent("", "  ")

	// Encode the JSON data to the buffer
	if err := enc.Encode(jsonMap); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func toMapOptionalJS(js string) (map[string]interface{}, error) {
	var jsonMap map[string]interface{}

	if err := json.Unmarshal([]byte(js), &jsonMap); err != nil {
		fmt.Println(err)
		return nil, err
	}

	return jsonMap, nil
}

func removeNewlinesFromJSONString(jsonStr string) (Json, error) {
	// parse the JSON string into an interface{}
	jsonMap, err := toMapOptionalJS(jsonStr)
	if err != nil {
		return "", err
	}

	// remove newline characters from all string values recursively
	removeNewlinesRecursively(jsonMap)

	// encode the modified JSON object back into a string
	modifiedJSONStr, err := json.Marshal(jsonMap)
	if err != nil {
		return "", err
	}

	return Json(modifiedJSONStr), nil
}

func removeNewlinesRecursively(jsonObj interface{}) {
	switch val := jsonObj.(type) {
	case string:
		// replace all newline characters in string values
		jsonObj = strings.ReplaceAll(val, "\n", "")
	case map[string]interface{}:
		// traverse map values recursively
		for _, v := range val {
			removeNewlinesRecursively(v)
		}
	case []interface{}:
		// traverse array values recursively
		for _, v := range val {
			removeNewlinesRecursively(v)
		}
	}
}
