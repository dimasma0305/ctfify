package parser

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/lqqyt2423/go-mitmproxy/proxy"
)

func ParseBody(f *proxy.Flow) (interface{}, error) {
	contentType := f.Request.Header.Get("content-type")
	var data interface{}
	var err error
	switch {
	case strings.Contains(contentType, "application/json"):
		data, err = parseJSON(f)
	case strings.Contains(contentType, "application/xml"):
		data, err = parseXML(f)
	case strings.Contains(contentType, "application/x-www-form-urlencoded"):
		data, err = parseFormURLEncoded(f)
	case strings.Contains(contentType, "multipart/form-data"):
		data, err = parseMultipartFormData(f)
	case strings.Contains(contentType, "text/plain"):
		data, err = parseTextPlain(f)
	default:
		data = nil
		err = fmt.Errorf("the body didn't have a match parser %s", contentType)
	}
	return data, err
}

func parseJSON(f *proxy.Flow) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal(f.Request.Body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func parseXML(f *proxy.Flow) (interface{}, error) {
	var data interface{}
	if err := xml.Unmarshal(f.Request.Body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func parseFormURLEncoded(f *proxy.Flow) (interface{}, error) {
	values, err := url.ParseQuery(string(f.Request.Body))
	if err != nil {
		return nil, err
	}
	return values, nil
}

func parseMultipartFormData(f *proxy.Flow) (interface{}, error) {
	reader := multipart.NewReader(strings.NewReader(string(f.Request.Body)), f.Request.Header.Get("Content-Type"))
	form, err := reader.ReadForm(0)
	if err != nil {
		return nil, err
	}
	return form.Value, nil
}

func parseTextPlain(f *proxy.Flow) (string, error) {
	return string(f.Request.Body), nil
}
