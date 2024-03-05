package other

import "github.com/dimasma0305/ctfify/function/template"

func ReadFlag(destination string) {
	template.TemplateToDestinationThrowError("templates/others/readflag", "", destination)
}

func Writeup(destination string, info any) {
	template.TemplateToDestinationThrowError("templates/others/writeup", info, destination)
}

func POC(destination string, info any) {
	template.TemplateToDestinationThrowError("templates/others/poc", info, destination)
}
