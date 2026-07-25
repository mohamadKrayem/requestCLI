// Package reqitem parses HTTPie-style positional request items.
//
// A request item is one command-line argument carrying a separator that decides
// what it contributes to the request:
//
//	name=Mohamad        a JSON string field, or a form field
//	age:=22             a raw JSON value, written verbatim
//	X-Token:abc         a header
//	User-Agent:         unset a header the tool would otherwise default
//	page==1             a query parameter
//	avatar@./me.png     a multipart file attachment
//	bio=@./bio.txt      a field whose value is read from a file
//
// The URL never reaches this package: the caller passes args[1:], with the URL
// fixed at argument 0. That single rule removes the whole ambiguity class
// between a header (Key:value) and a scheme-less URL (host:port), which would
// otherwise need heuristics to separate.
//
// This is a CLI-surface grammar, not a transport concern, so it sits beside
// core rather than inside it — core stays usable by a front-end that never sees
// a command line.
package reqitem

import (
	"fmt"
	"net/url"
	"strings"
)

// Kind is what a request item contributes to the request.
type Kind uint8

const (
	Header      Kind = iota + 1 // Key:value
	HeaderUnset                 // Key:
	Query                       // key==value
	Field                       // key=value    JSON string / form field
	RawField                    // key:=value   verbatim JSON value
	FileField                   // key=@path    field value read from a file
	FileUpload                  // key@path     multipart file attachment
)

// Item is one parsed positional argument.
type Item struct {
	Kind  Kind
	Key   string // separators unescaped
	Value string // literal value, or the path for FileField and FileUpload
	Arg   string // the original argument, used only in error messages
}

// Items is a parsed argument list, in the order the user wrote it.
type Items []Item

// KV is a header name and value.
type KV struct{ Key, Value string }

// Parse parses request items. It never sees the URL: the caller passes args[1:].
func Parse(args []string) (Items, error) {
	if len(args) == 0 {
		return nil, nil
	}

	items := make(Items, 0, len(args))
	for _, arg := range args {
		item, err := parseOne(arg)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// parseOne scans a single argument for the first separator that appears in it.
//
// The scan runs left to right rather than searching for each separator in turn,
// because which separator comes *first* decides the kind: in "email=a@b.com" the
// '=' wins and the '@' is part of the value, while in "avatar@./me.png" the '@'
// is the separator. Searching per-separator would get one of those wrong.
func parseOne(arg string) (Item, error) {
	var key strings.Builder

	for i := 0; i < len(arg); i++ {
		// An escape contributes the next byte to the key literally, so a key may
		// contain a character that would otherwise separate. Values are verbatim,
		// so escaping deliberately does not apply to them.
		if arg[i] == '\\' && i+1 < len(arg) {
			key.WriteByte(arg[i+1])
			i++
			continue
		}

		kind, width := separatorAt(arg, i)
		if kind == 0 {
			key.WriteByte(arg[i])
			continue
		}

		item := Item{
			Kind:  kind,
			Key:   key.String(),
			Value: arg[i+width:],
			Arg:   arg,
		}
		// A trailing ':' with nothing after it means "remove this header",
		// which is a different operation from setting it to the empty string.
		if item.Kind == Header && item.Value == "" {
			item.Kind = HeaderUnset
		}
		return item, validate(item)
	}

	return Item{}, fmt.Errorf(
		"not a request item: %q (expected key=value, key:=value, key==value, Key:value, key@file or key=@file)",
		arg)
}

// separatorAt reports the separator starting at i, and how many bytes it spans.
//
// Two-character forms are tested first: "==" must not be read as "=" followed by
// a value beginning with '=', and ":=" must not be read as a header.
func separatorAt(s string, i int) (Kind, int) {
	if i+1 < len(s) {
		switch s[i : i+2] {
		case "==":
			return Query, 2
		case ":=":
			return RawField, 2
		case "=@":
			return FileField, 2
		}
	}

	switch s[i] {
	case '@':
		return FileUpload, 1
	case '=':
		return Field, 1
	case ':':
		return Header, 1
	}
	return 0, 0
}

func validate(item Item) error {
	if item.Key == "" {
		return fmt.Errorf("request item %q has no name", item.Arg)
	}
	if item.Kind == FileUpload && item.Value == "" {
		return fmt.Errorf("request item %q has no file path", item.Arg)
	}
	return nil
}

// HeaderOps returns the headers to set, in the order given, and the header names
// to unset.
func (it Items) HeaderOps() (set []KV, unset []string) {
	for _, item := range it {
		switch item.Kind {
		case Header:
			set = append(set, KV{Key: item.Key, Value: item.Value})
		case HeaderUnset:
			unset = append(unset, item.Key)
		}
	}
	return set, unset
}

// QueryValues returns the query parameters. Repeated values for one key are
// preserved in order.
func (it Items) QueryValues() url.Values {
	values := url.Values{}
	for _, item := range it {
		if item.Kind == Query {
			values.Add(item.Key, item.Value)
		}
	}
	return values
}

// HasBody reports whether any item contributes to the request body.
func (it Items) HasBody() bool {
	for _, item := range it {
		switch item.Kind {
		case Field, RawField, FileField, FileUpload:
			return true
		}
	}
	return false
}

// HasFileUpload reports whether any item attaches a file.
func (it Items) HasFileUpload() bool {
	for _, item := range it {
		if item.Kind == FileUpload {
			return true
		}
	}
	return false
}
