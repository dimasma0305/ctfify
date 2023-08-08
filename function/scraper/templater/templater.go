package templater

import (
	"bytes"
	"embed"
	"html/template"
	"os"
	"path/filepath"
)

var (
	//go:embed template/*
	TemplateFile embed.FS
)

func templater(src string, obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	file, err := template.ParseFS(TemplateFile, src)
	if err != nil {
		return nil, err
	}
	if err := file.Execute(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeTemplateToFile(src string, dstFolder string, obj interface{}) error {
	srcData, err := templater(src, obj)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dstFolder, srcData, 0644); err != nil {
		return err
	}
	return nil
}

func writeTemplatesToDir(src string, dstFolder string, obj interface{}) error {
	files, err := TemplateFile.ReadDir(src)
	os.MkdirAll(dstFolder, 0755)
	if err != nil {
		return err
	}
	for _, file := range files {
		// check if the file has {{placeholder}} name, if true then next
		if file.Name() == "{{placeholder}}" {
			continue
		}
		newsrc := filepath.Join(src, file.Name())
		newdst := filepath.Join(dstFolder, file.Name())
		if file.IsDir() {
			if err := writeTemplatesToDir(newsrc, newdst, obj); err != nil {
				return err
			}
		} else {
			if err := writeTemplateToFile(newsrc, newdst, obj); err != nil {
				return err
			}
		}
	}
	return nil
}

// make a template from /template embed fs, and paste it in dstFolder
func WriteTemplatesToDirCTFD(dstFolder string, obj interface{}) error {
	if err := writeTemplatesToDir("template/ctfd", dstFolder, &obj); err != nil {
		return err
	}
	return nil
}

// make a template from /template embed fs, and paste it in dstFolder
func WriteTemplatesToDirRCTF(dstFolder string, obj interface{}) error {
	if err := writeTemplatesToDir("template/rctf", dstFolder, &obj); err != nil {
		return err
	}
	return nil
}
