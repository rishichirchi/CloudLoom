package github

import (
	"fmt"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/joho/godotenv"
)

func GetGHClient(installationId int64, appID int64) (*github.Client, error) {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("No .env file found or failed to load")
	}
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	transport, err := ghinstallation.New(http.DefaultTransport, appID, installationId, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)

	}
	client := github.NewClient(&http.Client{
		Transport: transport,
	})
	fmt.Println("Client:", client)
	return client, nil
}
