package solver

import (
	"github.com/dimasma0305/ctfify/function/template"
)

func PWN(destination string) {
	template.TemplateToDestinationThrowError("templates/solver/pwn", "", destination)
}
func Web(destination string) {
	template.TemplateToDestinationThrowError("templates/solver/web", "", destination)
}
func Web3(destination string) {
	template.TemplateToDestinationThrowError("templates/solver/web3", "", destination)
}
func WebPWN(destination string) {
	template.TemplateToDestinationThrowError("templates/solver/webPwn", "", destination)
}
