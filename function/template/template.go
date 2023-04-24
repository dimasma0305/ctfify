package template

import (
	"embed"
	"io"
	"os"
)

var (
	//go:embed templates/*
	TemplateFile embed.FS
)

type Options struct {
	Pwn bool
	res []byte
}

func Get(op *Options) *Options {
	var res []byte
	if op.Pwn {
		fs, _ := TemplateFile.Open("templates/pwn.py")
		res, _ = io.ReadAll(fs)
		return &Options{res: res}
	}
	return nil
}

func (o *Options) WriteToFile(dstfile string) error {
	if _, err := os.Stat(dstfile); os.IsNotExist(err) {
		if err := os.WriteFile(dstfile, o.res, 0644); err != nil {
			return err
		}
	}
	return nil

}

func (o *Options) WriteToFileWithPermisionExecutable(dstfile string) error {
	if _, err := os.Stat(dstfile); os.IsNotExist(err) {
		if err := os.WriteFile(dstfile, o.res, 0744); err != nil {
			return err
		}
	}
	return nil
}
