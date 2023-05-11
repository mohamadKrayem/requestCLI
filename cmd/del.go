/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var del_command command.Command = command.Command{
	&BodyJS,
	"DELETE",
	&Headersjs,
	&HeadersJS,
	Https,
	ShowStatus,
	ShowHeaders,
	ShowBody,
	&Form,
	Body,
	Headers,
}

// delCmd represents the del command
var delCmd = &cobra.Command{
	Use:   "del",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		del_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		del_command.Method = "DELETE"
		del_command.Body = Body
		del_command.Headers = Headers
		del_command.Form = &Form
		del_command.Sb = ShowBody
		del_command.Sh = ShowHeaders
		del_command.Ss = ShowStatus
		del_command.Https = Https
		del_command.BodyJS = &BodyJS
		del_command.HeadersJS = &HeadersJS
		del_command.Headersjs = &Headersjs
		del_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(delCmd)
}
