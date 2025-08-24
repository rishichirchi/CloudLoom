package steampipe

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
)

func ConfigureSteampipe(profileName, roleARN, externalID, sourceProfile string) error {
	if err := addAWSProfile(profileName, roleARN, externalID, sourceProfile); err != nil {
		return fmt.Errorf("failed to add AWS profile: %v", err)
	}

	if err := addSteampipeConnection(profileName, profileName); err != nil {
		return fmt.Errorf("failed to add Steampipe connection: %v", err)
	}

	if err := restartSteampipeService(); err != nil {
		return fmt.Errorf("failed to restart Steampipe service: %v", err)
	}

	return nil
}

func addAWSProfile(profileName string, roleARN string, externalID string, sourceProfile string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .aws directory: %v", err)
	}

	awsConfigPath := filepath.Join(awsDir, "config")

	cfg, err := ini.Load(awsConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = ini.Empty()
		} else {
			return fmt.Errorf("failed to load AWS config file: %v", err)
		}
	}

	sectionName := "profile " + profileName

	// Remove existing section if it exists
	cfg.DeleteSection(sectionName)

	section, err := cfg.NewSection(sectionName)
	if err != nil {
		return fmt.Errorf("failed to create new section: %v", err)
	}

	section.Key("role_arn").SetValue(roleARN)
	section.Key("external_id").SetValue(externalID)
	section.Key("source_profile").SetValue(sourceProfile)
	section.Key("region").SetValue("ap-south-1")

	return cfg.SaveTo(awsConfigPath)
}

func addSteampipeConnection(connectionName, profileName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	steampipeConfigDir := filepath.Join(home, ".steampipe", "config")
	if err := os.MkdirAll(steampipeConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create Steampipe config directory: %v", err)
	}

	steampipeConfigPath := filepath.Join(steampipeConfigDir, "aws.spc")

	// Check if connection already exists
	if connectionExists(steampipeConfigPath, connectionName) {
		log.Printf("Connection '%s' already exists, skipping...", connectionName)
		return nil
	}

	hclBlock := fmt.Sprintf("\n# Connection for %s\nconnection \"%s\" {\n  plugin  = \"aws\"\n  profile = \"%s\"\n  regions = [\"*\"]\n}\n",
		profileName, connectionName, profileName)

	f, err := os.OpenFile(steampipeConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open Steampipe config file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(hclBlock); err != nil {
		return fmt.Errorf("failed to write to Steampipe config file: %v", err)
	}

	return nil
}

func connectionExists(configPath, connectionName string) bool {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	searchString := fmt.Sprintf("connection \"%s\"", connectionName)
	return strings.Contains(string(content), searchString)
}

func restartSteampipeService() error {
	// First, stop the service if running
	stopCmd := exec.Command("steampipe", "service", "stop")
	stopCmd.Run() // Ignore errors as service might not be running

	// Start the service
	cmd := exec.Command("steampipe", "service", "start")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("steampipe start failed: %s\n%w", string(output), err)
	}
	log.Println("Steampipe service started:", string(output))
	return nil
}
