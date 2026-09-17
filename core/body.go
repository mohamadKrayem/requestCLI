package core

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mohamadkrayem/requestCLI/formats"
	"github.com/mohamadkrayem/requestCLI/input"
)

// SendsBodyInQuery reports whether a method carries its data as query
// parameters rather than as a request body. command consults this directly to
// route body-carrying request items the same way -b already is on these
// verbs.
func SendsBodyInQuery(method string) bool {
	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodHead,
		http.MethodTrace, http.MethodOptions, http.MethodConnect:
		return true
	}
	return false
}

// WithBody attaches the body to the request according to the form/multipart flags.
func (req *BaseRequest) WithBody(body string, form, multipartForm bool) error {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}

	switch {
	case form:
		mapBody, err := formats.ToMapOptionalJS(body)
		if err != nil {
			return fmt.Errorf("--form needs a json object as the body: %w", err)
		}
		if SendsBodyInQuery(req.Method) {
			return req.AddQueryString(mapBody)
		}
		req.Body = GenerateQueryParams(mapBody)
		req.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		return nil

	case multipartForm:
		multipartInput, err := input.NewMultipartInputInJSONFormat(body)
		if err != nil {
			return err
		}
		req.MultipartBody = multipartInput.Body
		req.Writer = multipartInput.Writer
		req.Headers["Content-Type"] = multipartInput.Writer.FormDataContentType()
		return nil

	default:
		if SendsBodyInQuery(req.Method) {
			mapBody, err := formats.ToMapOptionalJS(body)
			if err != nil {
				// Not JSON, so it cannot become query params: send it verbatim.
				req.Body = body
				req.setIfAbsent("Content-Type", "text/plain")
				return nil
			}
			return req.AddQueryString(mapBody)
		}
		req.Body = body
		// The same downgrade, for the verbs that carry a body. Without it
		// addDefaultHeaders fills in application/json for anything with a
		// body, so `rq post url -b hello` and a piped CSV both announced
		// themselves as JSON — the client asserting a payload type that is
		// false. The check is on the bytes, not on how they were supplied,
		// because -b, --body and piped stdin all arrive here.
		if body != "" && !json.Valid([]byte(body)) {
			req.setIfAbsent("Content-Type", "text/plain")
		}
		return nil
	}
}
