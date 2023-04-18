package ctfd

import (
	"os"
	"path/filepath"
	"strings"
)

type fileUrl string

// get filename from the url
func (fu *fileUrl) FileName() string {
	var (
		raw      = string(*fu)
		path     = strings.Split(raw, "?")[0]
		filename = filepath.Base(path)
	)
	return filename
}

// download file from ctfd platform
func (fu *fileUrl) DownloadFile() ([]byte, error) {
	res, err := scraper.client.R().Get(scraper.Url + string(*fu))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return res.Bytes(), nil
}

// download the file and put it into destination folder
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
