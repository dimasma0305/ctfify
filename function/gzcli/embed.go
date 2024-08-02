package gzcli

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed all:embeds/*
var embedTemplate embed.FS

func copyEmbedFile(src, dst string) error {
	srcFile, err := embedTemplate.Open(src)
	if err != nil {
		return fmt.Errorf("open embed file: %v", err)
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create file: %v", err)
	}
	dstFile.Chmod(0644)

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file: %v", err)
	}
	defer dstFile.Close()
	defer srcFile.Close()
	return nil
}

func copyAllEmbedFileIntoFolder(src, dst string) error {
	entries, err := embedTemplate.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read embed dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if err := copyAllEmbedFileIntoFolder(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return fmt.Errorf("copy embed dir: %v", err)
			}
		} else {
			if _, err := os.Stat(filepath.Join(dst, entry.Name())); os.IsNotExist(err) {
				if err := copyEmbedFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
					return fmt.Errorf("copy embed file: %v", err)
				}
			}
		}
	}
	return nil
}
