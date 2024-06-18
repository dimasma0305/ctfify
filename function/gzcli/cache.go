package gzcli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

func setCache(key string, data any) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	cachePath := filepath.Join(dir, ".gzcli", key+".yaml")
	if err := os.MkdirAll(filepath.Dir(cachePath), os.ModePerm); err != nil {
		return fmt.Errorf("error create cache directory: %w", err)
	}

	cacheFile, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("error create cache file: %w", err)
	}
	defer cacheFile.Close()

	if err := yaml.NewEncoder(cacheFile).Encode(data); err != nil {
		return fmt.Errorf("error encode data to cache file: %w", err)
	}

	return nil
}

func GetCache(key string, data any) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	cachePath := filepath.Join(dir, ".gzcli", key+".yaml")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return fmt.Errorf("cache not found")
	}

	cacheFile, err := os.Open(cachePath)
	if err != nil {
		return fmt.Errorf("error open cache file: %w", err)
	}
	defer cacheFile.Close()

	if err := yaml.NewDecoder(cacheFile).Decode(data); err != nil {
		return fmt.Errorf("error decode cache file: %w", err)
	}

	return nil
}
