package input

import (
// "strings"
// "net/url"
// "fmt"
)

type BaseInput struct {
	Method  string
	URL     string
	Body    string
	Cookies map[string]string
	Auth    map[string]string
}
