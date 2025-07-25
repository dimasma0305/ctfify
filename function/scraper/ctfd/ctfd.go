package ctfd

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/dimasma0305/ctfify/function/utils"
	"github.com/imroc/req/v3"
)

type Creds struct {
	Username string
	Password string
}

type ctfdScraper struct {
	Url           string
	creds         *Creds
	client        *req.Client
	challengesUrl string
	loginUrl      string
	hintsUrl      string
}

// ctfScraper struct global variable
var scraper *ctfdScraper

// Create a new ctfScraper and call Login method
func Init(url string, creds *Creds) (*ctfdScraper, error) {
	newCtf := New(url, creds)
	if err := newCtf.login(); err != nil {
		return nil, err
	}
	return newCtf, nil
}

// Create a New ctfScraper
func New(url string, creds *Creds) *ctfdScraper {
	challengeUrl := utils.UrlJoinPath(url, "/api/v1/challenges")
	hintsUrl := utils.UrlJoinPath(url, "/api/v1/hints")
	loginUrl := utils.UrlJoinPath(url, "/login")

	scraper = &ctfdScraper{
		client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0").
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
		Url:           url,
		challengesUrl: challengeUrl,
		hintsUrl:      hintsUrl,
		loginUrl:      loginUrl,
		creds:         creds,
	}
	return scraper
}

// login as user with username and password profided in Creds struct
func (cs *ctfdScraper) login() error {
	nonce, err := cs.getNonce()
	if err != nil {
		return err
	}
	res, err := cs.client.R().
		SetFormData(map[string]string{
			"name":     cs.creds.Username,
			"password": cs.creds.Password,
			"_submit":  "Submit",
			"nonce":    *nonce,
		}).
		Post(cs.loginUrl)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if strings.Contains(res.String(), "incorrect") {
		return (fmt.Errorf("invalid credential"))
	}
	return nil
}

// Get nonce from login page
func (cs *ctfdScraper) getNonce() (*string, error) {
	res, err := cs.client.R().Get(cs.loginUrl)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}
	nonce, exist := doc.Find("#nonce").Attr("value")
	if !exist {
		return nil, fmt.Errorf("nonce doesn't exist")
	}
	return &nonce, nil
}

// get all challenges from /api/v1/challenges in ctfd platform
func (cs *ctfdScraper) GetChallenges() (ChallengesInfo, error) {
	var (
		data ChallengesInfo
	)
	res, err := cs.client.R().Get(cs.challengesUrl)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := getData(res.Bytes(), &data); err != nil {
		return nil, err
	}
	return data, err
}

// get hostname from cs.Url
func (cs *ctfdScraper) HostName() string {
	res, _ := url.Parse(cs.Url)
	return res.Hostname()
}
