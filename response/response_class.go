package response

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/chroma/quick"
	"github.com/fatih/color"
	js "github.com/mohamadkrayem/requestCLI/formats"

	"net/http"
)

type Response struct {
	Proto   string
	Status  string
	Headers string
	Body    string
}

func NewResponse(httpRes *http.Response, showStatus, showHeaders, showBody bool) Response {
	var response Response
	if showStatus {
		response.Proto = httpRes.Proto
		response.Status = httpRes.Status
	} else if showHeaders {
		response.Headers = storeColorizedHeaders(httpRes)
	} else if showBody {
		response.Body, _ = storeColorizedBody(httpRes)
	} else {
		response = Response{
			Proto:  httpRes.Proto,
			Status: httpRes.Status,
		}
		response.Headers = storeColorizedHeaders(httpRes)
		response.Body, _ = storeColorizedBody(httpRes)
	}
	return response
}

func (res *Response) PrintResponse() {
	statusColor := color.New(color.FgHiBlue).SprintFunc()
	protoColor := color.New(color.FgHiCyan).SprintFunc()

	resSTR := fmt.Sprintf("\n%s %s\n", protoColor(res.Proto), statusColor(res.Status))
	resSTR += res.Headers
	resSTR += res.Body
	fmt.Println(resSTR)
}

func storeColorizedHeaders(res *http.Response) string {
	headers := res.Header
	keyColor := color.New(color.FgCyan).SprintFunc()
	valColor := color.New(color.FgHiWhite).SprintFunc()

	var resSTR string
	for key, val := range headers {
		resSTR += fmt.Sprintf("%s:   %s\n", keyColor(key), valColor(fmt.Sprintf("%v", val[0])))
	}
	resSTR += "\n"
	return resSTR
}

func storeColorizedBody(res *http.Response) (string, error) {
	var stringToBePrinted string

	contentType := res.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		htmlSTR, err := getColorizedHTML(res)
		if err != nil {
			return "", err
		}
		stringToBePrinted = htmlSTR
	} else if strings.Contains(contentType, "application/json") {
		resJS, err := storeColorizedBodyAsJSON(res)
		if err != nil {
			return "", err
		}
		stringToBePrinted = resJS
	} else {
		body, err := readResponseBody(res)
		if err != nil {
			return "", err
		}

		stringToBePrinted = string(body)
	}
	return stringToBePrinted, nil
}

func storeColorizedBodyAsJSON(res *http.Response) (string, error) {
	resBody, _ := readResponseBody(res)
	resJS, err := js.NewJson(string(resBody))
	if err != nil {
		return "", err
	}
	resStr, err := resJS.GetColorizedJSON()
	if err != nil {
		return "", err
	}

	return resStr, nil
}

func readResponseBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	return body, nil
}

func getColorizedHTML(res *http.Response) (string, error) {
	bodyBYTES, err := readResponseBody(res)
	if err != nil {
		return "", err
	}

	// Colorize the HTML code
	var buf bytes.Buffer
	quick.Highlight(&buf, string(bodyBYTES), "html", "terminal", "monokai")

	// Print the colorized HTML code to the console
	return buf.String(), nil
}
