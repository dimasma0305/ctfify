package ctfd

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

// ChallengeFullInfo is a struct containing the full information about a CTFd challenge.
// It includes fields for the challenge ID, name, description, category, tags, value,
// connection info, type, solves, whether it has been solved by the user, and a list of file URLs.
type ChallengeFullInfo struct {
	Id              int
	Name            string
	Description     string
	Category        string
	Tags            []string
	Value           int
	Connection_Info string
	Type            string
	Solves          int
	SolvedByMe      bool
	Files           []fileUrl
}

var (
	// TemplateFile is an embed.FS containing all template files for CTFd challenges.
	//go:embed templates/*
	TemplateFile embed.FS

	// ChallengeDir is the path to the directory where templates for challenge are stored.
	ChallengeDir = "templates/challenge"

	// TemplateGetDir is the path to the directory where templates for get are stored.
	TemplateGetDir = "templates/templateGet"

	// WriteupDir is the path to the directory where writeup templates are stored.
	WriteupDir = "templates/writeup"
)

// Templater generates a template from a given source file and a ChallengeFullInfo struct.
func (cfi *ChallengeFullInfo) Templater(src string) ([]byte, error) {
	// Parse the template file from the source file path and generate the template
	file, err := template.ParseFS(TemplateFile, src)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := file.Execute(&buf, cfi); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteTemplateToFile generates a template from a source file and writes it to a destination file.
func (cfi *ChallengeFullInfo) WriteTemplateToFile(src string, dstFolder string) error {
	// Generate the template from the source file path
	srcData, err := cfi.Templater(src)
	if err != nil {
		return err
	}
	// Write the template to the destination file
	if err := os.WriteFile(dstFolder, srcData, 0644); err != nil {
		return err
	}
	return nil
}

// WriteTemplatesToDir generates and writes templates for an entire directory to a destination folder.
func (cfi *ChallengeFullInfo) WriteTemplatesToDir(src string, dstFolder string) error {
	// Read all files in the source directory and create the destination directory if it doesn't exist
	files, err := TemplateFile.ReadDir(src)
	os.MkdirAll(dstFolder, 0755)
	if err != nil {
		return err
	}
	// Loop through all files in the source directory and write the templates to the destination directory
	for _, file := range files {
		if file.Name() == "{{placeholder}}" {
			continue
		}
		if file.Name() == "attachment" && len(cfi.Files) == 0 {
			continue
		}
		newsrc := filepath.Join(src, file.Name())
		newdst := filepath.Join(dstFolder, file.Name())
		if file.IsDir() {
			if err := cfi.WriteTemplatesToDir(newsrc, newdst); err != nil {
				return err
			}
		} else {
			if err := cfi.WriteTemplateToFile(newsrc, newdst); err != nil {
				return err
			}
		}
	}
	return nil
}

// DownloadFiles download all file from the challenge to destination folder
func (cfi *ChallengeFullInfo) DownloadFilesToDir(dstFolder string) error {
	for _, file := range cfi.Files {
		if err := file.DowloadFileToDir(dstFolder); err != nil {
			return err
		}
	}
	return nil
}
