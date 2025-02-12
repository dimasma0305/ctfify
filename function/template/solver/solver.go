package solver

import (
	"github.com/dimasma0305/ctfify/function/template"
)

func PWN(destination string) []error {
	return template.TemplateFSToDestination("templates/solver/pwn", "", destination)
}

func Web(destination string) []error {
	return template.TemplateFSToDestination("templates/solver/web", "", destination)
}

func Web3(destination string) []error {
	return template.TemplateFSToDestination("templates/solver/web3", "", destination)
}

func WebPWN(destination string) []error {
	return template.TemplateFSToDestination("templates/solver/webPwn", "", destination)
}

func WebServer(destination string) []error {
	return template.TemplateFSToDestination("templates/solver/webServer", "", destination)
}
