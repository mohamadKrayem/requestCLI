package input

import (
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseForm reads the generated body back through a multipart reader.
func parseForm(t *testing.T, m *MultipartInput) *multipart.Form {
	t.Helper()

	_, params, err := mime.ParseMediaType(m.Writer.FormDataContentType())
	if err != nil {
		t.Fatalf("parsing content type: %v", err)
	}

	form, err := multipart.NewReader(strings.NewReader(m.Body.String()), params["boundary"]).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("reading multipart form: %v", err)
	}
	return form
}

// Regression: generateData sliced key[:2] and panicked on any key shorter than
// two characters.
func TestShortFieldNameDoesNotPanic(t *testing.T) {
	m, err := NewMultipartInputInJSONFormat(`{"a":1}`)
	if err != nil {
		t.Fatalf("NewMultipartInputInJSONFormat: %v", err)
	}

	form := parseForm(t, m)
	if got := form.Value["a"]; len(got) != 1 || got[0] != "1" {
		t.Errorf(`form field "a" = %v, want ["1"]`, got)
	}
}

// Regression: the writer was only closed when the form contained a file, so a
// file-less form was emitted without its closing boundary.
func TestFormWithoutFilesIsProperlyClosed(t *testing.T) {
	m, err := NewMultipartInputInJSONFormat(`{"name":"Mohamad","age":22}`)
	if err != nil {
		t.Fatalf("NewMultipartInputInJSONFormat: %v", err)
	}

	form := parseForm(t, m)
	if got := form.Value["name"]; len(got) != 1 || got[0] != "Mohamad" {
		t.Errorf(`form field "name" = %v, want ["Mohamad"]`, got)
	}
	if got := form.Value["age"]; len(got) != 1 || got[0] != "22" {
		t.Errorf(`form field "age" = %v, want ["22"]`, got)
	}
}

func TestFileFieldsAreUploaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "letter.txt")
	if err := os.WriteFile(path, []byte("hello from a file"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	m, err := NewMultipartInputInJSONFormat(`{"@!letter":"` + path + `","name":"Mohamad"}`)
	if err != nil {
		t.Fatalf("NewMultipartInputInJSONFormat: %v", err)
	}

	form := parseForm(t, m)

	if got := form.Value["name"]; len(got) != 1 || got[0] != "Mohamad" {
		t.Errorf(`form field "name" = %v, want ["Mohamad"]`, got)
	}

	files := form.File["letter"]
	if len(files) != 1 {
		t.Fatalf(`form file "letter" = %v, want one entry`, files)
	}
	if files[0].Filename != "letter.txt" {
		t.Errorf("uploaded filename = %q, want letter.txt", files[0].Filename)
	}

	f, err := files[0].Open()
	if err != nil {
		t.Fatalf("opening uploaded file: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if string(content) != "hello from a file" {
		t.Errorf("uploaded content = %q", content)
	}
}

func TestMissingFileReturnsError(t *testing.T) {
	_, err := NewMultipartInputInJSONFormat(`{"@!nope":"/definitely/not/here.txt"}`)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), "opening") {
		t.Errorf("error = %v, want it to mention opening the file", err)
	}
}

func TestNonStringFilePathReturnsError(t *testing.T) {
	if _, err := NewMultipartInputInJSONFormat(`{"@!f":123}`); err == nil {
		t.Error("expected an error when a file field is not a string path")
	}
}

func TestMalformedJSONReturnsError(t *testing.T) {
	if _, err := NewMultipartInputInJSONFormat(`not json`); err == nil {
		t.Error("expected an error for a non-json body")
	}
}

func TestResolvePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "absolute path is untouched", in: "/tmp/x.txt", want: "/tmp/x.txt"},
		{name: "tilde expands to home", in: "~/x.txt", want: filepath.Join(home, "x.txt")},
		{name: "relative path joins cwd", in: "x.txt", want: filepath.Join(cwd, "x.txt")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePath(tt.in)
			if err != nil {
				t.Fatalf("resolvePath(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Regression: generateLocation indexed location[0] without a length check.
func TestResolvePathRejectsEmpty(t *testing.T) {
	if _, err := resolvePath(""); err == nil {
		t.Error("resolvePath(\"\") should return an error, not panic")
	}
}
