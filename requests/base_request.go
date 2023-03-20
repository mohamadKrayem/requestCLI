package requests

import (
	//"fmt"
	//	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	js "github.com/mohamadkrayem/requestCLI/formats"
	rs "github.com/mohamadkrayem/requestCLI/response"
	//"net/http"
)

type BaseRequest struct {
	Method    string
	URL       string
	Headers   map[string]any
	Cookies   map[string]string
	Body      string
	BasicAuth auth.BaseAuth
}

func (req *BaseRequest) WithHeader(key string, value string) *BaseRequest {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}

	req.Headers[key] = value
	return req
}

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
		query := url.Values{}
		for key, value := range queryParams {
			query.Set(key, value)
		}
		reqURL += "?" + query.Encode()
	}
	return reqURL
}

func (req *BaseRequest) WithHeaders(jsonData js.Json) (*BaseRequest, error) {
	jsonMap, err := jsonData.ToMap()
	if err != nil {
		panic(err)
	}

	req.Headers = jsonMap
	return req, err
}

func (req *BaseRequest) WithHeadersMap(headersMap *(map[string]string)) *BaseRequest {
	for key, value := range *headersMap {
		req.Headers[key] = value
	}
	return req
}

func (req *BaseRequest) WithCookie(key string, value string) *BaseRequest {
	if req.Cookies == nil {
		req.Cookies = make(map[string]string)
	}

	req.Cookies[key] = value
	return req
}

func (req *BaseRequest) WithBody(body string) *BaseRequest {
	req.Body = body
	return req
}

func (req *BaseRequest) Send() (*rs.Response, error) {
	client := &http.Client{}
	body := strings.NewReader(req.Body)

	reqHttp, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		panic(err)
	}

	for key, value := range req.Headers {
		reqHttp.Header.Add(key, value.(string))
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
	newRes := rs.NewResponse(resp)
	return &newRes, nil
}
