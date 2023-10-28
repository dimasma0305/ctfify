/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/template"
	"github.com/spf13/cobra"
)

var addFlag struct {
	Name          string
	Category      string
	Connection    string
	Tags          []string
	TemplateToUse string
}

type info struct {
	name string
	desc string
}

var category = map[string]info{
	"web": {
		name: "web",
		desc: "Web Exploitation",
	},
	"webPwn": {
		name: "web-pwn",
		desc: "Web Exploitation With Extra PWN",
	},
	"pwn": {
		name: "pwn",
		desc: "PWN",
	},
	"writeup": {
		name: "writeup",
		desc: "Writeup",
	},
}

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "add something to current directory",
	Long: `This command to add something onto you current directory
it can be a --template like pwn template of writeup template
that i specialy crafted`,
	Run: func(cmd *cobra.Command, args []string) {
		switch addFlag.TemplateToUse {
		case category["writeup"].name:
			if err := template.GetWriteup(addFlag).WriteToFile("README.md"); err != nil {
				log.Fatal(err)
			}
		case category["pwn"].name:
			if err := template.GetPwn().
				WriteToFileWithPermissionExecutable("solve.py"); err != nil {
				log.Fatal(err)
			}
		case category["web"].name:
			if err := template.GetWeb().WriteToFile("solve.py"); err != nil {
				log.Fatal(err)
			}
		case category["webPwn"].name:
			if err := template.GetWebPWN().WriteToFile("solve.py"); err != nil {
				log.Fatal(err)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addFlag.Name, "name", "n", "{.Name}", "challenge name")
	addCmd.Flags().StringVarP(&addFlag.Category, "category", "c", "{.Category}", "challenge category")
	addCmd.Flags().StringVar(&addFlag.Connection, "connection", "{.Connection}", "challenge connection info")
	addCmd.Flags().StringSliceVar(&addFlag.Tags, "tags", []string{}, "challenge tags")
	addCmd.Flags().StringVar(&addFlag.TemplateToUse, "template", "", "make a template")
	if err := addCmd.RegisterFlagCompletionFunc("template", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		matches := make([]string, 0)
		for c, d := range category {
			if strings.HasPrefix(c, toComplete) {
				matches = append(matches, c+"\t"+d.desc)
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Fatal(err)
	}
}
