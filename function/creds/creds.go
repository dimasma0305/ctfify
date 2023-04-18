package creds

import (
	"github.com/go-playground/validator/v10"
)

type CredsStruct struct {
	Username string `validate:"required"`
	Password string `validate:"required"`
	Url      string `validate:"required,url"`
}

func (cs *CredsStruct) Validate() error {
	v := validator.New()
	if err := v.Struct(cs); err != nil {
		return err
	}
	return nil
}
