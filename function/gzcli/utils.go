package gzcli

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/template"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

var (
	fileNameNormalizer = regexp.MustCompile(`[^a-zA-Z0-9\-_ ]+`)
	validTypes         = map[string]struct{}{
		"StaticAttachment":  {},
		"StaticContainer":   {},
		"DynamicAttachment": {},
		"DynamicContainer":  {},
	}
	bufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}
)

func NormalizeFileName(name string) string {
	buf := bufferPool.Get().(*bytes.Buffer)
	defer bufferPool.Put(buf)
	defer buf.Reset()

	buf.WriteString(name)
	result := fileNameNormalizer.ReplaceAllString(buf.String(), "")
	return strings.ToLower(result)
}

func ParseYamlFromBytes(b []byte, data any) error {
	if err := yaml.Unmarshal(b, data); err != nil {
		return fmt.Errorf("error unmarshal yaml: %w", err)
	}
	return nil
}

func ParseYamlFromFile(confPath string, data any) error {
	buf := bufferPool.Get().(*bytes.Buffer)
	defer bufferPool.Put(buf)
	defer buf.Reset()

	f, err := os.Open(confPath)
	if err != nil {
		return fmt.Errorf("file open error: %w", err)
	}
	defer f.Close()

	if _, err := buf.ReadFrom(f); err != nil {
		return fmt.Errorf("file read error: %w", err)
	}

	return ParseYamlFromBytes(buf.Bytes(), data)
}

func GetFileHashHex(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := f.WriteTo(h); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func isGoodChallenge(challenge ChallengeYaml) error {
	var errors []string

	if challenge.Name == "" {
		errors = append(errors, "missing name")
	}
	if challenge.Author == "" {
		errors = append(errors, "missing author")
	}
	if _, valid := validTypes[challenge.Type]; !valid {
		errors = append(errors, fmt.Sprintf("invalid type: %s", challenge.Type))
	}
	if challenge.Value < 0 {
		errors = append(errors, "negative value")
	}

	switch {
	case len(challenge.Flags) == 0 && (challenge.Type == "StaticAttachment" || challenge.Type == "StaticContainer"):
		errors = append(errors, "missing flags for static challenge")
	case challenge.Type == "DynamicContainer" && challenge.Container.FlagTemplate == "":
		errors = append(errors, "missing flag template for dynamic container")
	}

	if len(errors) > 0 {
		log.Error("Validation errors for %s:", challenge.Name)
		for _, e := range errors {
			log.Error("  - %s", e)
		}
		return fmt.Errorf("invalid challenge: %s", challenge.Name)
	}

	return nil
}

func isChallengeExist(challengeName string, challenges []gzapi.Challenge) bool {
	challengeMap := make(map[string]struct{}, len(challenges))
	for _, c := range challenges {
		challengeMap[c.Title] = struct{}{}
	}
	_, exists := challengeMap[challengeName]
	return exists
}

func isExistInArray(value string, array []string) bool {
	for _, v := range array {
		if v == value {
			return true
		}
	}
	return false
}

func isFlagExist(flag string, flags []gzapi.Flag) bool {
	flagMap := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		flagMap[f.Flag] = struct{}{}
	}
	_, exists := flagMap[flag]
	return exists
}

func zipSource(source, target string) error {
	// Create output file with buffered writer
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer f.Close()

	buffered := bufio.NewWriterSize(f, 1<<20) // 1MB buffer
	defer buffered.Flush()

	// Create zip writer with optimized compression
	writer := zip.NewWriter(buffered)
	defer writer.Close()

	// Set faster compression level
	writer.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})

	// Pre-allocate buffer pool
	bufPool := sync.Pool{
		New: func() interface{} { return make([]byte, 32<<10) }, // 32KB buffers
	}

	// Collect files first to enable parallel processing
	var filePaths []string
	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		filePaths = append(filePaths, path)
		return nil
	})

	// Process files in parallel but write sequentially
	type result struct {
		path string
		data []byte
		err  error
	}
	resultChan := make(chan result, len(filePaths))

	// Worker pool for parallel reading
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup

	// Use a fixed timestamp for reproducible builds
	fixedTime := time.Date(2025, 3, 18, 0, 0, 0, 0, time.UTC)

	for _, path := range filePaths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Read file content
			data, err := os.ReadFile(p)
			resultChan <- result{p, data, err}
		}(path)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Write results in original order while maintaining directory structure
	writtenFiles := make(map[string]struct{})
	for res := range resultChan {
		if res.err != nil {
			return res.err
		}

		relPath, err := filepath.Rel(source, res.path)
		if err != nil {
			return err
		}

		// Ensure directory entries exist
		dirPath := filepath.Dir(relPath)
		if dirPath != "." {
			if _, exists := writtenFiles[dirPath]; !exists {
				header := &zip.FileHeader{
					Name:     dirPath + "/",
					Method:   zip.Deflate,
					Modified: fixedTime,
				}
				if _, err := writer.CreateHeader(header); err != nil {
					return err
				}
				writtenFiles[dirPath] = struct{}{}
			}
		}

		// Create file header
		header := &zip.FileHeader{
			Name:     relPath,
			Method:   zip.Deflate,
			Modified: fixedTime,
		}
		header.SetMode(0644)

		// Use buffer from pool
		buf := bufPool.Get().([]byte)
		defer bufPool.Put(&buf)

		// Write to zip
		w, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		if _, err := io.CopyBuffer(w, bytes.NewReader(res.data), buf); err != nil {
			return err
		}
	}

	return nil
}

func isConfigEdited(challengeConf *ChallengeYaml, challengeData *gzapi.Challenge) bool {
	var cacheChallenge gzapi.Challenge
	if err := GetCache(challengeConf.Category+"/"+challengeConf.Name+"/challenge", &cacheChallenge); err != nil {
		return true
	}

	if challengeData.Hints == nil {
		challengeData.Hints = []string{}
	}
	return !cmp.Equal(*challengeData, cacheChallenge)
}

func mergeChallengeData(challengeConf *ChallengeYaml, challengeData *gzapi.Challenge) *gzapi.Challenge {
	// Set resource limits from container configuration, with defaults if not specified
	if challengeConf.Container.MemoryLimit > 0 {
		challengeData.MemoryLimit = challengeConf.Container.MemoryLimit
	} else {
		challengeData.MemoryLimit = 128 // Default fallback
	}

	if challengeConf.Container.CpuCount > 0 {
		challengeData.CpuCount = challengeConf.Container.CpuCount
	} else {
		challengeData.CpuCount = 1 // Default fallback
	}

	if challengeConf.Container.StorageLimit > 0 {
		challengeData.StorageLimit = challengeConf.Container.StorageLimit
	} else {
		challengeData.StorageLimit = 128 // Default fallback
	}

	challengeData.Title = challengeConf.Name
	challengeData.Category = challengeConf.Category
	challengeData.Content = fmt.Sprintf("Author: **%s**\n\n%s", challengeConf.Author, challengeConf.Description)
	challengeData.Type = challengeConf.Type
	challengeData.Hints = challengeConf.Hints
	challengeData.FlagTemplate = challengeConf.Container.FlagTemplate
	challengeData.ContainerImage = challengeConf.Container.ContainerImage
	challengeData.ContainerExposePort = challengeConf.Container.ContainerExposePort
	challengeData.EnableTrafficCapture = challengeConf.Container.EnableTrafficCapture
	challengeData.OriginalScore = challengeConf.Value

	if challengeData.OriginalScore >= 100 {
		challengeData.MinScoreRate = 0.10
	} else {
		challengeData.MinScoreRate = 1
	}

	return challengeData
}

func genStructure(challenges []ChallengeYaml) error {
	// Read the .structure file
	_, err := os.ReadDir(".structure")
	if err != nil {
		return fmt.Errorf(".structure dir doesn't exist: %w", err)
	}

	// Iterate over each challenge in the challenges slice
	for _, challenge := range challenges {
		// Construct the challenge path using the challenge data
		if err := template.TemplateToDestination(".structure", challenge, challenge.Cwd); err != nil {
			log.Error("Failed to copy .structure to %s: %v", challenge.Cwd, err)
			continue
		}
		log.Info("Successfully copied .structure to %s", challenge.Cwd)
	}

	return nil
}

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

	// Check existence using the original challenges list first to avoid unnecessary API calls
	if !isChallengeExist(challengeConf.Name, challenges) {
		// Double-check with fresh challenges list to prevent race conditions
		// This check happens inside the mutex-protected section in the calling function
		log.InfoH3("Challenge %s not found in initial list, fetching fresh challenges list", challengeConf.Name)
		freshChallenges, err := config.Event.GetChallenges()
		if err != nil {
			log.Error("Failed to get fresh challenges list for %s: %v", challengeConf.Name, err)
			// Fallback to original challenges list if fresh fetch fails
			freshChallenges = challenges
		} else {
			log.InfoH3("Fetched fresh challenges list for %s (%d challenges)", challengeConf.Name, len(freshChallenges))
		}

		// Final check to prevent duplicates
		if !isChallengeExist(challengeConf.Name, freshChallenges) {
			log.InfoH2("Creating new challenge: %s", challengeConf.Name)
			challengeData, err = config.Event.CreateChallenge(gzapi.CreateChallengeForm{
				Title:    challengeConf.Name,
				Category: challengeConf.Category,
				Tag:      challengeConf.Category,
				Type:     challengeConf.Type,
			})
			if err != nil {
				// Check if this is a duplicate creation error (common with race conditions)
				if strings.Contains(strings.ToLower(err.Error()), "already exists") ||
					strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
					strings.Contains(strings.ToLower(err.Error()), "conflict") {
					log.InfoH2("Challenge %s already exists (created by another process), fetching existing challenge", challengeConf.Name)
					challengeData, err = config.Event.GetChallenge(challengeConf.Name)
					if err != nil {
						log.Error("Failed to get existing challenge %s after creation conflict: %v", challengeConf.Name, err)
						return fmt.Errorf("get existing challenge %s: %w", challengeConf.Name, err)
					}
					challengeData.CS = api
					log.InfoH3("Successfully fetched existing challenge %s after creation conflict", challengeConf.Name)
				} else {
					log.Error("Failed to create challenge %s: %v", challengeConf.Name, err)
					return fmt.Errorf("create challenge %s: %w", challengeConf.Name, err)
				}
			} else {
				challengeData.CS = api
				log.InfoH2("Successfully created challenge: %s (ID: %d)", challengeConf.Name, challengeData.Id)
			}
		} else {
			log.InfoH2("Challenge %s was created by another process, fetching existing challenge", challengeConf.Name)
			// Challenge was created by another goroutine, fetch it
			challengeData, err = config.Event.GetChallenge(challengeConf.Name)
			if err != nil {
				log.Error("Failed to get newly created challenge %s: %v", challengeConf.Name, err)
				return fmt.Errorf("get challenge %s: %w", challengeConf.Name, err)
			}
			log.InfoH3("Successfully fetched existing challenge %s", challengeConf.Name)
		}

		// Ensure the API client is properly set for newly created/fetched challenges
		challengeData.CS = api
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

	zipFilename := "dist.zip"
	// Write zip to temp dir to avoid triggering watcher events inside challenge dir
	zipOutput := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s", NormalizeFileName(challengeConf.Name), zipFilename))
	attachmentPath := filepath.Join(challengeConf.Cwd, *challengeConf.Provide)

	// Artifact path that will be used for upload/uniqueness processing
	var artifactPath string
	var artifactBase string

	log.InfoH3("Checking attachment path: %s", attachmentPath)
	if info, err := os.Stat(attachmentPath); err != nil || info.IsDir() {
		log.InfoH3("Creating zip file for %s from: %s", challengeConf.Name, attachmentPath)
		if err := zipSource(attachmentPath, zipOutput); err != nil {
			log.Error("Failed to create zip for %s: %v", challengeConf.Name, err)
			return fmt.Errorf("zip creation failed for %s: %w", challengeConf.Name, err)
		}
		log.InfoH3("Successfully created zip file: %s", zipOutput)
		// Use the temp zip directly as the artifact, do not write into challenge directory
		artifactPath = zipOutput
		artifactBase = filepath.Base(zipOutput)
	} else {
		log.InfoH3("Using existing file: %s", attachmentPath)
		artifactPath = attachmentPath
		artifactBase = filepath.Base(attachmentPath)
	}

	// Create a unique attachment file for this challenge to avoid hash conflicts
	uniqueFilename := fmt.Sprintf("%s_%s", challengeConf.Name, artifactBase)
	uniqueFilePath := filepath.Join(os.TempDir(), NormalizeFileName(uniqueFilename))

	log.InfoH3("Creating unique attachment file: %s", uniqueFilePath)

	// Copy the artifact and append challenge metadata to make it unique
	if err := createUniqueAttachmentFile(artifactPath, uniqueFilePath, challengeConf.Name); err != nil {
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
	if shell == "" {
		shell = "/bin/sh"
	}
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
