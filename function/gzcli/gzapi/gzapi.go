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

type GZAPI struct {
	Url    string
	Creds  *Creds
	Client *req.Client
}

func Init(url string, creds *Creds) (*GZAPI, error) {
	url = strings.TrimRight(url, "/")
	newGz := &GZAPI{
		Client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0"),
		Url:   url,
		Creds: creds,
	}
	if err := newGz.Login(); err != nil {
		return nil, err
	}
	return newGz, nil
}

func Register(url string, creds *RegisterForm) (*GZAPI, error) {
	url = strings.TrimRight(url, "/")
	newGz := &GZAPI{
		Client: req.C().
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0"),
		Url: url,
		Creds: &Creds{
			Username: creds.Username,
			Password: creds.Password,
		},
	}
	if err := newGz.Register(creds); err != nil {
		return nil, err
	}
	return newGz, nil
}

func (cs *GZAPI) get(url string, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().Get(url)
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

func (cs *GZAPI) delete(url string, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().Delete(url)
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

func (cs *GZAPI) post(url string, json any, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().SetBodyJsonMarshal(json).Post(url)
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

func (cs *GZAPI) postMultiPart(url string, file string, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().SetFile("files", file).Post(url)
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

func (cs *GZAPI) putMultiPart(url string, file string, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().SetFile("file", file).Put(url)
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

func (cs *GZAPI) put(url string, json any, data any) error {
	url = cs.Url + url
	req, err := cs.Client.R().SetBodyJsonMarshal(json).Put(url)
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
