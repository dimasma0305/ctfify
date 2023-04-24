/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var addFlag struct {
	Name       string
	Category   string
	Connection string
}

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addFlag.Name, "name", "n", "", "challenge name")
	addCmd.Flags().StringVarP(&addFlag.Category, "category", "c", "", "challenge category")
	addCmd.Flags().StringVar(&addFlag.Connection, "connection", "", "challenge connection info")
}
