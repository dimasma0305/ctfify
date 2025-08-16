package gzapi

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
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
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0").
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
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
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/110.0").
			SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
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
	log.InfoH3("Making GET request to: %s", url)

	req, err := cs.Client.R().Get(url)
	if err != nil {
		log.Error("GET request failed for %s: %v", url, err)
		return fmt.Errorf("GET request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("GET request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("GET request successful for: %s", url)
	return nil
}

func (cs *GZAPI) delete(url string, data any) error {
	url = cs.Url + url
	log.InfoH3("Making DELETE request to: %s", url)

	req, err := cs.Client.R().Delete(url)
	if err != nil {
		log.Error("DELETE request failed for %s: %v", url, err)
		return fmt.Errorf("DELETE request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("DELETE request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("DELETE request successful for: %s", url)
	return nil
}

func (cs *GZAPI) post(url string, json any, data any) error {
	url = cs.Url + url
	log.InfoH3("Making POST request to: %s", url)

	req, err := cs.Client.R().SetBodyJsonMarshal(json).Post(url)
	if err != nil {
		log.Error("POST request failed for %s: %v", url, err)
		return fmt.Errorf("POST request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("POST request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("POST request successful for: %s", url)
	return nil
}

func (cs *GZAPI) postMultiPart(url string, file string, data any) error {
	url = cs.Url + url
	log.InfoH3("Making POST multipart request to: %s with file: %s", url, file)

	// Verify file exists before attempting upload
	if _, err := os.Stat(file); err != nil {
		log.Error("File does not exist: %s", file)
		return fmt.Errorf("file not found: %s", file)
	}

	// Use "files" for /api/assets endpoint as per API specification
	req, err := cs.Client.R().SetFile("files", file).Post(url)
	if err != nil {
		log.Error("POST multipart request failed for %s: %v", url, err)
		return fmt.Errorf("POST multipart request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("POST multipart request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("POST multipart request successful for: %s", url)
	return nil
}

func (cs *GZAPI) putMultiPart(url string, file string, data any) error {
	url = cs.Url + url
	log.InfoH3("Making PUT multipart request to: %s with file: %s", url, file)

	// Verify file exists before attempting upload
	if _, err := os.Stat(file); err != nil {
		log.Error("File does not exist: %s", file)
		return fmt.Errorf("file not found: %s", file)
	}

	// Use "file" for PUT operations (poster/avatar uploads) as per API specification
	req, err := cs.Client.R().SetFile("file", file).Put(url)
	if err != nil {
		log.Error("PUT multipart request failed for %s: %v", url, err)
		return fmt.Errorf("PUT multipart request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("PUT multipart request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("PUT multipart request successful for: %s", url)
	return nil
}

func (cs *GZAPI) put(url string, json any, data any) error {
	url = cs.Url + url
	log.InfoH3("Making PUT request to: %s", url)

	req, err := cs.Client.R().SetBodyJsonMarshal(json).Put(url)
	if err != nil {
		log.Error("PUT request failed for %s: %v", url, err)
		return fmt.Errorf("PUT request failed for %s: %w", url, err)
	}

	if req.StatusCode != 200 {
		log.Error("PUT request returned status %d for %s: %s", req.StatusCode, url, req.String())
		return fmt.Errorf("request end with %d status, %s", req.StatusCode, req.String())
	}

	if data != nil {
		if err := req.UnmarshalJson(&data); err != nil {
			log.Error("Failed to unmarshal JSON response from %s: %v", url, err)
			return fmt.Errorf("error unmarshal json: %w, %s", err, req.String())
		}
	}

	log.InfoH3("PUT request successful for: %s", url)
	return nil
}
