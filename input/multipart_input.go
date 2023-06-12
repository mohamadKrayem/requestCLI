package input

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	json "github.com/mohamadkrayem/requestCLI/formats"
)

type MultipartInput struct {
	Files  map[string]string
	Fields map[string]interface{}
	Body   *bytes.Buffer
	Writer *multipart.Writer
}

func NewMultipartInput() MultipartInput {
	return MultipartInput{}
}

func NewMultipartInputInJSONFormat(jsonData string) MultipartInput {
	jsonMap, err := json.ToMapOptionalJS(jsonData)
	if err != nil {
		fmt.Println(err)
	}
	var multipartInput MultipartInput = NewMultipartInput()
	multipartInput.Body, multipartInput.Writer = multipartInput.generateData(jsonMap)
	return multipartInput
}

// function to check for the file name and the path in the map of the json formated data
func (MultipartInput *MultipartInput) generateData(jsonMap map[string]interface{}) (*bytes.Buffer, *multipart.Writer) {
	var body *bytes.Buffer = &bytes.Buffer{}
	var writer *multipart.Writer = multipart.NewWriter(body)

	for key, value := range jsonMap {
		if key[:2] == "@!" {
			// create a slice
			if MultipartInput.Files == nil {
				MultipartInput.Files = make(map[string]string)
			}
			MultipartInput.Files[key[2:]] = value.(string)
		} else {
			valueOfTheField := fmt.Sprintf("%v", value)
			err := writer.WriteField(key, valueOfTheField)
			if err != nil {
				fmt.Println("Failed to write field:", err)
				return nil, nil
			}
		}
	}
	if MultipartInput.Files != nil {
		_, _ = MultipartInput.generateFiles(body, writer)
	}
	return body, writer
}

func generateLocation(location string) string {
	if location[0] == '/' {
		return location
	}
	if location[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			return ""
		}
		return home + location[1:]
	}

	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}
	return currentDir + "/" + location
}

func (MultipartInput *MultipartInput) generateFiles(body *bytes.Buffer, writer *multipart.Writer) (*bytes.Buffer, *multipart.Writer) {
	for key, value := range MultipartInput.Files {
		file, err := os.Open(generateLocation(value))
		fmt.Println(generateLocation(value))
		if err != nil {
			fmt.Println("Failed to open file:", err)
			return nil, nil
		}
		defer file.Close()
		// Add the file field
		fileField, err := writer.CreateFormFile(key, filepath.Base(value))
		if err != nil {
			fmt.Println("Failed to create form file:", err)
			return nil, nil
		}
		// Copy the file contents to the file field
		_, err = io.Copy(fileField, file)
		if err != nil {
			fmt.Println("Failed to copy file contents:", err)
			return nil, nil
		}
	}

	//Add additional fields in the form
	for key, value := range MultipartInput.Fields {
		err := writer.WriteField(key, fmt.Sprintf("%v", value))
		if err != nil {
			fmt.Println("Failed to write field:", err)
			return nil, nil
		}
	}

	// Close the multipart writer
	err := writer.Close()
	if err != nil {
		fmt.Println("Failed to close multipart writer:", err)
		return nil, nil
	}

	return body, writer
}
