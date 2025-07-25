package ctfd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
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
		// Check for permission-related errors and skip them
		message := strings.ToLower(tmp.Message)
		if strings.Contains(message, "permission") ||
			strings.Contains(message, "access") ||
			strings.Contains(message, "readable") ||
			strings.Contains(message, "protected") ||
			strings.Contains(message, "forbidden") {
			// Skip permission errors - return empty data
			log.ErrorH2("permission error: %s", tmp.Message)
			return nil
		}
		return fmt.Errorf("request end with %s status", tmp.Message)
	}
	if err := json.Unmarshal(tmp.Data, data); err != nil {
		return err
	}
	return nil
}
