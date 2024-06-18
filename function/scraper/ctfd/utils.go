package ctfd

import (
	"encoding/json"
	"fmt"
)

// Parse information from ctfd and get data response
func getData(byte []byte, data any) error {
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
