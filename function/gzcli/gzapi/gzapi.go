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

type gzapi struct {
	url    string
	creds  *Creds
	client *req.Client
}

var client *gzapi

func Init(url string, creds *Creds) (*gzapi, error) {
	url = strings.TrimRight(url, "/")
	newGz := New(url, creds)
	if err := newGz.login(); err != nil {
		return nil, err
	}
	return newGz, nil
}

func New(url string, creds *Creds) *gzapi {
	client = &gzapi{
		client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0"),
		url:   url,
		creds: creds,
	}
	return client
}

func (cs *gzapi) get(url string, data any) error {
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

func (cs *gzapi) delete(url string, data any) error {
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

func (cs *gzapi) post(url string, json any, data any) error {
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

func (cs *gzapi) postMultiPart(url string, file string, data any) error {
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

func (cs *gzapi) putMultiPart(url string, file string, data any) error {
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

func (cs *gzapi) put(url string, json any, data any) error {
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

func (cs *gzapi) login() error {
	if err := cs.post("/api/account/login", cs.creds, nil); err != nil {
		return err
	}
	return nil
}
