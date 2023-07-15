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
var connect_command command.Command = command.NewCommand()

// ConnectCmd represents the get command
var ConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Send a connect request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		connect_command.Run(args, cmd)
		// connect the current working directory
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		connect_command.Method = "CONNECT"
		connect_command.Body = Command.Body
		connect_command.Headers = Command.Headers
		connect_command.Form = &Command.Form
		connect_command.Sb = Command.ShowBody
		connect_command.Sh = Command.ShowHeaders
		connect_command.Ss = Command.ShowStatus
		connect_command.QueryParams = &Command.QueryParams
		connect_command.Https = Command.Https
		connect_command.BodyJS = &Command.BodyJS
		connect_command.HeadersJS = &Command.HeadersJS
		connect_command.Headersjs = &Command.Headersjs
		connect_command.Redirect = Command.Redirect
		connect_command.Cookies = &Command.Cookies
		connect_command.Multipart = Command.Multipart
		connect_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		connect_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(ConnectCmd)
}
