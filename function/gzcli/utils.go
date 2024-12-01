package gzcli

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

func NormalizeFileName(name string) string {
	re := regexp.MustCompile("[^a-zA-Z0-9\\-_ ]+")
	return strings.ToLower(re.ReplaceAllString(name, ""))
}

func ParseYamlFromFile(confPath string, data any) error {
	confFile, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("error read conf.yaml: %w", err)
	}

	if err := yaml.Unmarshal(confFile, data); err != nil {
		return fmt.Errorf("error unmarshal yaml: %w", err)
	}

	return nil
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
	badChallenge := false
	validTypes := map[string]bool{
		"StaticAttachment":  true,
		"StaticContainer":   true,
		"DynamicAttachment": true,
		"DynamicContainer":  true,
	}

	if challenge.Name == "" {
		badChallenge = true
		log.Error("challenge must have a name")
	}

	if challenge.Author == "" {
		badChallenge = true
		log.Error("challenge must have an author")
	}

	if !validTypes[challenge.Type] {
		badChallenge = true
		log.Error("bad challenge type, use one of: %v", validTypes)
	}

	if challenge.Value < 0 {
		badChallenge = true
		log.Error("bad challenge value, must be positive")
	}

	if len(challenge.Flags) == 0 {
		if challenge.Type == "StaticAttachment" || challenge.Type == "StaticContainer" {
			badChallenge = true
			log.Error("challenge must have at least one flag")
		}
		if challenge.Type == "DynamicContainer" {
			if challenge.Container.FlagTemplate == "" {
				badChallenge = true
				log.Error("challenge must have a flag template")
			}
		}
	}

	if badChallenge {
		return fmt.Errorf("bad challenge")
	}

	return nil
}

func isChallengeExist(challengeName string, challenges []gzapi.Challenge) bool {
	for _, challenge := range challenges {
		if challenge.Title == challengeName {
			return true
		}
	}
	return false
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
	for _, f := range flags {
		if f.Flag == flag {
			return true
		}
	}
	return false
}

func zipSource(source, target string) error {
	// 1. Create a ZIP file and zip.Writer
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := zip.NewWriter(f)
	defer writer.Close()

	// 2. Go through all the files of the source
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 3. Create a local file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// set compression
		header.Method = zip.Deflate

		// 4. Set relative path of a file as the header name
		header.Name, err = filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			header.Name += "/"
		}

		// 5. Create writer for the file header and save content of the file
		headerWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(headerWriter, f)
		return err
	})
}

func isConfigEdited(challengeConf *ChallengeYaml, challengeData *gzapi.Challenge) bool {
	var cacheChallenge gzapi.Challenge
	if err := GetCache(challengeConf.Category+"/"+challengeConf.Name+"/challenge", &cacheChallenge); err == nil {
		if challengeData.Hints == nil {
			challengeData.Hints = []string{}
		}
		if cmp.Equal(*challengeData, cacheChallenge) {
			return false
		}
	}
	return true
}

func mergeChallengeData(challengeConf *ChallengeYaml, challengeData *gzapi.Challenge) *gzapi.Challenge {
	challengeData.Title = challengeConf.Name
	challengeData.Category = challengeConf.Category
	challengeData.Content = "Author: **" + challengeConf.Author + "**\n\n" + challengeConf.Description
	challengeData.Type = challengeConf.Type
	challengeData.Hints = challengeConf.Hints
	challengeData.AcceptedCount = 0
	challengeData.FileName = "attachment"
	challengeData.FlagTemplate = challengeConf.Container.FlagTemplate
	challengeData.ContainerImage = challengeConf.Container.ContainerImage

	challengeData.MemoryLimit = challengeConf.Container.MemoryLimit
	challengeData.CpuCount = challengeConf.Container.CpuCount
	challengeData.StorageLimit = challengeConf.Container.StorageLimit

	if challengeData.MemoryLimit == 0 {
		challengeData.MemoryLimit = 128
	}
	if challengeData.CpuCount == 0 {
		challengeData.CpuCount = 1
	}
	if challengeData.StorageLimit == 0 {
		challengeData.StorageLimit = 128
	}
	challengeData.ContainerExposePort = challengeConf.Container.ContainerExposePort
	challengeData.EnableTrafficCapture = challengeConf.Container.EnableTrafficCapture

	challengeData.OriginalScore = challengeConf.Value
	if challengeData.OriginalScore < 100 {
		challengeData.MinScoreRate = 1
	} else {
		challengeData.MinScoreRate = 0.10
	}
	challengeData.Difficulty = 5
	challengeData.IsEnabled = nil
	return challengeData
}
