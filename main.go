package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config structure
type FolderConfig struct {
	Source       string   `toml:"source"`
	Destinations []string `toml:"destinations"`
}

type Config struct {
	Folders map[string]FolderConfig `toml:"folders"`
}

func main() {

	// Load TOML configuration
	var config Config
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		fmt.Println("Error reading config.toml:", err)
		return
	}

	// Sync logic
	for folderName, folder := range config.Folders {
		// TODO: add func to handle relative paths as well and delete this expandenv
		expandedSource := os.ExpandEnv(folder.Source)
		// TEST: test if expandedSource is valid
		fmt.Printf("Syncing %s to %s\n", expandedSource, folderName)

		for _, dest := range folder.Destinations {
			destParts := strings.Split(dest, ":")
			if len(destParts) != 2 {
				fmt.Printf("Invalid destination format in config for '%s': %s\n", folderName, dest)
				continue
			}

			remoteType := destParts[0]
			remotePath := destParts[1]
			syncToRemote(expandedSource, remoteType, remotePath)
		}
	}

}

func syncToRemote(folder, remoteType, remotePath string) {
}
