package template

import (
	"bytes"
	"embed"
	"io/fs"
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
func GetWeb3() FileByte            { return templates("web3.py", "") }

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

func WriteTemplatesToFolder(sourceFolder, destinationFolder string, info any) error {
	err := filepath.WalkDir(sourceFolder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(sourceFolder, path)
		destinationPath := filepath.Join(destinationFolder, relPath)

		// Create destination directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
			return err
		}

		// Apply template and write to the destination file
		fileBytes := templates(relPath, info)
		if err := fileBytes.WriteToFile(destinationPath); err != nil {
			return err
		}

		return nil
	})

	return err
}
