package rctf

import (
	"fmt"

	"github.com/dimasma0305/ctfify/function/scraper/templater"
)

type Challenges struct {
	Kind    string          `json:"kind"`
	Message string          `json:"message"`
	Data    []ChallengeData `json:"data"`
}

type ChallengeData struct {
	Files       []fileUrl `json:"files"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	Points      int       `json:"points"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Solves      int       `json:"solves"`
}

func (r *RCTFScraper) GetChalls() (*Challenges, error) {
	var challs *Challenges
	res, err := r.c.R().Get(r.Url.JoinPath("/api/v1/challs").String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	fmt.Println(string(res.Bytes()))
	if err := res.Unmarshal(&challs); err != nil {
		return nil, err
	}
	return challs, nil
}

// Same as WriteTemplatesToDir but use default directory `template`
func (c *ChallengeData) WriteTemplatesToDirDefault(dstFolder string) error {
	return templater.WriteTemplatesToDirRCTF(dstFolder, c)
}

// DownloadFiles download all file from the challenge to destination folder
func (c *ChallengeData) DownloadFilesToDir(dstFolder string) error {
	for _, file := range c.Files {
		if err := file.DowloadFileToDir(dstFolder); err != nil {
			return err
		}
	}
	return nil
}
