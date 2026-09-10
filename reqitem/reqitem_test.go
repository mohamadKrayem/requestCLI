package reqitem

import (
	"strings"
	"testing"
)

func kindName(k Kind) string {
	switch k {
	case Header:
		return "Header"
	case HeaderUnset:
		return "HeaderUnset"
	case Query:
		return "Query"
	case Field:
		return "Field"
	case RawField:
		return "RawField"
	case FileField:
		return "FileField"
	case FileUpload:
		return "FileUpload"
	}
	return "none"
}

// The consequences table from the design brief. Each row is a case where a
// naive implementation picks the wrong separator.
func TestParseSeparatorPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		arg   string
		kind  Kind
		key   string
		value string
	}{
		{name: "query beats field", arg: "page==1", kind: Query, key: "page", value: "1"},
		{name: "raw field beats header", arg: "age:=22", kind: RawField, key: "age", value: "22"},
		{name: "file field beats field", arg: "bio=@./bio.txt", kind: FileField, key: "bio", value: "./bio.txt"},
		{name: "file upload", arg: "avatar@./me.png", kind: FileUpload, key: "avatar", value: "./me.png"},
		// The '=' appears before the '@', so this is an ordinary field whose
		// value happens to contain an @ — not a file upload.
		{name: "earlier equals beats later at", arg: "email=a@b.com", kind: Field, key: "email", value: "a@b.com"},
		{name: "third equals is part of the value", arg: "key===v", kind: Query, key: "key", value: "=v"},
		{name: "empty query value", arg: "page==", kind: Query, key: "page", value: ""},
		{name: "empty field value", arg: "key=", kind: Field, key: "key", value: ""},
		{name: "header", arg: "X-Token:abc", kind: Header, key: "X-Token", value: "abc"},
		{name: "trailing colon unsets", arg: "User-Agent:", kind: HeaderUnset, key: "User-Agent", value: ""},
		// Values are never trimmed: a single space is a real header value.
		{name: "value is not trimmed", arg: "X-Token: ", kind: Header, key: "X-Token", value: " "},
		{name: "escaped colon in key", arg: `foo\:bar=1`, kind: Field, key: "foo:bar", value: "1"},
		{name: "escaped backslash in key", arg: `foo\\bar=1`, kind: Field, key: `foo\bar`, value: "1"},
		{name: "raw json array", arg: "tags:=[1,2]", kind: RawField, key: "tags", value: "[1,2]"},
		{name: "url as a field value", arg: "next=https://x/a?b=c", kind: Field, key: "next", value: "https://x/a?b=c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := Parse([]string{tt.arg})
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.arg, err)
			}
			if len(items) != 1 {
				t.Fatalf("Parse(%q) returned %d items, want 1", tt.arg, len(items))
			}

			got := items[0]
			if got.Kind != tt.kind {
				t.Errorf("Parse(%q) kind = %s, want %s", tt.arg, kindName(got.Kind), kindName(tt.kind))
			}
			if got.Key != tt.key {
				t.Errorf("Parse(%q) key = %q, want %q", tt.arg, got.Key, tt.key)
			}
			if got.Value != tt.value {
				t.Errorf("Parse(%q) value = %q, want %q", tt.arg, got.Value, tt.value)
			}
			if got.Arg != tt.arg {
				t.Errorf("Parse(%q) did not keep the original argument, got %q", tt.arg, got.Arg)
			}
		})
	}
}

func TestParseRejectsMalformedItems(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		// This is what rejects a second URL: `rq get a.com b.com`.
		{name: "no separator", arg: "b.com", want: "not a request item"},
		{name: "empty argument", arg: "", want: "not a request item"},
		{name: "no name before equals", arg: "=v", want: "has no name"},
		{name: "no name before colon", arg: ":v", want: "has no name"},
		{name: "no name before at", arg: "@file", want: "has no name"},
		{name: "file upload without a path", arg: "avatar@", want: "has no file path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]string{tt.arg})
			if err == nil {
				t.Fatalf("Parse(%q) should have failed", tt.arg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse(%q) error = %q, want it to contain %q", tt.arg, err, tt.want)
			}
			// The message must quote the argument, or the user cannot find it
			// in a long command line.
			if tt.arg != "" && !strings.Contains(err.Error(), tt.arg) {
				t.Errorf("Parse(%q) error = %q, does not quote the offending argument", tt.arg, err)
			}
		})
	}
}

func TestParseEmptyInput(t *testing.T) {
	items, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(items) != 0 {
		t.Errorf("Parse(nil) = %v, want no items", items)
	}
}

func TestParsePreservesOrder(t *testing.T) {
	items, err := Parse([]string{"a=1", "B:2", "c==3"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := []Kind{Field, Header, Query}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for i, kind := range want {
		if items[i].Kind != kind {
			t.Errorf("item %d kind = %s, want %s", i, kindName(items[i].Kind), kindName(kind))
		}
	}
}

func TestHeaderOps(t *testing.T) {
	items, err := Parse([]string{"X-First:1", "User-Agent:", "X-Second:2", "Accept:"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	set, unset := items.HeaderOps()

	if len(set) != 2 || set[0] != (KV{"X-First", "1"}) || set[1] != (KV{"X-Second", "2"}) {
		t.Errorf("set = %v, want X-First then X-Second in order", set)
	}
	if len(unset) != 2 || unset[0] != "User-Agent" || unset[1] != "Accept" {
		t.Errorf("unset = %v, want [User-Agent Accept]", unset)
	}
}

func TestQueryValuesKeepsRepeats(t *testing.T) {
	items, err := Parse([]string{"tag==a", "tag==b", "page==1", "name=ignored"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	values := items.QueryValues()

	if got := values["tag"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("tag = %v, want both values in order", got)
	}
	if got := values.Get("page"); got != "1" {
		t.Errorf("page = %q, want 1", got)
	}
	if _, ok := values["name"]; ok {
		t.Error("a field item leaked into the query values")
	}
}

func TestHasBody(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "headers only", args: []string{"X-Token:abc"}, want: false},
		{name: "unset only", args: []string{"User-Agent:"}, want: false},
		{name: "query only", args: []string{"page==1"}, want: false},
		{name: "field", args: []string{"name=Mohamad"}, want: true},
		{name: "raw field", args: []string{"age:=22"}, want: true},
		{name: "file field", args: []string{"bio=@./bio.txt"}, want: true},
		{name: "file upload", args: []string{"avatar@./me.png"}, want: true},
		{name: "nothing at all", args: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := items.HasBody(); got != tt.want {
				t.Errorf("HasBody() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFileUpload(t *testing.T) {
	items, err := Parse([]string{"name=Mohamad", "avatar@./me.png"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !items.HasFileUpload() {
		t.Error("HasFileUpload() = false, want true")
	}

	// A file *field* reads a file but is not an upload: it becomes an ordinary
	// value, so it must not imply multipart.
	items, err = Parse([]string{"bio=@./bio.txt"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if items.HasFileUpload() {
		t.Error("a key=@file item was treated as a file upload")
	}
}
