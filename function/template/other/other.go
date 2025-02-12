package other

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/dimasma0305/ctfify/function/template"
)

func ReadFlag(destination string) []error {
	return template.TemplateFSToDestination("templates/others/readflag", "", destination)
}

func Writeup(destination string, info any) []error {
	return template.TemplateFSToDestination("templates/others/writeup", info, destination)
}

func POC(destination string, info any) []error {
	return template.TemplateFSToDestination("templates/others/poc", info, destination)
}

func JavaExploitationPlus(destination string, info any) []error {
	return template.TemplateFSToDestination("templates/others/java-exploit-plus", info, destination)
}

type CTFInfo struct {
	XorKey         string
	PublicEntry    string
	DiscordWebhook string
	Url            string
	Username       string
	Password       string
}

func randomize(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func getUserInput(str string) string {
	var input string
	fmt.Print(str)
	fmt.Scanln(&input)
	return input
}

func CTFTemplate(destination string, info any) []error {
	url := getUserInput("URL: ")
	publicEntry := getUserInput("Public Entry: ")
	discordWebhook := getUserInput("Discord Webhook: ")
	ctfInfo := &CTFInfo{
		XorKey:         randomize(16),
		Username:       "admin",
		Password:       "ADMIN" + randomize(16) + "ADMIN",
		Url:            url,
		PublicEntry:    publicEntry,
		DiscordWebhook: discordWebhook,
	}
	return template.TemplateFSToDestination("templates/others/ctf-template", ctfInfo, destination)
}
