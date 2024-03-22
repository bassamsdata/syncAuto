package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
)

// config file should look like this

// [folders]
//     [folders.folder1]
//     source = "~/Sync/test"
//     destinations = ["googledrive:test"]
//
//     [folders.folder2]
//     source = "/home/docs/sync"
//     destinations = ["onedrive:test"]

// Config structure
type FolderConfig struct {
	Source       string   `toml:"source"`
	Destinations []string `toml:"destinations"`
}

type Config struct {
	Folders map[string]FolderConfig `toml:"folders"`
}

func main() {

	// TODO: Create file if it doesn't exist
	var config Config
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		fmt.Println("Error reading config.toml:", err)
		return
	}

	// Sync logic
	for folderName, folder := range config.Folders {
		expandedSource, err := expandTilde(folder.Source)
		if err != nil {
			fmt.Printf("Error expanding tilde in '%s': %s\n", folderName, err)
			continue
		}
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

func syncToRemote(folder, sourceRemote, destRemote string) {
	_, err := exec.LookPath("rclone")
	if err != nil {
		fmt.Println("Error: rclone not found in your PATH.")
		return
	}

	cmd := exec.Command("rclone", "sync", folder, fmt.Sprintf("%s:%s", sourceRemote, destRemote))
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error syncing folder '%s' to '%s': %v\nOutput: %s\n", folder, destRemote, err, out)
	} else {
		// TEST: just for testing
		fmt.Printf("Folder '%s' synced successfully to '%s'\n", folder, destRemote)
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
