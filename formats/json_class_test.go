package formats

import (
	"strings"
	"testing"
)

func TestNewJsonRejectsEmptyInput(t *testing.T) {
	// Regression: this used to index jsonStr[0] and panic.
	for _, in := range []string{"", "   ", "\n"} {
		if _, err := NewJson(in); err == nil {
			t.Errorf("NewJson(%q) should have returned an error", in)
		}
	}
}

func TestNewJsonRejectsMalformedInput(t *testing.T) {
	// Regression: this used to call log.Fatal and kill the process.
	for _, in := range []string{"hello", "{", `{"a":}`} {
		if _, err := NewJson(in); err == nil {
			t.Errorf("NewJson(%q) should have returned an error", in)
		}
	}
}

func TestNewJsonAcceptsObjectsAndArrays(t *testing.T) {
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
			got, err := NewJson(tt.in)
			if err != nil {
				t.Fatalf("NewJson(%q): %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Errorf("NewJson(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Regression: removeNewLines assigned to the loop variable, so it never
// actually stripped anything.
func TestNewJsonStripsNewlinesFromValues(t *testing.T) {
	got, err := NewJson(`{"a":"x\ny"}`)
	if err != nil {
		t.Fatalf("NewJson: %v", err)
	}
	if strings.Contains(string(got), `\n`) {
		t.Errorf("newline not stripped: got %q", got)
	}
	if string(got) != `{"a":"xy"}` {
		t.Errorf("got %q, want %q", got, `{"a":"xy"}`)
	}
}

func TestNewJsonStripsNewlinesInsideNestedStructures(t *testing.T) {
	got, err := NewJson(`{"outer":{"inner":["a\nb"]}}`)
	if err != nil {
		t.Fatalf("NewJson: %v", err)
	}
	if strings.Contains(string(got), `\n`) {
		t.Errorf("nested newline not stripped: got %q", got)
	}
}

func TestIsArray(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "", want: false}, // must not panic
		{in: "   ", want: false},
		{in: "[1]", want: true},
		{in: "  [1]", want: true},
		{in: `{"a":1}`, want: false},
	}

	for _, tt := range tests {
		if got := isArray(tt.in); got != tt.want {
			t.Errorf("isArray(%q) = %v, want %v", tt.in, got, tt.want)
		}
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

func TestGetColorizedJSONHandlesObjectsAndArrays(t *testing.T) {
	for _, in := range []string{`{"a":1}`, `[{"a":1}]`} {
		js := Json(in)
		out, err := js.GetColorizedJSON()
		if err != nil {
			t.Fatalf("GetColorizedJSON(%q): %v", in, err)
		}
		if !strings.Contains(out, "a") {
			t.Errorf("GetColorizedJSON(%q) = %q, expected it to contain the key", in, out)
		}
	}
}
