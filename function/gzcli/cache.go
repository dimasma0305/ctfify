package gzcli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// cacheDir caches the working directory to avoid repeated lookups
var cacheDir = func() string {
	dir, _ := os.Getwd()
	return filepath.Join(dir, ".gzcli")
}()

// setCache atomically writes data to cache with proper directory creation
func setCache(key string, data any) error {
	cachePath := filepath.Join(cacheDir, key+".yaml")

	// Create cache directory with proper permissions
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Atomic write pattern using temp file
	tmpFile, err := os.CreateTemp(cacheDir, "tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Use buffered writer with pre-allocated buffer
	bw := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB buffer
	if err := yaml.NewEncoder(bw).Encode(data); err != nil {
		return fmt.Errorf("encoding failed: %w", err)
	}

	// Flush buffer before renaming
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("buffer flush failed: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("temp file close failed: %w", err)
	}

	// Atomic rename to final path
	if err := os.Rename(tmpFile.Name(), cachePath); err != nil {
		return fmt.Errorf("failed to finalize cache: %w", err)
	}

	return nil
}

// GetCache reads cached data using optimized file access
func GetCache(key string, data any) error {
	cachePath := filepath.Join(cacheDir, key+".yaml")

	file, err := os.Open(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache not found")
		}
		return fmt.Errorf("cache access error: %w", err)
	}
	defer file.Close()

	buffered := bufio.NewReader(file)
	if err := yaml.NewDecoder(buffered).Decode(data); err != nil {
		return fmt.Errorf("decoding error: %w", err)
	}

	return nil
}

// DeleteCache removes cache files with minimal syscalls
func DeleteCache(key string) error {
	cachePath := filepath.Join(cacheDir, key+".yaml")

	if err := os.Remove(cachePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache not found: %s", key)
		}
		return fmt.Errorf("deletion error: %w", err)
	}

	return nil
}
