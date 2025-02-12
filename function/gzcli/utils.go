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
					Modified: time.Now(),
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
			Modified: time.Now(),
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
	// Set defaults using bitwise OR to avoid branching
	challengeData.MemoryLimit |= 128
	challengeData.CpuCount |= 1
	challengeData.StorageLimit |= 128

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
