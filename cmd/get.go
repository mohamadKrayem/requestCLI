/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

// command.Command is a struct that contains all the data entered by the user in a single command
// it can be found in command/parentCMD.go
var get_command command.Command = command.NewCommand()

// getCmd represents the get command
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Send a GET request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		get_command.Run(args, cmd)
		// get the current working directory
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		get_command.Method = "GET"
		get_command.Body = Command.Body
		get_command.Headers = Command.Headers
		get_command.Form = &Command.Form
		get_command.Sb = Command.ShowBody
		get_command.Sh = Command.ShowHeaders
		get_command.Ss = Command.ShowStatus
		get_command.QueryParams = &Command.QueryParams
		get_command.Https = Command.Https
		get_command.BodyJS = &Command.BodyJS
		get_command.HeadersJS = &Command.HeadersJS
		get_command.Headersjs = &Command.Headersjs
		get_command.Redirect = Command.Redirect
		get_command.Cookies = &Command.Cookies
		get_command.Multipart = Command.Multipart
		get_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		get_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(GetCmd)
}
