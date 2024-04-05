package rctf

import (
	"fmt"
	"os"
	"path/filepath"
)

type File struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type fileUrl File

func (fu *fileUrl) FileName() string {
	fmt.Println(fu.Name)
	return fu.Name
}

func (fu *fileUrl) DownloadFile() ([]byte, error) {
	res, err := rctfScraper.c.R().Get(rctfScraper.Url.JoinPath(fu.URL).String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return res.Bytes(), nil
}

func (fu *fileUrl) DowloadFileToDir(dstFolder string) error {
	data, err := fu.DownloadFile()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dstFolder, fu.FileName()), data, 0644); err != nil {
		return err
	}
	return nil
}
