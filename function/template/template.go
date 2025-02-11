package template

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/utils"
)

var (
	//go:embed all:templates
	File embed.FS
)

// TemplateToDestination reads a template from the embedded file system and writes it to the destination.
// If it's a folder, it recursively writes its contents to the destination. If it's a file, it writes that file to the destination.
func TemplateToDestination(file string, info interface{}, destination string) {
	if isDir(file) {
		processDir(file, info, destination)
	} else {
		if err := processFile(file, info, destination); err != nil {
			log.ErrorH2("%s", err)
		}
	}
}

// New function to check if the path is a directory
func isDir(path string) bool {
	dirEntries, err := File.ReadDir(filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, de := range dirEntries {
		if de.Name() == filepath.Base(path) && de.IsDir() {
			return true
		}
	}
	return false
}

// New function to process directories
func processDir(dir string, info interface{}, destination string) {
	if err := os.MkdirAll(destination, 0755); err != nil {
		log.ErrorH2("Failed to create directory %q: %v", destination, err)
		return
	}

	entries, err := File.ReadDir(dir)
	if err != nil {
		log.ErrorH2("Failed to read directory %q: %v", dir, err)
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(dir, entry.Name())
		destPath := filepath.Join(destination, entry.Name())
		TemplateToDestination(srcPath, info, destPath)
	}
}

// Updated processFile function
func processFile(file string, info interface{}, destination string) error {
	file = utils.NormalizePath(file)
	destination = strings.ReplaceAll(destination, "{{replaceit}}", "")

	content, err := processTemplate(file, info)
	if err != nil {
		log.Error("Falling back to raw file copy for %q: %v", file, err)
		rawFile, openErr := File.Open(file)
		if openErr != nil {
			return fmt.Errorf("failed to open raw file: %w (template error: %v)", openErr, err)
		}
		defer rawFile.Close()
		content = rawFile
	}

	if err := writeContent(destination, content); err != nil {
		return fmt.Errorf("failed to write to %q: %w", destination, err)
	}

	log.Info("File processed successfully: %s", destination)
	return nil
}

// New function to process templates
func processTemplate(file string, info interface{}) (io.Reader, error) {
	tmpl, err := template.ParseFS(File, file)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, info); err != nil {
		return nil, fmt.Errorf("template execute error: %w", err)
	}

	return strings.NewReader(buf.String()), nil
}

// New function to write content atomically
func writeContent(destination string, content io.Reader) error {
	destFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("file already exists")
		}
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, content); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}
