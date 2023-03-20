package methods

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/alecthomas/chroma/quick"
	"github.com/fatih/color"
	"github.com/mattn/go-colorable"
	"github.com/neilotoole/jsoncolor"
	"io"
	"net/http"
	urlLib "net/url"
	"strings"
)

func GetLinkToPrint(url string) (string, error) {
	res, err := MakeGetRequest(url)
	if err != nil {
		return "", err
	}

	stringToBePrinted, err := StoreResponseHeaders(res)
	if err != nil {
		return "", err
	}

	contentType := res.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		htmlSTR, err := GetColorizedHTML(res)
		if err != nil {
			return "", err
		}
		stringToBePrinted += htmlSTR
	} else if strings.Contains(contentType, "text/parse") {
		body, err := ReadResponseBody(res)
		if err != nil {
			return "", err
		}

		stringToBePrinted += string(body)
	} else if strings.Contains(contentType, "application/json") {
		body, err := ReadResponseBody(res)
		if err != nil {
			return "", err
		}

		jsonSTR, err := GetColorizedJSON(body)
		if err != nil {
			return "", err
		}
		stringToBePrinted += jsonSTR
	}
	return stringToBePrinted, nil
}

func MakeGetRequest(url string) (*http.Response, error) {
	reqURL, err := urlLib.Parse(url)
	if err != nil {
		return nil, err
	}

	host := reqURL.Hostname()

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "/")
	req.Header.Set("Host", host)
	req.Header.Set("User-Agent", "requestCLI/1.0")

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func ReadResponseBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	return body, nil
}

func StoreResponseHeaders(res *http.Response) (string, error) {
	headers := res.Header
	keyColor := color.New(color.FgCyan).SprintFunc()
	valColor := color.New(color.FgHiWhite).SprintFunc()
	statusColor := color.New(color.FgHiBlue).SprintFunc()
	protoColor := color.New(color.FgHiCyan).SprintFunc()

	resSTR := fmt.Sprintf("\n%s %s\n", protoColor(res.Proto), statusColor(res.Status))

	for key, val := range headers {
		resSTR += fmt.Sprintf("%s:   %s\n", keyColor(key), valColor(fmt.Sprintf("%v", val[0])))
	}
	resSTR += "\n"
	return resSTR, nil
}

func GetColorizedHTML(res *http.Response) (string, error) {
	bodyBYTES, err := ReadResponseBody(res)
	if err != nil {
		return "", err
	}

	// Colorize the HTML code
	var buf bytes.Buffer
	quick.Highlight(&buf, string(bodyBYTES), "html", "terminal", "monokai")

	// Print the colorized HTML code to the console
	return buf.String(), nil
}

func GetColorizedJSON(jsonData []byte) (string, error) {
	var buf bytes.Buffer

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
	var m any
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return "", err
	}

	enc.SetIndent("", "  ")

	// Encode the JSON data to the buffer
	if err := enc.Encode(m); err != nil {
		return "", err
	}

	return buf.String(), nil
}
