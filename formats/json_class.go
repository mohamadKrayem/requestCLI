package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/neilotoole/jsoncolor"
)

type Json string

func NewJson(jsonInput string) (Json, error) {
	jsonString, err := removeNewLinesFromJSONString(jsonInput)
	if err != nil {
		return Json(""), err
	}

	return jsonString, nil
}

func ToJSONStr(JsonAsMap map[string]string) (string, error) {
	JsonString, err := json.Marshal(JsonAsMap)
	if err != nil {
		log.Fatal("error with your json !!!")
	}
	return string(JsonString), nil
}

func ToJSON(JsonAsMap map[string]string) (Json, error) {
	JsonString, err := json.Marshal(JsonAsMap)
	if err != nil {
		log.Fatal("Error with your json !!!")
	}
	var JsonJS Json
	JsonJS, _ = NewJson(string(JsonString))
	return JsonJS, nil
}

func (js *Json) ToMap() (map[string]any, error) {
	var jsonMap map[string]any

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

	if isArray(string(jsonData)) {
		return encodeArrayOfMaps(jsonData, *enc, &buf)
	} else {
		return encodeMaps(jsonData, *enc, &buf)
	}

}

func encodeMaps(jsonData []byte, enc jsoncolor.Encoder, buf *bytes.Buffer) (string, error) {
	var jsonMap map[string]any
	// Unmarshal the JSON data
	jsonMap, err := ToMapOptionalJS(string(jsonData))
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

func encodeArrayOfMaps(jsonData []byte, enc jsoncolor.Encoder, buf *bytes.Buffer) (string, error) {

	var jsonArray []any
	// Unmarshal the JSON data
	jsonArray, err := toArrayOfMaps(string(jsonData))
	if err != nil {
		return "", err
	}

	enc.SetIndent("", "  ")

	// Encode the JSON data to the buffer
	if err := enc.Encode(jsonArray); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func ToMapOptionalJS(js string) (map[string]any, error) {
	var jsonMap map[string]any

	if err := json.Unmarshal([]byte(js), &jsonMap); err != nil {
		log.Fatal("Error in your json format")
		return nil, err
	}

	return jsonMap, nil
}

func toArrayOfMaps(js string) ([]any, error) {
	var arrayOfMaps []any
	if err := json.Unmarshal([]byte(js), &arrayOfMaps); err != nil {
		return nil, err
	}

	return arrayOfMaps, nil
}

func isArray(js string) bool {
	if string(js[0]) == "[" {
		return true
	}
	return false
}

func removeNewLinesFromJSONString(jsonStr string) (Json, error) {
	// parse the JSON string into an interface{}
	if string(jsonStr[0]) == "[" {
		var jsonArrayOfMaps []any
		jsonArrayOfMaps, err := toArrayOfMaps(jsonStr)
		_ = jsonArrayOfMaps
		if err != nil {
			return "", err
		}
		var modifiedJSONStr []byte
		// remove newline characters from all string values recursively
		removeNewLinesRecursively(jsonArrayOfMaps)
		// encode the modified JSON object back into a string
		modifiedJSONStr, err = json.Marshal(jsonArrayOfMaps)
		_ = modifiedJSONStr
		if err != nil {
			return "", err
		}
		return Json(modifiedJSONStr), nil
	} else {
		var jsonMap map[string]any
		jsonMap, err := ToMapOptionalJS(jsonStr)
		if err != nil {
			return "", err
		}

		var modifiedJSONStr []byte

		// remove newline characters from all string values recursively
		removeNewLinesRecursively(jsonMap)
		// encode the modified JSON object back into a string
		modifiedJSONStr, err = json.Marshal(jsonMap)
		if err != nil {
			return "", err
		}

		//	if err != nil {
		//	}

		return Json(modifiedJSONStr), nil
	}
}

func removeNewLinesRecursively(jsonObj any) {
	switch val := jsonObj.(type) {
	case string:
		// replace all newline characters in string values
		jsonObj = strings.ReplaceAll(val, "\n", "")
	case map[string]any:
		// traverse map values recursively
		for _, v := range val {
			removeNewLinesRecursively(v)
		}
	case []any:
		// traverse array values recursively
		for _, v := range val {
			removeNewLinesRecursively(v)
		}
	}
}

func IsJson(data string) (bool, error) {
	var jsonData any
	err := json.Unmarshal([]byte(data), &jsonData)
	if err != nil {
		fmt.Println(err)
		return false, err
	} else {
		return true, nil
	}

}
