package template

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

var (
	//go:embed templates/*
	TemplateFile embed.FS
)

func GetPwn() fileByte             { return templater("pwn.py", "") }
func GetWriteup(info any) fileByte { return templater("writeup.md", info) }

type fileByte []byte

func templater(fsfile string, info any) fileByte {
	var buf bytes.Buffer
	fs, _ := template.ParseFS(TemplateFile, filepath.Join("templates", fsfile))
	fs.Execute(&buf, info)
	return buf.Bytes()
}

func (o fileByte) WriteToFile(dstfile string) error {
	if _, err := os.Stat(dstfile); os.IsNotExist(err) {
		if err := os.WriteFile(dstfile, []byte(o), 0644); err != nil {
			return err
		}
	}
	return nil

}

func (o fileByte) WriteToFileWithPermisionExecutable(dstfile string) error {
	if _, err := os.Stat(dstfile); os.IsNotExist(err) {
		if err := os.WriteFile(dstfile, []byte(o), 0744); err != nil {
			return err
		}
	}
	return nil
}
