package command

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	json "github.com/mohamadkrayem/requestCLI/formats"
	rq "github.com/mohamadkrayem/requestCLI/requests"
	"github.com/spf13/cobra"
)

type Command struct {
	BodyJS      *string
	Method      string
	Headersjs   *json.Json
	HeadersJS   *map[string]string
	Https       bool
	Ss          bool
	Sh          bool
	Sb          bool
	Form        *bool
	Multipart   bool
	Body        bool
	Headers     bool
	Redirect    bool
	Cookies     *map[string]string
	BasicAuth   auth.BaseAuth
	QueryParams *map[string]string
}

func NewCommand() Command {
	return Command{}
}

func (command *Command) Run(args []string, cmd *cobra.Command) {
	URL := rq.GenerateUrl(args[0], command.Https, *command.QueryParams)
	request := rq.NewRequest(command.Method, URL)

	if *command.Form {
		*command.HeadersJS = make(map[string]string)
		(*command.HeadersJS)["Content-Type"] = "application/x-www-Form-urlencoded"
	} else if command.Multipart {
		(*command.HeadersJS)["Content-Type"] = "multipart/form-data"
	}

	if *command.Cookies != nil {
		for key, value := range *command.Cookies {
			request.WithCookie(key, value)
		}
	}

	if command.BasicAuth.Username != "" && command.BasicAuth.Password != "" {
		request.BasicAuth = command.BasicAuth
	}

	if *command.HeadersJS != nil {
		request.WithHeadersMap(command.HeadersJS)
	} else if *command.Headersjs != "" {
		request.WithHeaders(*command.Headersjs)
	}

	if *command.BodyJS != "" {
		request.WithBody(*command.BodyJS, command.Form, command.Multipart)
	}

	resp, err := request.Send(command.Ss, command.Sh, command.Sb, command.Redirect)
	if err != nil {
		log.Fatal("error in sending the request !!!")
	}
	resp.PrintResponse()
}

func (command *Command) PersistentPreRun(cmd *cobra.Command, args []string) {
	//if no --command.Body or --command.Headers than no need for newScaner();
	if !command.Body && !command.Headers {
		return
	}

	//if nested json; than we need newScanner()
	if command.Headers && *command.HeadersJS == nil {
		*command.Headersjs, _ = json.NewJson(scanRequest())

		//if simple json (map[string]string) than command.Headersjs = jsonOfMap and no need for newScanner()
	} else if !command.Headers && *command.HeadersJS != nil {
		*command.Headersjs, _ = json.ToJSON(*command.HeadersJS)
	}

	//if --command.Body => we need newScanner()
	if command.Body && *command.BodyJS == "" {
		*command.BodyJS = scanRequest()
	}
}

func scanRequest() string {
	// Read in the user's input
	scanner := bufio.NewScanner(os.Stdin)
	var input, strTest string
	var count int

	for scanner.Scan() {
		strTest = strings.TrimSpace(scanner.Text())

		//input[lastIndex] == ';' ? end of the input;
		if strTest[len(strTest)-1] == ';' {
			break
		} else if strTest[len(strTest)-1] == '{' || strTest[len(strTest)-1] == '[' {
			count += 2
		} else if strTest[len(strTest)-1] != ',' {
			count -= 2
		}
		fmt.Print(strings.Repeat(" ", count))
		input += scanner.Text()
	}

	input = strings.ReplaceAll(input, "\\n", "")
	input = strings.ReplaceAll(input, "\n", "")
	// Replace any instances of a backslash followed by a newline
	input += "}"

	return input
}
