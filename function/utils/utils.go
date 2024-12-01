package utils

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// normalize path for windows compability
func NormalizePath(str string) string {
	str = strings.ReplaceAll(str, "\\", "/")
	return str
}

func GetJson(byte []byte, data any) error {
	var tmp struct {
		Message string
		Success bool
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(byte, &tmp); err != nil {
		return err
	}
	if !tmp.Success {
		return fmt.Errorf("request end with %s status", tmp.Message)
	}
	if err := json.Unmarshal(tmp.Data, data); err != nil {
		return err
	}
	return nil
}

// marshall object to json
func Jsonify(data any) ([]byte, error) {
	return json.Marshal(data)
}

func UrlJoinPath(base string, path ...string) string {
	res, err := url.JoinPath(base, path...)
	if err != nil {
		panic(err)
	}
	return res
}
