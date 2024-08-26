package gzcli

import (
	"fmt"

	"github.com/dimasma0305/ctfify/function/log"
)

func (gz *GZ) DeleteAllUser() error {
	teams, err := gz.api.Teams()
	if err != nil {
		return err
	}
	for t := range teams {
		log.Info("deleting team %s", teams[t].Name)
		if err := teams[t].Delete(); err != nil {
			log.Error(err.Error())
		}
	}
	users, err := gz.api.Users()
	fmt.Println(users)
	if err != nil {
		return err
	}
	for i := range users {
		log.Info("deleting user %s", users[i].UserName)
		if err := users[i].Delete(); err != nil {
			log.Error(err.Error())
		}
	}
	return nil
}
