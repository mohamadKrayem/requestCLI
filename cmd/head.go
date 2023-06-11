/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var head_command command.Command = command.NewCommand()

// headCmd represents the head command
var headCmd = &cobra.Command{
	Use:   "head",
	Short: "Send a HEAD request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		head_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		head_command.Method = "HEAD"
		head_command.Body = Command.Body
		head_command.Headers = Command.Headers
		head_command.Form = &Command.Form
		head_command.Sb = Command.ShowBody
		head_command.Sh = Command.ShowHeaders
		head_command.Ss = Command.ShowStatus
		head_command.Https = Command.Https
		head_command.BodyJS = &Command.BodyJS
		head_command.HeadersJS = &Command.HeadersJS
		head_command.Headersjs = &Command.Headersjs
		head_command.Redirect = Command.Redirect
		head_command.QueryParams = &Command.QueryParams
		head_command.Cookies = &Command.Cookies
		head_command.Multipart = Command.Multipart
		head_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		head_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(headCmd)
}
