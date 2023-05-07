package ctfd

import (
	"github.com/dimasma0305/ctfify/function/scraper/templater"
)

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

// Same as WriteTemplatesToDir but use default directory `template`
func (cfi *ChallengeFullInfo) WriteTemplatesToDirDefault(dstFolder string) error {
	return templater.WriteTemplatesToDirCTFD(dstFolder, cfi)
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
