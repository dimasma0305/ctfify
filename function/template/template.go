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
	File embed.FS
)

func GetPwn() FileByte             { return templates("pwn.py", "") }
func GetWriteup(info any) FileByte { return templates("writeup.md", info) }
func GetWeb() FileByte             { return templates("web.py", "") }
func GetWebPWN() FileByte          { return templates("webPwn.py", "") }

type FileByte []byte

func templates(file string, info any) FileByte {
	var buf bytes.Buffer
	fs, _ := template.ParseFS(File, filepath.Join("templates", file))
	err := fs.Execute(&buf, info)
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

func (o FileByte) WriteToFile(destination string) error {
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if err := os.WriteFile(destination, []byte(o), 0644); err != nil {
			return err
		}
	}
	return nil

}

func (o FileByte) WriteToFileWithPermissionExecutable(destination string) error {
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if err := os.WriteFile(destination, []byte(o), 0744); err != nil {
			return err
		}
	}
	return nil
}
