package ctfd

import (
	"strconv"

	"github.com/dimasma0305/ctfify/function/utils"
)

type ChallengeInfo struct {
	Id           int
	Name         string
	Category     string
	Tags         []interface{}
	Solved_By_Me bool
}

// Get all info of the chall from ctfd plaform
func (cis *ChallengeInfo) GetFullInfo() (*ChallengeFullInfo, error) {
	var data ChallengeFullInfo
	res, err := scraper.client.R().Get(utils.UrlJoinPath(scraper.challengesUrl, strconv.Itoa(cis.Id)))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := getData(res.Bytes(), &data); err != nil {
		return nil, err
	}
	return &data, nil
}
