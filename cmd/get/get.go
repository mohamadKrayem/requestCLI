/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package get

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	https bool
	url   string
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
	Run: func(cmd *cobra.Command, args []string) {
		url = args[0]
		if https {
			if !strings.Contains(url, "https://") {
				url = "https://" + url
			}
		} else {
			if !strings.Contains(url, "http://") {
				url = "http://" + url
			}
		}
		fmt.Println(url)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// getCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// getCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	GetCmd.Flags().BoolVarP(&https, "secure", "s", false, "http or https")
}

func getLink(url string) {

}
