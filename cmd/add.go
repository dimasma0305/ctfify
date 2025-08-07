/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/template/challenge"
	"github.com/dimasma0305/ctfify/function/template/other"
	"github.com/dimasma0305/ctfify/function/template/solver"
	"github.com/spf13/cobra"
)

var addFlag struct {
	Name              string
	Destination       string
	TemplateSolver    string
	TemplateChallenge string
	TemplateOther     string
}

type info struct {
	name string
	desc string
}

var solverTemplateList = map[string]info{
	"web": {
		name: "web",
		desc: "Web Exploitation solver template",
	},
	"webPwn": {
		name: "webPwn",
		desc: "Web Exploitation With Extra PWN solver template",
	},
	"pwn": {
		name: "pwn",
		desc: "PWN solver template",
	},
	"web3": {
		name: "web3",
		desc: "Web3 solver template",
	},
	"webServer": {
		name: "webServer",
		desc: "Web Server template",
	},
}

var challengeTemplateList = map[string]info{
	"web3": {
		name: "web3",
		desc: "Web3 challenge template",
	},
	"xss": {
		name: "xss",
		desc: "XSS challenge template",
	},
	"php-fpm": {
		name: "php-fpm",
		desc: "php-fpm challenge template",
	},
}

var otherTemplateList = map[string]info{
	"readflag": {
		name: "readflag",
		desc: "readflag.c template",
	},
	"writeup": {
		name: "writeup",
		desc: "Writeup template",
	},
	"poc": {
		name: "poc",
		desc: "POC Template",
	},
	"java-exploitation-plus": {
		name: "java-exploitation-plus",
		desc: "Java Exploitation Framework",
	},
	"ctfTemplate": {
		name: "ctfTemplate",
		desc: "CTF Template",
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
		var errors [][]error // Slice to collect errors

		if addFlag.TemplateSolver != "" {
			switch addFlag.TemplateSolver {
			case solverTemplateList["writeup"].name:
				errors = append(errors, other.Writeup(addFlag.Destination, addFlag))
			case solverTemplateList["pwn"].name:
				errors = append(errors, solver.PWN(addFlag.Destination))
			case solverTemplateList["web"].name:
				errors = append(errors, solver.Web(addFlag.Destination))
			case solverTemplateList["webPwn"].name:
				errors = append(errors, solver.WebPWN(addFlag.Destination))
			case solverTemplateList["web3"].name:
				errors = append(errors, solver.Web3(addFlag.Destination))
			case solverTemplateList["webServer"].name:
				errors = append(errors, solver.WebServer(addFlag.Destination))
			}
		} else if addFlag.TemplateChallenge != "" {
			switch addFlag.TemplateChallenge {
			case challengeTemplateList["web3"].name:
				errors = append(errors, challenge.Web3(addFlag.Destination))
			case challengeTemplateList["xss"].name:
				errors = append(errors, challenge.XSS(addFlag.Destination))
			case challengeTemplateList["php-fpm"].name:
				errors = append(errors, challenge.PHPFPM(addFlag.Destination))
			}
		} else if addFlag.TemplateOther != "" {
			switch addFlag.TemplateOther {
			case otherTemplateList["readflag"].name:
				errors = append(errors, other.ReadFlag(addFlag.Destination))
			case otherTemplateList["writeup"].name:
				errors = append(errors, other.Writeup(addFlag.Destination, addFlag))
			case otherTemplateList["poc"].name:
				errors = append(errors, other.POC(addFlag.Destination, addFlag))
			case otherTemplateList["java-exploitation-plus"].name:
				errors = append(errors, other.JavaExploitationPlus(addFlag.Destination, addFlag))
			case otherTemplateList["ctfTemplate"].name:
				errors = append(errors, other.CTFTemplate(addFlag.Destination, addFlag))
			}
		}

		// Log all collected errors
		for _, errs := range errors {
			for _, err := range errs {
				if err != nil {
					log.Error("%s", err)
				}
			}
		}
	},
}

func completerBuilder(tmplList map[string]info) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		matches := make([]string, 0)
		for c, d := range tmplList {
			if strings.HasPrefix(c, toComplete) {
				matches = append(matches, c+"\t"+d.desc)
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&addFlag.Name, "name", "n", "{.Name}", "Name")
	addCmd.Flags().StringVarP(&addFlag.Destination, "destination", "t", ".", "destination")
	addCmd.Flags().StringVar(&addFlag.TemplateSolver, "solver", "", "solver template")
	addCmd.Flags().StringVar(&addFlag.TemplateChallenge, "challenge", "", "challenge template")
	addCmd.Flags().StringVar(&addFlag.TemplateOther, "other", "", "other template")
	if err := addCmd.RegisterFlagCompletionFunc("solver", completerBuilder(solverTemplateList)); err != nil {
		log.Fatal(err)
	}
	if err := addCmd.RegisterFlagCompletionFunc("challenge", completerBuilder(challengeTemplateList)); err != nil {
		log.Fatal(err)
	}
	if err := addCmd.RegisterFlagCompletionFunc("other", completerBuilder(otherTemplateList)); err != nil {
		log.Fatal(err)
	}
}
