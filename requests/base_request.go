package requests

import (
	//"crypto/tls"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	js "github.com/mohamadkrayem/requestCLI/formats"
	"github.com/mohamadkrayem/requestCLI/input"
	rs "github.com/mohamadkrayem/requestCLI/response"
)

// BaseRequest is the base request object
type BaseRequest struct {
	Method        string
	URL           string
	Headers       map[string]any
	Cookies       map[string]string
	Body          string
	BasicAuth     auth.BaseAuth
	MultipartBody io.Reader
	Writer        *multipart.Writer
}

// NewBaseRequest creates a new BaseRequest object
func NewRequest(method, url string) BaseRequest {
	return BaseRequest{
		Method:        method,
		URL:           url,
		Headers:       make(map[string]any),
		Cookies:       make(map[string]string),
		Body:          "",
		MultipartBody: nil,
		Writer:        nil,
	}
}

// WithHeaders adds single header to the request
func (req *BaseRequest) WithHeader(key string, value string) *BaseRequest {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}

	req.Headers[key] = value
	return req
}

// GenerateURL builds the URL for the request
func GenerateUrl(reqURL string, securityFlag bool, queryParams map[string]string) string {
	existHTTPS := strings.Contains(reqURL, "https://")
	existHTTP := strings.Contains(reqURL, "http://")
	if securityFlag {
		if !existHTTPS && !existHTTP {
			reqURL = "https://" + reqURL
		}
	} else {
		if !existHTTPS && !existHTTP {
			reqURL = "http://" + reqURL
		}
	}
	if queryParams != nil {
		queryString := GenerateQueryParams(queryParams)
		reqURL += "?" + queryString
	}
	return reqURL
}

// AddQueryParams adds a query string to the request url.
func (req *BaseRequest) AddQueryString(queryParams map[string]any) {
	req.URL += "?" + GenerateQueryParams(queryParams)
}

// GenerateQueryParamsForStrings generates query params of map[string]string type.
func GenerateQueryParamsForStrings(queryParams map[string]string) string {
	query := url.Values{}
	for key, value := range queryParams {
		query.Set(key, value)
	}
	queryString := query.Encode()

	return queryString
}

// GenerateQueryParamsForAny generates query params of map[string]any type.
func GenerateQueryParamsForAny(queryParams map[string]any) string {
	query := url.Values{}

	// convert any to string
	for key, value := range queryParams {
		switch m := value.(type) {
		case string:
			query.Set(key, m)
		case int:
			query.Set(key, strconv.Itoa(m))
		case float32:
		case float64:
			query.Set(key, strconv.FormatFloat(float64(m), 'f', -1, 64))
		case bool:
			query.Set(key, strconv.FormatBool(m))
		}
	}
	return query.Encode()
}

// GenerateQueryParams can be used as the container function for both GenerateQueryParamsForStrings and GenerateQueryParamsForAny.
func GenerateQueryParams(data interface{}) string {
	var queryString string
	switch m := data.(type) {
	case map[string]string:
		queryString = GenerateQueryParamsForStrings(m)
	case map[string]any:
		queryString = GenerateQueryParamsForAny(m)
	}
	return queryString
}

// WithHeaders converts json to map and adds it to the request headers.
func (req *BaseRequest) WithHeaders(jsonData js.Json) (*BaseRequest, error) {
	jsonMap, err := jsonData.ToMap()
	if err != nil {
		log.Fatal("error with your json input !!")
	}

	req.Headers = jsonMap
	return req, err
}

// WithHeadersMap adds the key-value of a map to the request headers.
func (req *BaseRequest) WithHeadersMap(headersMap *(map[string]string)) *BaseRequest {
	for key, value := range *headersMap {
		req.Headers[key] = value
	}
	return req
}

// WithCookies adds single cookie to the request
func (req *BaseRequest) WithCookie(key string, value string) *BaseRequest {
	if req.Cookies == nil {
		req.Cookies = make(map[string]string)
	}

	req.Cookies[key] = value
	return req
}

// WithBody adds body to the request based on the request body data type and form flags.
func (req *BaseRequest) WithBody(body string, form *bool, multipart bool) *BaseRequest {

	// if form flag is true, convert json to map and add it to the request body as query params
	// in the request body for POST and PUT requests, and in the request url for GET and DELETE requests.
	if *form {
		mapBody, err := js.ToMapOptionalJS(body)
		if err != nil {
			log.Fatal("Issue in json format")
		}

		if req.Method == "GET" || req.Method == "DELETE" {
			req.AddQueryString(mapBody)
		} else {
			req.Body = GenerateQueryParams(mapBody)
		}
	} else if multipart {
		multipartInput := input.NewMultipartInputInJSONFormat(body)
		req.MultipartBody = multipartInput.Body
		req.Writer = multipartInput.Writer
		req.Headers["Content-Type"] = req.Writer.FormDataContentType()
	} else {
		/*
			If form flag is false, add the json to the request body for Post and Put requests,
			and as query params in the request url for Get and Delete requests,
			if the request can be parsed as json.
			else if the request body is not in json format, send it as text/plain.
		*/

		if req.Method == "GET" || req.Method == "Delete" || req.Method == "HEAD" || req.Method == "Trace" || req.Method == "Connect" || req.Method == "Options" {
			mapBody, err := js.ToMapOptionalJS(body)
			if err != nil {
				log.Println("Your request body is not in json format, so it will be sent as text/plain.")
				req.Body = body
				req.Headers["Content-Type"] = "text/plain"
			} else {
				req.AddQueryString(mapBody)
			}
		} else {
			// send the request body as it for Post and Put requests.
			req.Body = body
		}
	}

	return req
}

// Send function sends the request to the server.
func (req *BaseRequest) Send(ss, sh, sb, redirect bool) (*rs.Response, error) {
	client := &http.Client{}

	client = &http.Client{
		// CheckRedirect specifies the policy for handling redirects.
		// If CheckRedirect is not nil, the client calls it before following an HTTP redirect.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if redirect {
				return nil
			} else {
				return http.ErrUseLastResponse
			}
		},
		Transport: &http.Transport{
			// setting it to true will ignore the validity of the certificate, so it will work with self-signed certificates.
			// it removes the protection against man-in-the-middle attacks (if it is true).
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var reqHttp *http.Request
	var err error

	if req.Writer != nil {
		reqHttp, err = http.NewRequest(req.Method, req.URL, req.MultipartBody)
	} else {
		reqHttp, err = http.NewRequest(req.Method, req.URL, strings.NewReader(req.Body))
	}
	if err != nil {
		log.Fatal("error in instatiating a new request !!!")
	}

	addDefaultHeaders(&req.Headers)

	for key, value := range req.Headers {
		reqHttp.Header.Add(key, fmt.Sprintf("%v", value))
	}

	if len(req.BasicAuth.Username) > 0 {
		reqHttp.SetBasicAuth(req.BasicAuth.Username, req.BasicAuth.Password)
	}

	for key, value := range req.Cookies {
		reqHttp.AddCookie(&http.Cookie{Name: key, Value: value})
	}

	resp, err := client.Do(reqHttp)
	if err != nil {
		println(err)
		return nil, err
	}

	defer resp.Body.Close()
	newRes := rs.NewResponse(resp, ss, sh, sb)
	return &newRes, nil
}

func addDefaultHeaders(headers *map[string]any) {
	if _, ok := (*headers)["Content-Type"]; !ok {
		(*headers)["Content-Type"] = "application/json"
	}
	if _, ok := (*headers)["Accept-Encoding"]; !ok {
		(*headers)["Accept-Encoding"] = "gzip, deflate, br"
	}
	if _, ok := (*headers)["Accept"]; !ok {
		(*headers)["Accept"] = "*/*"
	}
	if _, ok := (*headers)["Connection"]; !ok {
		(*headers)["Connection"] = "keep-alive"
	}
	if _, ok := (*headers)["User-Agent"]; !ok {
		(*headers)["User-Agent"] = "RequestCLI"
	}

}
