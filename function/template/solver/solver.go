package solver

import (
	"github.com/dimasma0305/ctfify/function/template"
)

func PWN(destination string) {
	template.TemplateToDestination("templates/solver/pwn", "", destination)
}
func Web(destination string) {
	template.TemplateToDestination("templates/solver/web", "", destination)
}
func Web3(destination string) {
	template.TemplateToDestination("templates/solver/web3", "", destination)
}
func WebPWN(destination string) {
	template.TemplateToDestination("templates/solver/webPwn", "", destination)
}
func WebServer(destination string) {
	template.TemplateToDestination("templates/solver/webServer", "", destination)
}
