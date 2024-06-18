package gzcli

import (
	"embed"
	"io"
	"os"
	"path/filepath"
)

//go:embed all:embeds/*
var embedTemplate embed.FS

func copyEmbedFile(src, dst string) error {
	srcFile, err := embedTemplate.Open(src)
	if err != nil {
		return err
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	dstFile.Chmod(0644)

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	defer dstFile.Close()
	defer srcFile.Close()
	return nil
}

func copyAllEmbedFileIntoFolder(src, dst string) error {
	entries, err := embedTemplate.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if err := copyAllEmbedFileIntoFolder(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		} else {
			if _, err := os.Stat(filepath.Join(dst, entry.Name())); os.IsNotExist(err) {
				if err := copyEmbedFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
