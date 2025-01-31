package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

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
	OriginalSource string   `toml:"originalSource"`
	Source         string   `toml:"source"`
	Destination    []string `toml:"destination"`
}

type Config struct {
	Folders map[string]FolderConfig `toml:"folders"`
}

type logEntry struct {
	Timestamp  string
	FolderName string
	Operation  string
	Message    string
}

func (l logEntry) String() string {
	return fmt.Sprintf("[%s] [%s] [%s] %s", l.Timestamp, l.FolderName, l.Operation, l.Message)
}

func logMessage(logger *log.Logger, entry logEntry) {
	entry.Timestamp = time.Now().Format(time.RFC3339)
	logger.Println(entry)
}

func main() {
	// Create a log file
	logpath := filepath.Join(os.Getenv("HOME"), "repos", "syncAuto", "syncAuto.log")
	logFile, err := os.OpenFile(logpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
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
	configFilePath := filepath.Join(os.Getenv("HOME"), ".config", "syncAuto", "config.toml")

	if _, err := toml.DecodeFile(configFilePath, &config); err != nil {
		logger.Println("Error reading config.toml:", err)
		return
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit to 5 concurrent operations

	for folderName, folder := range config.Folders {
		wg.Add(1)
		go func(name string, cfg FolderConfig) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			if err := processFolderAsync(name, cfg, logger); err != nil {
				logMessage(logger, logEntry{
					FolderName: name,
					Operation:  "ERROR",
					Message:    fmt.Sprintf("Error processing folder: %v", err),
				})
			}
		}(folderName, folder)
	}
	wg.Wait()
}

func processFolderAsync(folderName string, folder FolderConfig, logger *log.Logger) error {
	expandedSource, err := expandTilde(folder.Source)
	if err != nil {
		return fmt.Errorf("error expanding tilde in '%s': %v", folderName, err)
	}

	copyFrom, _ := expandTilde(folder.OriginalSource)
	if err := copyIsDir(copyFrom, expandedSource, logger, folderName); err != nil {
		logMessage(logger, logEntry{
			FolderName: folderName,
			Operation:  "COPY_WARNING",
			Message:    fmt.Sprintf("Warning copying item: '%s', maybe the original source path is empty: '%s'", err, copyFrom),
		})
	}

	var syncWg sync.WaitGroup
	for _, dest := range folder.Destination {
		destParts := strings.Split(dest, ":")
		if len(destParts) != 2 {
			logMessage(logger, logEntry{
				FolderName: folderName,
				Operation:  "CONFIG_ERROR",
				Message:    fmt.Sprintf("Invalid destination format in config: %s", dest),
			})
			continue
		}

		remoteType := destParts[0]
		remotePath := destParts[1]

		syncWg.Add(1)
		go func(src, rType, rPath string) {
			defer syncWg.Done()
			syncToRemote(src, rType, rPath, logger, folderName)
		}(expandedSource, remoteType, remotePath)
	}

	syncWg.Wait()
	return nil
}

// copyIsDir determines if the source is a file or directory and calls the appropriate function
func copyIsDir(src, dst string, logger *log.Logger, folderName string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDirectory(src, dst, logger, folderName)
	}
	dst = filepath.Join(dst, filepath.Base(src)) // Add the file name to the destination path
	return copyFile(src, dst, logger, folderName)
}

// copyDirectory copies a directory from src to dst
func copyDirectory(src, dst string, logger *log.Logger, folderName string) error {
	// Create the destination directory if it doesn't exist
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	// List all files and directories within src
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntry := filepath.Join(src, entry.Name())
		dstEntry := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectories
			if err := copyDirectory(srcEntry, dstEntry, logger, folderName); err != nil {
				return err
			}
			// TODO: remove this
		} else {
			// Copy files
			if err := copyFile(srcEntry, dstEntry, logger, folderName); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(srcPath, dstPath string, logger *log.Logger, folderName string) error {
	// 1. Open the source file for reading
	srcFile, err := os.Open(srcPath)
	if err != nil {
		logger.Println("Error opening source file: ", err)
		return fmt.Errorf("error opening source file: %s", err)
	}
	defer srcFile.Close()

	// 2. Create destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		logger.Println("Error creating destination file: ", err)
		return fmt.Errorf("error creating destination file: %s", err)
	}
	defer dstFile.Close()

	// 3. Copy the contents
	if _, err = io.Copy(dstFile, srcFile); err != nil {
		logger.Println("Error copying file contents: ", err)
		return fmt.Errorf("error copying file contents: %s", err)
	}

	// 4. Ensure contents are written to disk
	err = dstFile.Sync()
	if err != nil {
		logger.Println("Error syncing destination file: ", err)
		return fmt.Errorf("error syncing destination file: %s", err)
	}

	logMessage(logger, logEntry{
		FolderName: folderName,
		Operation:  "COPY",
		Message:    fmt.Sprintf("File '%s' copied successfully to '%s'", filepath.Base(srcPath), dstPath),
	})
	return nil
}

func syncToRemote(folder, sourceRemote, destRemote string, logger *log.Logger, folderName string) {
	_, err := exec.LookPath("/usr/local/bin/rclone")
	if err != nil {
		logMessage(logger, logEntry{
			FolderName: folderName,
			Operation:  "SYNC_ERROR",
			Message:    "Error: rclone not found in your PATH.",
		})
		return
	}

	cmd := exec.Command("/usr/local/bin/rclone", "sync", folder, fmt.Sprintf("%s:%s", sourceRemote, destRemote))
	out, err := cmd.CombinedOutput()
	if err != nil {
		logMessage(logger, logEntry{
			FolderName: folderName,
			Operation:  "SYNC_ERROR",
			Message:    fmt.Sprintf("Error syncing folder '%s' to '%s': %v\nOutput: %s", folder, destRemote, err, out),
		})
	} else {
		logMessage(logger, logEntry{
			FolderName: folderName,
			Operation:  "SYNC",
			Message:    fmt.Sprintf("Folder '%s' synced successfully to '%s:%s'", folder, sourceRemote, destRemote),
		})
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
	if _, err = os.Stat(configFile); err == nil {
		logger.Println("Note Config file already exists at:", configFile)
		return nil
	} else if !os.IsNotExist(err) {
		logger.Printf("error checking for config file: %v", err)
		return err
	}

	err = os.MkdirAll(configDir, 0o755)
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
