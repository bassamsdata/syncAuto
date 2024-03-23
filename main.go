package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// TODO: add this to the readme,
// config file should look like this

// TODO: add comments to the code

// [folders]
//     [folders.folder1]
//     source = "~/Sync/test"
//     destinations = ["googledrive:test"] # this is must be an array, it can have more than one destination
//
//     [folders.folder2]
//     source = "/home/docs/sync"
//     destinations = ["onedrive:test"]

// Config structure
type FolderConfig struct {
	Source      string   `toml:"source"`
	Destination []string `toml:"destination"`
}

type Config struct {
	Folders map[string]FolderConfig `toml:"folders"`
}

func main() {

	// Create a log file
	logFile, err := os.OpenFile("syncAuto.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("Error creating log file:", err)
		return
	}
	defer logFile.Close()

	// Create a loggerger
	logger := log.New(logFile, "syncAuto: ", log.LstdFlags|log.Lshortfile)

	if err := createConfigFile(logger); err != nil {
		logger.Println("Error creating config file:", err)
	}

	var config Config
	// TODO: change it once creating the init file
	configFilePath := filepath.Join(os.Getenv("HOME"), ".config", "syncAuto", "config.toml")

	if _, err := toml.DecodeFile(configFilePath, &config); err != nil {
		logger.Println("Error reading config.toml:", err)
		return
	}

	// Sync loggeric
	for folderName, folder := range config.Folders {
		expandedSource, err := expandTilde(folder.Source)
		if err != nil {
			logger.Printf("Error expanding tilde in '%s': %s\n", folderName, err)
			continue
		}

		for _, dest := range folder.Destination {
			destParts := strings.Split(dest, ":")
			if len(destParts) != 2 {
				logger.Printf("Invalid destination format in config for '%s': %s\n", folderName, dest)
				continue
			}

			remoteType := destParts[0]
			remotePath := destParts[1]
			syncToRemote(expandedSource, remoteType, remotePath, logger)
		}
	}

}

func syncToRemote(folder, sourceRemote, destRemote string, logger *log.Logger) {
	_, err := exec.LookPath("rclone")
	if err != nil {
		logger.Println("Error: rclone not found in your PATH.")
		return
	}

	cmd := exec.Command("rclone", "sync", folder, fmt.Sprintf("%s:%s", sourceRemote, destRemote))
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Printf("Note syncing folder '%s' to '%s': %v\nOutput: %s\n", folder, destRemote, err, out)
	} else {
		logger.Printf("Folder '%s' synced successfully to '%s'\n", folder, destRemote)
	}
}

func expandTilde(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return strings.Replace(path, "~", home, 1), nil
	}
	return path, nil
}

func createConfigFile(logger *log.Logger) error {
	// TODO: Move it to init() function
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Printf("error getting home directory: %v", err)
		return err
	}

	configDir := filepath.Join(homeDir, ".config", "syncAuto")
	configFile := filepath.Join(configDir, "config.toml")

	// Check if the config file already exists
	if _, err := os.Stat(configFile); err == nil {
		logger.Println("Note Config file already exists at:", configFile)
		return nil
	} else if !os.IsNotExist(err) {
		logger.Printf("error checking for config file: %v", err)
		return err
	}

	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		logger.Printf("error creating config directory: %v", err)
		return err
	}

	// Create an initial default config
	defaultConfig := Config{
		Folders: map[string]FolderConfig{},
	}

	f, err := os.Create(configFile)
	if err != nil {
		logger.Printf("error creating config file: %v", err)
		return err
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(defaultConfig); err != nil {
		logger.Printf("error writing default config to file: %v", err)
		return err
	}

	logger.Println("Config file created at:", configFile)
	return nil
}
