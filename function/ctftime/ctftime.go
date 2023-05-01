package ctftime

// https://ctftime.org/api/v1/events/?limit=100&start=1622019499&finish=1623029499

import (
	"net/http"

	"github.com/imroc/req/v3"
)

type ctftimeApi struct {
	url    string
	client *req.Client
}

var api *ctftimeApi

func Init() *ctftimeApi {
	api = &ctftimeApi{
		url: "https://ctftime.org/api/v1",
		client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0").
			SetRedirectPolicy(func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}),
	}
	return api
}
