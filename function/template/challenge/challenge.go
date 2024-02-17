package challenge

import "github.com/dimasma0305/ctfify/function/template"

func Web3(destination string) {
	template.TemplateToDestinationThrowError("templates/challenges/web3", "", destination)
}
func XSS(destination string) {
	template.TemplateToDestinationThrowError("templates/challenges/xss", "", destination)
}
func PHPFPM(destination string) {
	template.TemplateToDestinationThrowError("templates/challenges/php-fpm", "", destination)
}
