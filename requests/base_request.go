package requests

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	js "github.com/mohamadkrayem/requestCLI/formats"
	rs "github.com/mohamadkrayem/requestCLI/response"
)

type BaseRequest struct {
	Method    string
	URL       string
	Headers   map[string]any
	Cookies   map[string]string
	Body      string
	BasicAuth auth.BaseAuth
}

func NewRequest(method, url string) BaseRequest {
	return BaseRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]any),
		Cookies: make(map[string]string),
		Body:    "",
	}
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
		queryString := GenerateQueryParams(queryParams)
		reqURL += "?" + queryString
	}
	return reqURL
}

func (req *BaseRequest) AddQueryString(queryParams map[string]any) {
	req.URL += "?" + GenerateQueryParams(queryParams)
}

func GenerateQueryParamsForStrings(queryParams map[string]string) string {
	query := url.Values{}
	for key, value := range queryParams {
		query.Set(key, value)
	}
	queryString := query.Encode()

	return queryString
}

func GenerateQueryParamsForAny(queryParams map[string]any) string {
	query := url.Values{}
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

func (req *BaseRequest) WithBody(body string, form *bool) *BaseRequest {
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
	} else {
		if req.Method == "GET" || req.Method == "Delete" {
			mapBody, err := js.ToMapOptionalJS(body)
			if err != nil {
				log.Fatal("Iusse in json format")
			}
			req.AddQueryString(mapBody)
		} else {
			req.Body = body
		}
	}

	return req
}

func (req *BaseRequest) Send(ss, sh, sb bool) (*rs.Response, error) {
	client := &http.Client{}
	body := strings.NewReader(req.Body)
	reqHttp, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		panic(err)
	}

	addDefaultHeaders(&req.Headers)

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
	newRes := rs.NewResponse(resp, ss, sh, sb)
	return &newRes, nil
}

func addDefaultHeaders(headers *map[string]any) {
	if _, ok := (*headers)["Content-Type"]; !ok {
		(*headers)["Content-Type"] = "application/json"
	}
	//if _, ok := (*headers)["Accept-Encoding"]; !ok {
	//	(*headers)["Accept-Encoding"] = "gzip, deflate"
	//}
	if _, ok := (*headers)["Accept"]; !ok {
		(*headers)["Accept"] = "*/*"
	}
	if _, ok := (*headers)["Connection"]; !ok {
		(*headers)["Connection"] = "keep-alive"
	}

	(*headers)["User-Agent"] = "RequestCLI"

}
