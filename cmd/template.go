/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/template"
	"github.com/spf13/cobra"
)

var templateFlags struct{ template.Options }

// templateCmd represents the template command
var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Add file template to curent directory",
	Run: func(cmd *cobra.Command, args []string) {
		if templateFlags.Pwn {
			if err := template.Get(&templateFlags.Options).
				WriteToFileWithPermisionExecutable("solve.py"); err != nil {
				log.Fatal(err)
			}
		}
	},
}

func init() {
	addCmd.AddCommand(templateCmd)
	templateCmd.Flags().BoolVar(&templateFlags.Pwn, "pwn", false, "make solve.py for pwn challenge")
}
