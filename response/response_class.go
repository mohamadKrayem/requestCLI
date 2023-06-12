package response

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"

	"github.com/alecthomas/chroma/quick"
	"github.com/dsnet/compress/brotli"
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

// NewResponse creates a new Response object
func NewResponse(httpRes *http.Response, showStatus, showHeaders, showBody bool) Response {
	var response Response

	// how the user wants to show all the response
	if showStatus {
		response.Proto = httpRes.Proto
		response.Status = httpRes.Status
	} else if showHeaders {
		response.Headers = storeColorizedHeaders(httpRes)

		//
		//!!!!!!!!!!!!!!!!!!!!!!!! for testing purposes only !!!!!!!!!!!!!!!!!
		/*
			for key, val := range httpRes.Header {
				fmt.Printf("%s:   %s\n", key, fmt.Sprintf("%v", val[0]))
			}*/
		//!!!!!!!!!!!!!!!!!!!!!!!! for testing purposes only !!!!!!!!!!!!!!!!!
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

// PrintResponse prints the response to the console
func (res *Response) PrintResponse() {
	statusColor := color.New(color.FgHiBlue).SprintFunc()
	protoColor := color.New(color.FgHiCyan).SprintFunc()

	resSTR := fmt.Sprintf("\n%s %s\n", protoColor(res.Proto), statusColor(res.Status))
	resSTR += res.Headers
	resSTR += res.Body
	fmt.Println(resSTR)
}

// storeColorizedHeaders stores the headers in a colorized way
func storeColorizedHeaders(res *http.Response) string {
	headers := res.Header
	keyColor := color.New(color.FgCyan).SprintFunc()
	valColor := color.New(color.FgHiWhite).SprintFunc()

	var resSTR string
	for key, val := range headers {
		if len(val) > 1 {
			for _, v := range val {
				resSTR += fmt.Sprintf("%s:   %s\n", keyColor(key), valColor(v))
			}
		} else {
			resSTR += fmt.Sprintf("%s:   %s\n", keyColor(key), valColor(fmt.Sprintf("%v", val[0])))
		}
	}
	resSTR += "\n"
	return resSTR
}

// storeColorizedBody stores the body in a colorized way
func storeColorizedBody(res *http.Response) (string, error) {
	var stringToBePrinted string

	if res.Body == nil {
		return "", nil
	}

	// check the content type of the response body first to know how to colorize it
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

// storeColorizedBodyAsJSON stores the json data of the body in a colorized way
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

// readResponseBody reads the response body
func readResponseBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()

	var contentEncoding string = res.Header.Get("Content-Encoding")

	if contentEncoding == "gzip" {
		// Create a gzip reader to decompress the response body
		reader, err := gzip.NewReader(res.Body)
		if err != nil {
			log.Fatal(err)
		}
		defer reader.Close()

		// Read the decompressed data
		decompressedData, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatal("error decoding gzip response", err)
		}
		return decompressedData, nil
	} else if contentEncoding == "deflate" {
		compressedData, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Fatal(err)
		}

		reader := flate.NewReader(bytes.NewReader(compressedData))

		decompressedData, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatal("error decoding deflate response", err)
		}

		defer reader.Close()
		return decompressedData, nil

	} else if contentEncoding == "br" {
		// Create a br reader to decompress the response body
		//create a conf for the reader
		conf := brotli.ReaderConfig{}
		reader, err := brotli.NewReader(res.Body, &conf)
		if err != nil {
			log.Fatal(err)
		}
		defer reader.Close()
		resBody, err := io.ReadAll(reader)
		if err != nil {
			log.Fatal("error decoding br response", err)
		}
		return resBody, nil
	} else {

		body, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Print(err)
			return nil, err
		}

		return body, nil
	}
}

// getColorizedHTML colorizes the HTML code
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
