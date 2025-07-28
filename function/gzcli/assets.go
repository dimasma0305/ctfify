package gzcli

import (
	"fmt"
	"strings"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
)

func createAssetsIfNotExistOrDifferent(file string, client *gzapi.GZAPI) (*gzapi.FileInfo, error) {
	log.DebugH3("Checking assets for file: %s", file)

	assets, err := client.GetAssets()
	if err != nil {
		log.Error("Failed to get assets: %v", err)
		return nil, err
	}
	log.DebugH3("Found %d existing assets", len(assets))

	hash, err := GetFileHashHex(file)
	if err != nil {
		log.Error("Failed to get file hash for %s: %v", file, err)
		return nil, err
	}
	log.DebugH3("File hash for %s: %s", file, hash)

	for _, asset := range assets {
		if asset.Hash == hash {
			log.DebugH3("Found existing asset with matching hash: %s", asset.Name)
			return &asset, nil
		}
	}

	log.DebugH3("No existing asset found, creating new asset for file: %s", file)
	asset, err := client.CreateAssets(file)
	if err != nil {
		log.Error("Failed to create asset for %s: %v", file, err)
		return nil, err
	}

	if len(asset) == 0 {
		log.Error("Asset creation returned empty result for %s", file)
		return nil, fmt.Errorf("error creating asset")
	}

	log.DebugH3("Successfully created asset: %s (hash: %s)", asset[0].Name, asset[0].Hash)
	return &asset[0], nil
}

func createPosterIfNotExistOrDifferent(file string, game *gzapi.Game, client *gzapi.GZAPI) (string, error) {
	assets, err := client.GetAssets()
	if err != nil {
		return "", err
	}

	hash, err := GetFileHashHex(file)
	if err != nil {
		return "", err
	}

	for _, asset := range assets {
		if asset.Name == "poster.webp" && asset.Hash == hash {
			return "/assets/" + asset.Hash + "/poster", nil
		}
	}

	asset, err := game.UploadPoster(file)
	if err != nil {
		return "", err
	}

	if len(asset) == 0 {
		return "", fmt.Errorf("error creating poster")
	}
	asset = strings.Replace(asset, ".webp", "", 1)
	return asset, nil
}

func GetClient(api *gzapi.GZAPI) (*gzapi.GZAPI, error) {
	config, err := GetConfig(api)
	if err != nil {
		return nil, err
	}

	client, err := gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		return nil, err
	}

	return client, nil
}
