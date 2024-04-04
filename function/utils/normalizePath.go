package utils

import "strings"

// normalize path for windows compability
func NormalizePath(str string) string {
	return strings.ReplaceAll(str, "\\", "/")
}
