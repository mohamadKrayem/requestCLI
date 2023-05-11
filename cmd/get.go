/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	"fmt"

	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var get_command command.Command = command.NewCommand()

// getCmd represents the get command
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("hello")
		get_command.Method = "GET"
		get_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		get_command.Body = Body
		get_command.Headers = Headers
		get_command.Form = &Form
		get_command.Sb = ShowBody
		get_command.Sh = ShowHeaders
		get_command.Ss = ShowStatus
		get_command.Https = Https
		get_command.BodyJS = &BodyJS
		get_command.HeadersJS = &HeadersJS
		get_command.Headersjs = &Headersjs
		get_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(GetCmd)
}
