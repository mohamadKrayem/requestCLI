package command

import (
	"bufio"
	//"fmt"
	"os"
	"strings"

	json "github.com/mohamadkrayem/requestCLI/formats"
	rq "github.com/mohamadkrayem/requestCLI/requests"
	"github.com/spf13/cobra"
	//"strings"
)

type Command struct{}

func Run(args []string, cmd *cobra.Command, bodyJS *string, method string, headersjs *json.Json, headersJS *map[string]string, https bool) {
	URL := rq.GenerateUrl(args[0], https, nil)
	request := rq.NewRequest(method, URL)
	if *bodyJS != "" {
		request.WithBody(*bodyJS)
	}

	if headersJS != nil {
		request.WithHeadersMap(headersJS)
	} else if *headersjs != "" {
		request.WithHeaders(*headersjs)
	}

	resp, err := request.Send()
	if err != nil {
		panic(err)
	}
	resp.PrintResponse()
}

func PersistentPreRun(cmd *cobra.Command, args []string, body, headers bool, bodyJS *string, headersJS *map[string]string, headersjs json.Json) {
	if !body && !headers {
		return
	}
	if body && *bodyJS == "" {
		*bodyJS = scanRequest()
	}

	if headers && headersJS == nil {
		headersjs, _ = json.NewJson(scanRequest())
	} else if !headers && headersJS != nil {
		headersjs, _ = json.ToJSON(*headersJS)
	}
}

func scanRequest() string {
	// Read in the user's input
	scanner := bufio.NewScanner(os.Stdin)
	var input, strTest string

	for scanner.Scan() {
		strTest = strings.TrimSpace(scanner.Text())
		if strTest[len(strTest)-1] == ';' {
			break
		}
		input += scanner.Text() + "\n"
	}

	input = strings.ReplaceAll(input, "\\n", "")
	input = strings.ReplaceAll(input, "\n", "")
	// Replace any instances of a backslash followed by a newline
	input += "}"

	return input
}
