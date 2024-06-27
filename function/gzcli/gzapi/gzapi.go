package gzapi

import (
	"fmt"
	"strings"

	"github.com/imroc/req/v3"
)

type Creds struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

type API struct {
	url    string
	creds  *Creds
	client *req.Client
}

var client *API

func Init(url string, creds *Creds) (*API, error) {
	url = strings.TrimRight(url, "/")
	newGz := &API{
		client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0"),
		url:   url,
		creds: creds,
	}
	client = newGz
	if err := newGz.login(); err != nil {
		return nil, err
	}
	return newGz, nil
}

func (cs *API) get(url string, data any) error {
	url = client.url + url
	req, err := cs.client.R().Get(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) delete(url string, data any) error {
	url = client.url + url
	req, err := cs.client.R().Delete(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) post(url string, json any, data any) error {
	url = client.url + url
	req, err := cs.client.R().SetBodyJsonMarshal(json).Post(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) postMultiPart(url string, file string, data any) error {
	url = client.url + url
	req, err := cs.client.R().SetFile("files", file).Post(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) putMultiPart(url string, file string, data any) error {
	url = client.url + url
	req, err := cs.client.R().SetFile("file", file).Put(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) put(url string, json any, data any) error {
	url = client.url + url
	req, err := cs.client.R().SetBodyJsonMarshal(json).Put(url)
	if err != nil {
		return err
	}
	if req.StatusCode != 200 {
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}
	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}
	return nil
}

func (cs *API) login() error {
	if err := cs.post("/api/account/login", cs.creds, nil); err != nil {
		return err
	}
	return nil
}
