package reqitem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustParse is a small helper so encoder tests can build an Items value from
// literal argument strings without repeating the error check everywhere.
func mustParse(t *testing.T, args ...string) Items {
	t.Helper()
	items, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse(%v): %v", args, err)
	}
	return items
}

// JSONBody must never go through map[string]any: that would both sort keys
// and round large integers through float64. This test writes keys that would
// sort differently than they were typed, so a regression to map-based
// encoding fails loudly rather than by chance.
func TestJSONBodyPreservesWrittenOrder(t *testing.T) {
	items := mustParse(t, "b:=2", "a:=1", "c:=3")

	got, err := items.JSONBody()
	if err != nil {
		t.Fatalf("JSONBody: %v", err)
	}

	want := `{"b":2,"a":1,"c":3}`
	if string(got) != want {
		t.Errorf("JSONBody() = %s, want %s (alphabetical order would mean a map crept back in)", got, want)
	}
}

// Regression: the previous map[string]any-based encoder rounded a 19-digit
// integer through float64, corrupting it in transit. JSONBody must write the
// raw text of a RawField verbatim so the id reaches the server byte-exact.
func TestJSONBodyKeepsLargeIntegerByteExact(t *testing.T) {
	items := mustParse(t, "id:=1234567890123456789")

	got, err := items.JSONBody()
	if err != nil {
		t.Fatalf("JSONBody: %v", err)
	}

	want := `{"id":1234567890123456789}`
	if string(got) != want {
		t.Errorf("JSONBody() = %s, want %s (large integer must survive byte-exact)", got, want)
	}

	// Confirm the digits themselves, not just object equality, actually appear
	// verbatim in the emitted bytes.
	if !strings.Contains(string(got), "1234567890123456789") {
		t.Errorf("JSONBody() = %s, does not contain the id verbatim", got)
	}
}

func TestJSONBodyRawFieldProducesRealJSONTypes(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "number", arg: "n:=42", want: `{"n":42}`},
		{name: "bool", arg: "b:=true", want: `{"b":true}`},
		{name: "array", arg: "arr:=[1,2,3]", want: `{"arr":[1,2,3]}`},
		{name: "object", arg: `obj:={"x":1}`, want: `{"obj":{"x":1}}`},
		{name: "null", arg: "nul:=null", want: `{"nul":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := mustParse(t, tt.arg)
			got, err := items.JSONBody()
			if err != nil {
				t.Fatalf("JSONBody(%q): %v", tt.arg, err)
			}
			if string(got) != tt.want {
				t.Errorf("JSONBody(%q) = %s, want %s", tt.arg, got, tt.want)
			}
		})
	}
}

// Field (a bare "=") must always produce a JSON string, even when the literal
// text looks like a number or a bool — that distinction from ":=" is the
// entire point of the two separators.
func TestJSONBodyFieldAlwaysProducesAString(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "numeric-looking string", arg: "n=42", want: `{"n":"42"}`},
		{name: "bool-looking string", arg: "b=true", want: `{"b":"true"}`},
		{name: "empty string", arg: "e=", want: `{"e":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := mustParse(t, tt.arg)
			got, err := items.JSONBody()
			if err != nil {
				t.Fatalf("JSONBody(%q): %v", tt.arg, err)
			}
			if string(got) != tt.want {
				t.Errorf("JSONBody(%q) = %s, want %s", tt.arg, got, tt.want)
			}
		})
	}
}

// Duplicate keys: the last occurrence wins, and it appears at the position of
// that last occurrence, not the position of the first.
func TestJSONBodyDuplicateKeyLastOccurrenceWinsAtItsPosition(t *testing.T) {
	items := mustParse(t, "a=1", "b=2", "a=3")

	got, err := items.JSONBody()
	if err != nil {
		t.Fatalf("JSONBody: %v", err)
	}

	// "a" was first written at position 0, but its last occurrence (value 3)
	// is at position 2, after "b" — so "b" must come first in the output.
	want := `{"b":"2","a":"3"}`
	if string(got) != want {
		t.Errorf("JSONBody() = %s, want %s (dup key must resolve to last value, at last position)", got, want)
	}
}

// Invalid raw JSON is rejected, and the error quotes the offending argument
// so the user can find it in a long command line.
func TestJSONBodyRejectsInvalidRawJSON(t *testing.T) {
	items := mustParse(t, "age:=abc")

	_, err := items.JSONBody()
	if err == nil {
		t.Fatal("JSONBody should have rejected age:=abc")
	}
	if !strings.Contains(err.Error(), "age:=abc") {
		t.Errorf("error = %q, does not quote the offending argument", err)
	}
	if !strings.Contains(err.Error(), "not valid json") {
		t.Errorf("error = %q, want it to say the value is not valid json", err)
	}
}

func TestJSONBodyFileFieldReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bio.txt")
	if err := os.WriteFile(path, []byte("hello from disk"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	items := mustParse(t, "bio=@"+path)
	got, err := items.JSONBody()
	if err != nil {
		t.Fatalf("JSONBody: %v", err)
	}

	want := `{"bio":"hello from disk"}`
	if string(got) != want {
		t.Errorf("JSONBody() = %s, want %s", got, want)
	}
}

// An unreadable file must produce a wrapped error, not a panic and not an
// empty/omitted value.
func TestJSONBodyUnreadableFileReturnsError(t *testing.T) {
	items := mustParse(t, "bio=@/definitely/does/not/exist.txt")

	_, err := items.JSONBody()
	if err == nil {
		t.Fatal("JSONBody should have failed for an unreadable file")
	}
	if !strings.Contains(err.Error(), "/definitely/does/not/exist.txt") {
		t.Errorf("error = %q, want it to name the unreadable file", err)
	}
}

func TestFormValuesEncodesFieldsAndRawValuesVerbatim(t *testing.T) {
	items := mustParse(t, "age:=22", "name=Mo", "tags:=[1,2]")

	values, err := items.FormValues()
	if err != nil {
		t.Fatalf("FormValues: %v", err)
	}

	// url.Values.Encode sorts keys alphabetically; this is the existing,
	// documented behaviour and not something the item encoders change.
	want := "age=22&name=Mo&tags=%5B1%2C2%5D"
	if got := values.Encode(); got != want {
		t.Errorf("FormValues().Encode() = %q, want %q", got, want)
	}
}

func TestFormValuesFileFieldReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bio.txt")
	if err := os.WriteFile(path, []byte("form content"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	items := mustParse(t, "bio=@"+path)
	values, err := items.FormValues()
	if err != nil {
		t.Fatalf("FormValues: %v", err)
	}
	if got := values.Get("bio"); got != "form content" {
		t.Errorf("bio = %q, want %q", got, "form content")
	}
}

// A FileUpload item is skipped by FormValues: the caller has already rejected
// combining a file upload with -f, so this only guards against a silent leak.
func TestFormValuesSkipsFileUpload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "me.png")
	if err := os.WriteFile(path, []byte("binary"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	items := mustParse(t, "name=Mo", "avatar@"+path)
	values, err := items.FormValues()
	if err != nil {
		t.Fatalf("FormValues: %v", err)
	}
	if _, ok := values["avatar"]; ok {
		t.Error("FormValues should skip a FileUpload item, not include it")
	}
	if got := values.Get("name"); got != "Mo" {
		t.Errorf("name = %q, want Mo", got)
	}
}

// Duplicate keys resolve to the last occurrence for FormValues too.
func TestFormValuesDuplicateKeyLastOccurrenceWins(t *testing.T) {
	items := mustParse(t, "age=1", "age:=2")

	values, err := items.FormValues()
	if err != nil {
		t.Fatalf("FormValues: %v", err)
	}
	if got := values.Get("age"); got != "2" {
		t.Errorf("age = %q, want 2 (last occurrence wins)", got)
	}
}

func TestFormValuesRejectsInvalidRawJSON(t *testing.T) {
	items := mustParse(t, "age:=abc")
	if _, err := items.FormValues(); err == nil {
		t.Fatal("FormValues should reject age:=abc")
	} else if !strings.Contains(err.Error(), "age:=abc") {
		t.Errorf("error = %q, does not quote the offending argument", err)
	}
}

func TestFormValuesUnreadableFileReturnsError(t *testing.T) {
	items := mustParse(t, "bio=@/definitely/does/not/exist.txt")
	if _, err := items.FormValues(); err == nil {
		t.Fatal("FormValues should have failed for an unreadable file")
	}
}

func TestMultipartFieldsPreservesWrittenOrder(t *testing.T) {
	dir := t.TempDir()
	avatarPath := filepath.Join(dir, "me.png")
	bioPath := filepath.Join(dir, "bio.txt")
	if err := os.WriteFile(avatarPath, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("writing avatar fixture: %v", err)
	}
	if err := os.WriteFile(bioPath, []byte("bio text"), 0o600); err != nil {
		t.Fatalf("writing bio fixture: %v", err)
	}

	items := mustParse(t, "name=Mohamad", "avatar@"+avatarPath, "bio=@"+bioPath, "age:=22")

	fields, err := items.MultipartFields()
	if err != nil {
		t.Fatalf("MultipartFields: %v", err)
	}

	if len(fields) != 4 {
		t.Fatalf("got %d fields, want 4: %+v", len(fields), fields)
	}

	wantOrder := []string{"name", "avatar", "bio", "age"}
	for i, name := range wantOrder {
		if fields[i].Name != name {
			t.Errorf("field %d name = %q, want %q (order not preserved)", i, fields[i].Name, name)
		}
	}

	if fields[0].Value != "Mohamad" {
		t.Errorf("name value = %q, want Mohamad", fields[0].Value)
	}
	if fields[1].Path == "" {
		t.Errorf("avatar field should carry a Path, got Value %q", fields[1].Value)
	}
	if fields[2].Value != "bio text" {
		t.Errorf("bio value = %q, want %q (read from disk)", fields[2].Value, "bio text")
	}
	if fields[3].Value != "22" {
		t.Errorf("age value = %q, want 22 verbatim", fields[3].Value)
	}
}

// A field named twice, once as a plain value and once as a file upload,
// collapses to a single part: the last occurrence wins across all four body
// kinds together, not just within one kind.
func TestMultipartFieldsDuplicateAcrossKindsLastWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "avatar.png")
	if err := os.WriteFile(path, []byte("real image"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	items := mustParse(t, "avatar=placeholder", "avatar@"+path)

	fields, err := items.MultipartFields()
	if err != nil {
		t.Fatalf("MultipartFields: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("got %d fields, want 1 (duplicate must collapse): %+v", len(fields), fields)
	}
	if fields[0].Path == "" {
		t.Errorf("expected the file-upload occurrence to win, got Value %q instead of a Path", fields[0].Value)
	}
}

func TestMultipartFieldsRejectsInvalidRawJSON(t *testing.T) {
	items := mustParse(t, "age:=abc")
	if _, err := items.MultipartFields(); err == nil {
		t.Fatal("MultipartFields should reject age:=abc")
	} else if !strings.Contains(err.Error(), "age:=abc") {
		t.Errorf("error = %q, does not quote the offending argument", err)
	}
}

// FileField (key=@path) reads its content eagerly, so MultipartFields itself
// must fail when the file cannot be read.
func TestMultipartFieldsUnreadableFileFieldReturnsError(t *testing.T) {
	items := mustParse(t, "bio=@/definitely/does/not/exist.txt")
	if _, err := items.MultipartFields(); err == nil {
		t.Fatal("MultipartFields should have failed for an unreadable FileField")
	}
}

// FileUpload (key@path), by contrast, only resolves the path here — the file
// itself is opened later by input.NewMultipartInputFromFields, when the
// multipart body is actually built. MultipartFields must still produce a
// Path so the later open is attempted at all, rather than silently dropping
// the attachment.
func TestMultipartFieldsFileUploadDefersTheOpen(t *testing.T) {
	items := mustParse(t, "avatar@/definitely/does/not/exist.png")
	fields, err := items.MultipartFields()
	if err != nil {
		t.Fatalf("MultipartFields: %v", err)
	}
	if len(fields) != 1 || fields[0].Path == "" {
		t.Fatalf("fields = %+v, want one field carrying a (still unresolved) Path", fields)
	}
}

// Sanity check that every JSONBody encoding this suite asserts on is in fact
// valid JSON, since the test above compares raw bytes rather than decoding.
func TestJSONBodyOutputsAreValidJSON(t *testing.T) {
	items := mustParse(t, "id:=1234567890123456789", "name=Mo", "tags:=[1,2]")
	got, err := items.JSONBody()
	if err != nil {
		t.Fatalf("JSONBody: %v", err)
	}
	if !json.Valid(got) {
		t.Errorf("JSONBody() produced invalid JSON: %s", got)
	}
}
