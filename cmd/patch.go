/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var patch_command command.Command = command.NewCommand()

// patchCmd represents the patch command
var patchCmd = &cobra.Command{
	Use:   "patch",
	Short: "Send a PATCH request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		patch_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		patch_command.Method = "PATCH"
		patch_command.Body = Command.Body
		patch_command.Headers = Command.Headers
		patch_command.Form = &Command.Form
		patch_command.Sb = Command.ShowBody
		patch_command.Sh = Command.ShowHeaders
		patch_command.Ss = Command.ShowStatus
		patch_command.Https = Command.Https
		patch_command.BodyJS = &Command.BodyJS
		patch_command.QueryParams = &Command.QueryParams
		patch_command.HeadersJS = &Command.HeadersJS
		patch_command.Headersjs = &Command.Headersjs
		patch_command.Redirect = Command.Redirect
		patch_command.Cookies = &Command.Cookies
		patch_command.Multipart = Command.Multipart
		patch_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		patch_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(patchCmd)
}
