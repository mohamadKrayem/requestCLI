package formats

import (
	"strings"
	"testing"
)

func TestNewJSONRejectsEmptyInput(t *testing.T) {
	// Regression: this used to index jsonStr[0] and panic.
	for _, in := range []string{"", "   ", "\n"} {
		if _, err := NewJSON(in); err == nil {
			t.Errorf("NewJSON(%q) should have returned an error", in)
		}
	}
}

func TestNewJSONRejectsMalformedInput(t *testing.T) {
	// Regression: this used to call log.Fatal and kill the process.
	for _, in := range []string{"hello", "{", `{"a":}`} {
		if _, err := NewJSON(in); err == nil {
			t.Errorf("NewJSON(%q) should have returned an error", in)
		}
	}
}

func TestNewJSONAcceptsObjectsAndArrays(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "object", in: `{"a":1}`, want: `{"a":1}`},
		{name: "array of scalars", in: `[1,2]`, want: `[1,2]`},
		{name: "array of objects", in: `[{"a":1}]`, want: `[{"a":1}]`},
		{name: "leading whitespace before array", in: "  [1]", want: `[1]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewJSON(tt.in)
			if err != nil {
				t.Fatalf("NewJSON(%q): %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Errorf("NewJSON(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Regression, inverted: removeNewLines used to strip \n out of string *data*,
// silently corrupting any description or markdown field on its way to the
// server. It was meant to normalize multi-line stdin input, but scanRequest
// already joins those lines, so it was redundant as well as destructive.
func TestNewJSONPreservesNewlinesInsideValues(t *testing.T) {
	got, err := NewJSON(`{"a":"x\ny"}`)
	if err != nil {
		t.Fatalf("NewJSON: %v", err)
	}
	if string(got) != `{"a":"x\ny"}` {
		t.Errorf("got %q, want the newline preserved", got)
	}
}

func TestNewJSONPreservesNewlinesInsideNestedStructures(t *testing.T) {
	got, err := NewJSON(`{"outer":{"inner":["a\nb"]}}`)
	if err != nil {
		t.Fatalf("NewJSON: %v", err)
	}
	if !strings.Contains(string(got), `a\nb`) {
		t.Errorf("nested newline lost: got %q", got)
	}
}

// Compacting rather than decoding also keeps large integers and key order
// intact, which the old marshal round trip destroyed.
func TestNewJSONPreservesLargeIntegersAndKeyOrder(t *testing.T) {
	const in = `{"zebra":1234567890123456789,"apple":2}`

	got, err := NewJSON(in)
	if err != nil {
		t.Fatalf("NewJSON: %v", err)
	}
	if string(got) != in {
		t.Errorf("NewJSON(%q) = %q, want it unchanged", in, got)
	}
}

func TestNewJSONCompactsWhitespace(t *testing.T) {
	got, err := NewJSON("{\n  \"a\": 1,\n  \"b\": 2\n}")
	if err != nil {
		t.Fatalf("NewJSON: %v", err)
	}
	if string(got) != `{"a":1,"b":2}` {
		t.Errorf("got %q, want it compacted onto one line", got)
	}
}

func TestToMapOptionalJS(t *testing.T) {
	got, err := ToMapOptionalJS(`{"a":1,"b":"two"}`)
	if err != nil {
		t.Fatalf("ToMapOptionalJS: %v", err)
	}
	if got["b"] != "two" {
		t.Errorf(`got["b"] = %v, want "two"`, got["b"])
	}

	if _, err := ToMapOptionalJS(`not json`); err == nil {
		t.Error("ToMapOptionalJS should reject non-json input")
	}
}

func TestToJSON(t *testing.T) {
	got, err := ToJSON(map[string]string{"X-Token": "123"})
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if string(got) != `{"X-Token":"123"}` {
		t.Errorf("ToJSON = %q", got)
	}
}
