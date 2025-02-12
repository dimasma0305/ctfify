package challenge

import "github.com/dimasma0305/ctfify/function/template"

func Web3(destination string) []error {
	return template.TemplateFSToDestination("templates/challenges/web3", "", destination)
}

func XSS(destination string) []error {
	return template.TemplateFSToDestination("templates/challenges/xss", "", destination)
}

func PHPFPM(destination string) []error {
	return template.TemplateFSToDestination("templates/challenges/php-fpm", "", destination)
}
