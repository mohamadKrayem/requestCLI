/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

// command.Command is a struct that contains all the data entered by the user in a single command
// it can be found in command/parentCMD.go
var options_command command.Command = command.NewCommand()

// optionsCmd represents the options command
var optionsCmd = &cobra.Command{
	Use:   "options",
	Short: "Send an OPTIONS request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		options_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		options_command.Method = "OPTIONS"
		options_command.Body = Command.Body
		options_command.Headers = Command.Headers
		options_command.Form = &Command.Form
		options_command.Sb = Command.ShowBody
		options_command.Sh = Command.ShowHeaders
		options_command.Ss = Command.ShowStatus
		options_command.QueryParams = &Command.QueryParams
		options_command.Https = Command.Https
		options_command.BodyJS = &Command.BodyJS
		options_command.HeadersJS = &Command.HeadersJS
		options_command.Headersjs = &Command.Headersjs
		options_command.Redirect = Command.Redirect
		options_command.Cookies = &Command.Cookies
		options_command.Multipart = Command.Multipart
		options_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		options_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(optionsCmd)
}
