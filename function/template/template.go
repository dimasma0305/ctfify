package template

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/utils"
)

var (
	//go:embed all:templates/*
	File embed.FS
)

func TemplateToDestinationThrowError(file string, info interface{}, destination string) {
	if err := TemplateToDestination(file, info, destination); err != nil {
		log.Fatal(err)
	}
}

// TemplateToDestination reads a template from the embedded file system and writes it to the destination.
// If it's a folder, it recursively writes its contents to the destination. If it's a file, it writes that file to the destination.
func TemplateToDestination(file string, info interface{}, destination string) error {
	// Check if the template is a directory
	dirEntries, err := File.ReadDir(file)
	if err == nil { // It's a directory
		return processDirectory(file, dirEntries, info, destination)
	}
	// It's a file, process the template
	return processFile(file, info, destination)
}

func processDirectory(directory string, dirEntries []os.DirEntry, info interface{}, destination string) error {
	// Create the destination directory
	err := os.MkdirAll(destination, os.ModePerm)
	if err != nil {
		return err
	}

	// Recursively process each file in the directory
	for _, entry := range dirEntries {
		entryPath := filepath.Join(directory, entry.Name())
		destPath := filepath.Join(destination, entry.Name())
		if err := TemplateToDestination(entryPath, info, destPath); err != nil {
			return err
		}
	}

	return nil
}

func processFile(file string, info interface{}, destination string) error {
	// Check if the destination file already exists
	if _, err := os.Stat(destination); err == nil {
		// File exists, return an error or handle it as needed
		return fmt.Errorf("destination file already exists: %s", destination)
	}

	// Parse the template
	tmpl, err := template.ParseFS(File, utils.NormalizePath(file))
	if err != nil {
		return err
	}

	// Execute the template with the provided info
	var outputBuffer bytes.Buffer
	if err := tmpl.Execute(&outputBuffer, info); err != nil {
		return err
	}

	// Write the result to the destination
	destFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, &outputBuffer); err != nil {
		return err
	}

	log.Info("Template written to destination: %s", destFile.Name())
	return nil
}
