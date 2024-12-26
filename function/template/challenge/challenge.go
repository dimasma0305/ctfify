package challenge

import "github.com/dimasma0305/ctfify/function/template"

func Web3(destination string) {
	template.TemplateToDestination("templates/challenges/web3", "", destination)
}
func XSS(destination string) {
	template.TemplateToDestination("templates/challenges/xss", "", destination)
}
func PHPFPM(destination string) {
	template.TemplateToDestination("templates/challenges/php-fpm", "", destination)
}
