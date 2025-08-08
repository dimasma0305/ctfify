package gzcli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
)

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

func findCurrentGame(games []*gzapi.Game, title string, api *gzapi.GZAPI) *gzapi.Game {
	for _, game := range games {
		if game.Title == title {
			game.CS = api
			return game
		}
	}
	return nil
}

func createNewGame(config *Config, api *gzapi.GZAPI) (*gzapi.Game, error) {
	log.Info("Create new game")
	event := gzapi.CreateGameForm{
		Title: config.Event.Title,
		Start: config.Event.Start.Time,
		End:   config.Event.End.Time,
	}
	game, err := api.CreateGame(event)
	if err != nil {
		return nil, err
	}
	if config.Event.Poster == "" {
		return nil, fmt.Errorf("poster is required")
	}

	poster, err := createPosterIfNotExistOrDifferent(config.Event.Poster, game, api)
	if err != nil {
		return nil, err
	}

	config.Event.Id = game.Id
	config.Event.PublicKey = game.PublicKey
	config.Event.Poster = poster
	if err := game.Update(&config.Event); err != nil {
		return nil, err
	}
	if err := setCache("config", config); err != nil {
		return nil, err
	}
	return game, nil
}

func updateGameIfNeeded(config *Config, currentGame *gzapi.Game, api *gzapi.GZAPI) error {
	poster, err := createPosterIfNotExistOrDifferent(config.Event.Poster, currentGame, api)
	if err != nil {
		return err
	}
	config.Event.Poster = poster
	if fmt.Sprintf("%v", config.Event) != fmt.Sprintf("%v", *currentGame) {
		log.Info("Updated %s game", config.Event.Title)

		config.Event.Id = currentGame.Id
		config.Event.PublicKey = currentGame.PublicKey

		if err := currentGame.Update(&config.Event); err != nil {
			return err
		}
		if err := setCache("config", config); err != nil {
			return err
		}
	}
	return nil
}

func validateChallenges(challengesConf []ChallengeYaml) error {
	// Track seen names and duplicate occurrences
	seenNames := make(map[string]int, len(challengesConf))
	var duplicates []string

	// First pass: count occurrences
	for _, challengeConf := range challengesConf {
		seenNames[challengeConf.Name]++
	}

	// Collect names with duplicates
	for name, count := range seenNames {
		if count > 1 {
			duplicates = append(duplicates, name)
		}
	}

	// Return all duplicates at once
	if len(duplicates) > 0 {
		return fmt.Errorf("multiple challenges with the same name found:\n  - %s",
			strings.Join(duplicates, "\n  - "))
	}

	// Existing validation logic
	for _, challengeConf := range challengesConf {
		if challengeConf.Type == "" {
			challengeConf.Type = "StaticAttachments"
		}
		log.Info("Validating %s challenge...", challengeConf.Cwd)
		if err := isGoodChallenge(challengeConf); err != nil {
			return fmt.Errorf("invalid challenge %q: %w", challengeConf.Name, err)
		}
		log.Info("Challenge %s is valid.", challengeConf.Cwd)
	}

	return nil
}

func syncChallenge(config *Config, challengeConf ChallengeYaml, challenges []gzapi.Challenge, api *gzapi.GZAPI) error {
	var challengeData *gzapi.Challenge
	var err error

	log.InfoH2("Starting sync for challenge: %s (Type: %s, Category: %s)", challengeConf.Name, challengeConf.Type, challengeConf.Category)

	// Fetch fresh challenges list to prevent race conditions with parallel sync
	freshChallenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("Failed to get fresh challenges list for %s: %v", challengeConf.Name, err)
		// Fallback to original challenges list if fresh fetch fails
		freshChallenges = challenges
	} else {
		log.InfoH3("Fetched fresh challenges list for %s (%d challenges)", challengeConf.Name, len(freshChallenges))
	}

	if !isChallengeExist(challengeConf.Name, freshChallenges) {
		log.InfoH2("Creating new challenge: %s", challengeConf.Name)
		challengeData, err = config.Event.CreateChallenge(gzapi.CreateChallengeForm{
			Title:    challengeConf.Name,
			Category: challengeConf.Category,
			Tag:      challengeConf.Category,
			Type:     challengeConf.Type,
		})
		if err != nil {
			log.Error("Failed to create challenge %s: %v", challengeConf.Name, err)
			return fmt.Errorf("create challenge %s: %w", challengeConf.Name, err)
		}
		log.InfoH2("Successfully created challenge: %s (ID: %d)", challengeConf.Name, challengeData.Id)
	} else {
		log.InfoH2("Updating existing challenge: %s", challengeConf.Name)
		if err = GetCache(challengeConf.Category+"/"+challengeConf.Name+"/challenge", &challengeData); err != nil {
			log.InfoH3("Cache miss for %s, fetching from API", challengeConf.Name)
			challengeData, err = config.Event.GetChallenge(challengeConf.Name)
			if err != nil {
				log.Error("Failed to get challenge %s from API: %v", challengeConf.Name, err)
				return fmt.Errorf("get challenge %s: %w", challengeConf.Name, err)
			}
			log.InfoH3("Successfully fetched challenge %s from API", challengeConf.Name)
		} else {
			log.InfoH3("Found challenge %s in cache", challengeConf.Name)
		}

		// fix bug nill pointer because cache didn't return gzapi
		challengeData.CS = api
		// fix bug isEnable always be false after sync
		challengeData.IsEnabled = nil
	}

	log.InfoH2("Processing attachments for %s", challengeConf.Name)
	err = handleChallengeAttachments(challengeConf, challengeData, api)
	if err != nil {
		log.Error("Failed to handle attachments for %s: %v", challengeConf.Name, err)
		return fmt.Errorf("attachment handling failed for %s: %w", challengeConf.Name, err)
	}
	log.InfoH2("Attachments processed successfully for %s", challengeConf.Name)

	log.InfoH2("Updating flags for %s", challengeConf.Name)
	err = updateChallengeFlags(config, challengeConf, challengeData)
	if err != nil {
		log.Error("Failed to update flags for %s: %v", challengeConf.Name, err)
		return fmt.Errorf("update flags for %s: %w", challengeConf.Name, err)
	}
	log.InfoH2("Flags updated successfully for %s", challengeConf.Name)

	log.InfoH2("Merging challenge data for %s", challengeConf.Name)
	challengeData = mergeChallengeData(&challengeConf, challengeData)

	if isConfigEdited(&challengeConf, challengeData) {
		log.InfoH2("Configuration changed for %s, updating...", challengeConf.Name)
		if challengeData, err = challengeData.Update(*challengeData); err != nil {
			log.Error("Update failed for %s: %v", challengeConf.Name, err.Error())
			if strings.Contains(err.Error(), "404") {
				log.InfoH3("Got 404 error, refreshing challenge data for %s", challengeConf.Name)
				challengeData, err = config.Event.GetChallenge(challengeConf.Name)
				if err != nil {
					log.Error("Failed to get challenge %s after 404: %v", challengeConf.Name, err)
					return fmt.Errorf("get challenge %s: %w", challengeConf.Name, err)
				}
				log.InfoH3("Retrying update for %s", challengeConf.Name)
				challengeData, err = challengeData.Update(*challengeData)
				if err != nil {
					log.Error("Update retry failed for %s: %v", challengeConf.Name, err)
					return fmt.Errorf("update challenge %s: %w", challengeConf.Name, err)
				}
			} else {
				return fmt.Errorf("update challenge %s: %w", challengeConf.Name, err)
			}
		}
		if challengeData == nil {
			log.Error("Update returned nil challenge data for %s", challengeConf.Name)
			return fmt.Errorf("update challenge failed for %s", challengeConf.Name)
		}
		log.InfoH2("Successfully updated challenge %s", challengeConf.Name)

		log.InfoH3("Caching updated challenge data for %s", challengeConf.Name)
		if err := setCache(challengeData.Category+"/"+challengeConf.Name+"/challenge", challengeData); err != nil {
			log.Error("Failed to cache challenge data for %s: %v", challengeConf.Name, err)
			return fmt.Errorf("cache error for %s: %w", challengeConf.Name, err)
		}
		log.InfoH3("Successfully cached challenge data for %s", challengeConf.Name)
	} else {
		log.InfoH2("Challenge %s is unchanged, skipping update", challengeConf.Name)
	}

	log.InfoH2("Successfully completed sync for challenge: %s", challengeConf.Name)
	return nil
}

func handleChallengeAttachments(challengeConf ChallengeYaml, challengeData *gzapi.Challenge, api *gzapi.GZAPI) error {
	log.InfoH3("Processing attachments for challenge: %s", challengeConf.Name)

	if challengeConf.Provide != nil {
		log.InfoH3("Challenge %s has attachment: %s", challengeConf.Name, *challengeConf.Provide)

		if strings.HasPrefix(*challengeConf.Provide, "http") {
			log.InfoH3("Creating remote attachment for %s: %s", challengeConf.Name, *challengeConf.Provide)
			if err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
				AttachmentType: "Remote",
				RemoteUrl:      *challengeConf.Provide,
			}); err != nil {
				log.Error("Failed to create remote attachment for %s: %v", challengeConf.Name, err)
				return fmt.Errorf("remote attachment creation failed for %s: %w", challengeConf.Name, err)
			}
			log.InfoH3("Successfully created remote attachment for %s", challengeConf.Name)
		} else {
			log.InfoH3("Processing local attachment for %s: %s", challengeConf.Name, *challengeConf.Provide)
			return handleLocalAttachment(challengeConf, challengeData, api)
		}
	} else if challengeData.Attachment != nil {
		log.InfoH3("Removing existing attachment for %s", challengeConf.Name)
		if err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
			AttachmentType: "None",
		}); err != nil {
			log.Error("Failed to remove attachment for %s: %v", challengeConf.Name, err)
			return fmt.Errorf("attachment removal failed for %s: %w", challengeConf.Name, err)
		}
		log.InfoH3("Successfully removed attachment for %s", challengeConf.Name)
	} else {
		log.InfoH3("No attachment processing needed for %s", challengeConf.Name)
	}

	log.InfoH3("Attachment processing completed for %s", challengeConf.Name)
	return nil
}

func handleLocalAttachment(challengeConf ChallengeYaml, challengeData *gzapi.Challenge, api *gzapi.GZAPI) error {
	log.InfoH3("Creating local attachment for %s", challengeConf.Name)

	zipFilename := NormalizeFileName(*challengeConf.Provide) + ".zip"
	zipOutput := filepath.Join(challengeConf.Cwd, zipFilename)
	attachmentPath := filepath.Join(challengeConf.Cwd, *challengeConf.Provide)

	log.InfoH3("Checking attachment path: %s", attachmentPath)
	if info, err := os.Stat(attachmentPath); err != nil || info.IsDir() {
		log.InfoH3("Creating zip file for %s from: %s", challengeConf.Name, attachmentPath)
		if err := zipSource(attachmentPath, zipOutput); err != nil {
			log.Error("Failed to create zip for %s: %v", challengeConf.Name, err)
			return fmt.Errorf("zip creation failed for %s: %w", challengeConf.Name, err)
		}
		log.InfoH3("Successfully created zip file: %s", zipOutput)
		challengeConf.Provide = &zipFilename
	} else {
		log.InfoH3("Using existing file: %s", attachmentPath)
	}

	// Create a unique attachment file for this challenge to avoid hash conflicts
	originalFilePath := filepath.Join(challengeConf.Cwd, *challengeConf.Provide)
	uniqueFilename := fmt.Sprintf("%s_%s", challengeConf.Name, *challengeConf.Provide)
	uniqueFilePath := filepath.Join(challengeConf.Cwd, uniqueFilename)

	log.InfoH3("Creating unique attachment file: %s", uniqueFilePath)

	// Copy the original file and append challenge metadata to make it unique
	if err := createUniqueAttachmentFile(originalFilePath, uniqueFilePath, challengeConf.Name); err != nil {
		log.Error("Failed to create unique attachment file for %s: %v", challengeConf.Name, err)
		return fmt.Errorf("unique file creation failed for %s: %w", challengeConf.Name, err)
	}

	log.InfoH3("Creating/checking assets for %s", challengeConf.Name)
	fileinfo, err := createAssetsIfNotExistOrDifferent(uniqueFilePath, api)
	if err != nil {
		os.Remove(uniqueFilePath) // Clean up on error
		log.Error("Failed to create/check assets for %s: %v", challengeConf.Name, err)
		return fmt.Errorf("asset creation failed for %s: %w", challengeConf.Name, err)
	}
	log.InfoH3("Asset info for %s: Hash=%s, Name=%s", challengeConf.Name, fileinfo.Hash, fileinfo.Name)

	// Check if the challenge already has the same attachment hash
	if challengeData.Attachment != nil && strings.Contains(challengeData.Attachment.Url, fileinfo.Hash) {
		log.InfoH3("Attachment for %s is unchanged (hash: %s)", challengeConf.Name, fileinfo.Hash)
	} else {
		var attachmentUrl string
		if challengeData.Attachment != nil {
			attachmentUrl = challengeData.Attachment.Url
		}
		log.InfoH3("Updating attachment for %s (hash: %s, current: %s)", challengeConf.Name, fileinfo.Hash, attachmentUrl)

		// Try to create the attachment
		err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
			AttachmentType: "Local",
			FileHash:       fileinfo.Hash,
		})

		if err != nil {
			log.Error("Failed to create local attachment for %s: %v", challengeConf.Name, err)
			os.Remove(uniqueFilePath) // Clean up on error
			return fmt.Errorf("local attachment creation failed for %s: %w", challengeConf.Name, err)
		} else {
			log.InfoH3("Successfully created local attachment for %s", challengeConf.Name)
		}
	}

	// Clean up temporary files
	if strings.HasSuffix(zipOutput, ".zip") {
		log.InfoH3("Cleaning up temporary zip file: %s", zipOutput)
		os.Remove(zipOutput)
	}

	// Clean up the unique file after successful upload
	log.InfoH3("Cleaning up unique attachment file: %s", uniqueFilePath)
	os.Remove(uniqueFilePath)

	log.InfoH3("Local attachment processing completed for %s", challengeConf.Name)
	return nil
}

// createUniqueAttachmentFile creates a unique version of the attachment file by appending metadata
func createUniqueAttachmentFile(srcPath, dstPath, challengeName string) error {
	// Copy the original file
	if err := copyFile(srcPath, dstPath); err != nil {
		return err
	}

	// Append challenge-specific metadata to make the file unique
	file, err := os.OpenFile(dstPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Add a comment or metadata that makes this file unique for this challenge
	metadata := fmt.Sprintf("\n# Challenge: %s\n", challengeName)
	_, err = file.WriteString(metadata)
	return err
}

func updateChallengeFlags(config *Config, challengeConf ChallengeYaml, challengeData *gzapi.Challenge) error {
	for _, flag := range challengeData.Flags {
		if !isExistInArray(flag.Flag, challengeConf.Flags) {
			flag.GameId = config.Event.Id
			flag.ChallengeId = challengeData.Id
			flag.CS = config.Event.CS
			if err := flag.Delete(); err != nil {
				return err
			}
		}
	}

	isCreatingNewFlag := false

	for _, flag := range challengeConf.Flags {
		if !isFlagExist(flag, challengeData.Flags) {
			if err := challengeData.CreateFlag(gzapi.CreateFlagForm{
				Flag: flag,
			}); err != nil {
				return err
			}
			isCreatingNewFlag = true
		}
	}

	if isCreatingNewFlag {
		newChallData, err := challengeData.Refresh()
		if err != nil {
			return err
		}
		challengeData.Flags = newChallData.Flags
	}

	return nil
}

var shell = os.Getenv("SHELL")

func runScript(challengeConf ChallengeYaml, script string) error {
	if challengeConf.Scripts[script] == "" {
		return nil
	}
	log.InfoH2("Running:\n%s", challengeConf.Scripts[script])
	return runShell(challengeConf.Scripts[script], challengeConf.Cwd)
}

func runShell(script string, cwd string) error {
	cmd := exec.Command(shell, "-c", script)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
