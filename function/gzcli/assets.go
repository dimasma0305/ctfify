package gzcli

import (
	"fmt"
	"strings"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
)

func createAssetsIfNotExistOrDifferent(file string, client *gzapi.API) (*gzapi.FileInfo, error) {
	assets, err := client.GetAssets()
	if err != nil {
		return nil, err
	}

	hash, err := GetFileHash(file)
	if err != nil {
		return nil, err
	}

	for _, asset := range assets {
		if asset.Hash == hash {
			return &asset, nil
		}
	}

	asset, err := client.CreateAssets(file)
	if err != nil {
		return nil, err
	}

	if len(asset) == 0 {
		return nil, fmt.Errorf("error creating asset")
	}

	return &asset[0], nil
}

func createPosterIfNotExistOrDifferent(file string, game *gzapi.Game, client *gzapi.API) (string, error) {
	assets, err := client.GetAssets()
	if err != nil {
		return "", err
	}

	hash, err := GetFileHash(file)
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

func GetClient() (*gzapi.API, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}

	client, err := gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		return nil, err
	}

	return client, nil
}
