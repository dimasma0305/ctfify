package gzcli

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
	"unicode"
)

// LeetSpeakMap defines the character replacements to make the username cooler
var LeetSpeakMap = map[rune]string{
	'a': "4",
	'e': "3",
	'i': "1",
	'o': "0",
	's': "5",
	't': "7",
	'g': "9",
}

// transformRandomly applies leetspeak transformation randomly and converts characters to uppercase randomly
func transformRandomly(s string) string {
	rand.Seed(time.Now().UnixNano())
	var transformed strings.Builder
	for _, r := range s {
		// Replace space with underscore
		if r == ' ' {
			transformed.WriteRune('_')
			continue
		}

		// Randomly decide to apply leetspeak transformation
		if replacement, exists := LeetSpeakMap[r]; exists && rand.Intn(2) == 0 {
			transformed.WriteString(replacement)
		} else {
			transformed.WriteRune(r)
		}

		// Randomly decide to capitalize the character
		if rand.Intn(2) == 0 {
			transformedStr := transformed.String()
			lastChar := strings.ToUpper(string(transformedStr[len(transformedStr)-1]))
			transformedStr = transformedStr[:len(transformedStr)-1] + lastChar
			transformed.Reset()
			transformed.WriteString(transformedStr)
		}
	}
	return transformed.String()
}

// generateUsername generates a unique and cooler username based on the real name and the specified max length.
func generateUsername(realName string, maxLength int, existingUsernames map[string]struct{}) (string, error) {
	// Convert realName to lowercase
	baseUsername := strings.ToLower(realName)

	// Remove non-alphanumeric characters except spaces
	var usernameBuilder strings.Builder
	for _, r := range baseUsername {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			usernameBuilder.WriteRune(r)
		}
	}
	baseUsername = usernameBuilder.String()

	// Apply random transformations: leetspeak, underscores, and random uppercase
	baseUsername = transformRandomly(baseUsername)

	// Truncate if necessary to ensure it doesn't exceed maxLength
	if len(baseUsername) > maxLength {
		baseUsername = baseUsername[:maxLength]
	}

	// Check for uniqueness and generate a unique username if needed
	username := baseUsername
	i := 1
	for {
		if _, exists := existingUsernames[username]; !exists {
			break
		}
		// Append a numeric suffix to ensure uniqueness
		suffix := fmt.Sprintf("%d", i)
		if len(baseUsername)+len(suffix) > maxLength {
			username = baseUsername[:maxLength-len(suffix)] + suffix
		} else {
			username = baseUsername + suffix
		}
		i++
	}

	// Add the final username to the map of existing usernames
	existingUsernames[username] = struct{}{}

	return username, nil
}

// normalizeTeamName ensures the team name doesn't exceed the max length and is unique by appending a numeric suffix if needed.
func normalizeTeamName(teamName string, maxLength int, existingTeamNames map[string]struct{}) string {
	// Truncate the team name if it exceeds the maxLength
	if len(teamName) > maxLength {
		teamName = teamName[:maxLength]
	}

	// Ensure uniqueness by appending a numeric suffix if necessary
	i := 1
	uniqueTeamName := teamName
	for {
		if _, exists := existingTeamNames[uniqueTeamName]; !exists {
			break
		}
		// Append suffix if name is not unique
		suffix := fmt.Sprintf("_%d", i)
		if len(teamName)+len(suffix) > maxLength {
			uniqueTeamName = teamName[:maxLength-len(suffix)] + suffix
		} else {
			uniqueTeamName = teamName + suffix
		}
		i++
	}

	// Add the unique team name to the set of existing team names
	existingTeamNames[uniqueTeamName] = struct{}{}

	return uniqueTeamName
}
