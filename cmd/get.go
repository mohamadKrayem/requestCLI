/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	json "github.com/mohamadkrayem/requestCLI/formats"
	rq "github.com/mohamadkrayem/requestCLI/requests"
	"github.com/spf13/cobra"
	//"strings"
)

var (
	//https bool
	url       string
	headersjs json.Json
)

// getCmd represents the get command
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PersistentPreRun: PersistentPreRun,
	Run: func(cmd *cobra.Command, args []string) {
		request := &rq.BaseRequest{
			Method: "get",
		}
		if bodyJS != "" {
			fmt.Println(bodyJS)
			request.Body = bodyJS
		}

		if headersJS != nil {
			request.WithHeadersMap(&headersJS)
		} else if headersjs != "" {
			request.WithHeaders(headersjs)
		}

		request.URL = rq.GenerateUrl(args[0], https, nil)

		resp, err := request.Send()
		if err != nil {
			panic(err)
		}
		resp.PrintResponse()
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
}

func PersistentPreRun(cmd *cobra.Command, args []string) {
	if !body && !headers {
		return
	}
	if body && bodyJS == "" {
		bodyJS = scanRequest()
	}

	if headers && headersJS == nil {
		headersjs, _ = json.NewJson(scanRequest())
	} else if !headers && headersJS != nil {
		headersjs, _ = json.ToJSON(headersJS)
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
