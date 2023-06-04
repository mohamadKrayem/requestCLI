/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var del_command command.Command = command.NewCommand()

// delCmd represents the del command
var delCmd = &cobra.Command{
	Use:   "del",
	Short: "Send a DELETE request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		del_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		del_command.Method = "GET"
		del_command.Body = Command.Body
		del_command.Headers = Command.Headers
		del_command.Form = &Command.Form
		del_command.Sb = Command.ShowBody
		del_command.Sh = Command.ShowHeaders
		del_command.Ss = Command.ShowStatus
		del_command.Https = Command.Https
		del_command.BodyJS = &Command.BodyJS
		del_command.HeadersJS = &Command.HeadersJS
		del_command.Headersjs = &Command.Headersjs
		del_command.Redirect = Command.Redirect
		del_command.Cookies = &Command.Cookies
		del_command.Multipart = Command.Multipart
		del_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		del_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(delCmd)
}
