/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/templater"
	"github.com/spf13/cobra"
)

type addCmdFlags struct {
	Name       string
	Category   string
	Connection string
}

var addFlag addCmdFlags

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

	var (
		templateCmd = &cobra.Command{
			Use:   "template",
			Short: "Add template to curent directory",
		}
		pwnCmd = &cobra.Command{
			Use:   "pwn",
			Short: "template for pwn",
			Run: func(cmd *cobra.Command, args []string) {
				if err := templater.Get(templater.Options{Pwn: true}).
					WriteToFileWithPermisionExecutable("solve.py"); err != nil {
					log.Fatal(err)
				}
			},
		}
	)

	addCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(pwnCmd)
	addCmd.Flags().StringVarP(&addFlag.Name, "name", "n", "", "challenge name")
	addCmd.Flags().StringVarP(&addFlag.Category, "category", "c", "", "challenge category")
	addCmd.Flags().StringVar(&addFlag.Connection, "connection", "", "challenge connection info")
}
