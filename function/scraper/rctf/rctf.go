package rctf

import (
	"net/http"
	"net/url"

	"github.com/imroc/req/v3"
)

type RCTFScraper struct {
	Token string
	Url   *url.URL
	c     *req.Client
}

var rctfScraper *RCTFScraper

func Init(Url string, Token string) (*RCTFScraper, error) {
	var (
		client = req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0").
			SetRedirectPolicy(func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			})
		data struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
			Data    struct {
				AuthToken string `json:"authToken"`
			} `json:"data"`
		}
	)
	newUrl, err := url.Parse(Url)
	if err != nil {
		return nil, err
	}
	res, err := client.R().
		SetBodyJsonMarshal(map[string]string{
			"teamToken": Token,
		}).
		Post(newUrl.JoinPath("/api/v1/auth/login").String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := res.UnmarshalJson(&data); err != nil {
		return nil, err
	}

	rctfScraper = &RCTFScraper{
		Url:   newUrl,
		Token: Token,
		c:     client.SetCommonBearerAuthToken(data.Data.AuthToken),
	}
	return rctfScraper, nil
}
