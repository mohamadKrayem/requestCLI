// Package input builds request bodies from user-supplied data.
package input

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/mohamadkrayem/requestCLI/formats"
)

// FilePrefix marks a JSON field whose value is a path to a file to upload
// rather than a literal field value.
const FilePrefix = "@!"

type MultipartInput struct {
	Files  map[string]string
	Body   *bytes.Buffer
	Writer *multipart.Writer
}

func NewMultipartInput() MultipartInput {
	return MultipartInput{}
}

// NewMultipartInputInJSONFormat builds a multipart body from a JSON object.
//
// Keys prefixed with "@!" are treated as file uploads whose value is a path;
// every other key becomes a plain form field.
func NewMultipartInputInJSONFormat(jsonData string) (*MultipartInput, error) {
	jsonMap, err := formats.ToMapOptionalJS(jsonData)
	if err != nil {
		return nil, fmt.Errorf("--multi needs a json object as the body: %w", err)
	}

	multipartInput := NewMultipartInput()
	if err := multipartInput.generateData(jsonMap); err != nil {
		return nil, err
	}
	return &multipartInput, nil
}

// generateData writes every field and file into the multipart body.
func (m *MultipartInput) generateData(jsonMap map[string]any) error {
	m.Body = &bytes.Buffer{}
	m.Writer = multipart.NewWriter(m.Body)

	for key, value := range jsonMap {
		// A plain strings.HasPrefix check; slicing key[:2] panics on 1-char keys.
		if name, isFile := strings.CutPrefix(key, FilePrefix); isFile {
			path, ok := value.(string)
			if !ok {
				return fmt.Errorf("file field %q must be a string path, got %T", key, value)
			}
			if name == "" {
				return fmt.Errorf("file field %q has no name after the %q prefix", key, FilePrefix)
			}
			if m.Files == nil {
				m.Files = make(map[string]string)
			}
			m.Files[name] = path
			continue
		}

		if err := m.Writer.WriteField(key, fmt.Sprintf("%v", value)); err != nil {
			return fmt.Errorf("writing form field %q: %w", key, err)
		}
	}

	if err := m.attachFiles(); err != nil {
		return err
	}

	// Always close: the closing boundary is required even when the form has no
	// files, otherwise the body is malformed.
	if err := m.Writer.Close(); err != nil {
		return fmt.Errorf("finalizing multipart body: %w", err)
	}
	return nil
}

func (m *MultipartInput) attachFiles() error {
	for field, path := range m.Files {
		location, err := resolvePath(path)
		if err != nil {
			return err
		}
		if err := m.attachFile(field, location); err != nil {
			return err
		}
	}
	return nil
}

// attachFile copies one file into the form. It is split out so each handle is
// closed as soon as its file is written, rather than all of them at the end.
func (m *MultipartInput) attachFile(field, location string) error {
	file, err := os.Open(location)
	if err != nil {
		return fmt.Errorf("opening %s: %w", location, err)
	}
	defer file.Close()

	fileField, err := m.Writer.CreateFormFile(field, filepath.Base(location))
	if err != nil {
		return fmt.Errorf("creating form file %q: %w", field, err)
	}
	if _, err := io.Copy(fileField, file); err != nil {
		return fmt.Errorf("copying %s into the form: %w", location, err)
	}
	return nil
}

// resolvePath expands a user-supplied path: "~" is the home directory, a
// leading "/" is absolute, anything else is relative to the working directory.
func resolvePath(location string) (string, error) {
	if location == "" {
		return "", errors.New("empty file path")
	}

	switch location[0] {
	case '/':
		return location, nil
	case '~':
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
		return filepath.Join(home, location[1:]), nil
	default:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolving working directory: %w", err)
		}
		return filepath.Join(cwd, location), nil
	}
}
