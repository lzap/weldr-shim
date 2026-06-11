package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// InitializeStorage creates the cache directory structure
func InitializeStorage(cacheDir string) error {
	dirs := []string{
		filepath.Join(cacheDir, "blueprints"),
		filepath.Join(cacheDir, "composes"),
		filepath.Join(cacheDir, "store"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// BlueprintPath returns the path to a blueprint file
func BlueprintPath(cacheDir, name string) string {
	return filepath.Join(cacheDir, "blueprints", filepath.Base(name)+".json")
}

// ComposePath returns the path to a compose directory
func ComposePath(cacheDir, uuid string) string {
	return filepath.Join(cacheDir, "composes", filepath.Base(uuid))
}

// SaveBlueprint writes a blueprint to disk
func SaveBlueprint(cacheDir, name string, data []byte) error {
	path := BlueprintPath(cacheDir, name)

	// Atomic write: write to temp file, then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write blueprint: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to save blueprint: %w", err)
	}

	return nil
}

// LoadBlueprint reads a blueprint from disk
func LoadBlueprint(cacheDir, name string) ([]byte, error) {
	path := BlueprintPath(cacheDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blueprint not found: %s", name)
		}
		return nil, fmt.Errorf("failed to read blueprint: %w", err)
	}
	return data, nil
}

// ListBlueprints returns all blueprint names
func ListBlueprints(cacheDir string) ([]string, error) {
	blueprintsDir := filepath.Join(cacheDir, "blueprints")
	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list blueprints: %w", err)
	}

	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			name := strings.TrimSuffix(entry.Name(), ".json")
			names = append(names, name)
		}
	}

	return names, nil
}

// DeleteBlueprint removes a blueprint file
func DeleteBlueprint(cacheDir, name string) error {
	path := BlueprintPath(cacheDir, name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blueprint: %w", err)
	}
	return nil
}

// ComposeMetadata stores compose information
type ComposeMetadata struct {
	ID               string    `json:"id"`
	BlueprintName    string    `json:"blueprint_name"`
	BlueprintVersion string    `json:"blueprint_version"`
	ComposeType      string    `json:"compose_type"`
	Distro           string    `json:"distro"`
	Arch             string    `json:"arch"`
	Created          time.Time `json:"created"`
	Started          time.Time `json:"started,omitzero"`
	Finished         time.Time `json:"finished,omitzero"`
	Command          string    `json:"command,omitempty"`
}

// CreateCompose creates a new compose directory structure
func CreateCompose(cacheDir string, metadata ComposeMetadata) error {
	composePath := ComposePath(cacheDir, metadata.ID)

	// Create compose directory
	if err := os.MkdirAll(composePath, 0755); err != nil {
		return fmt.Errorf("failed to create compose directory: %w", err)
	}

	// Create result directory
	resultPath := filepath.Join(composePath, "result")
	if err := os.MkdirAll(resultPath, 0755); err != nil {
		return fmt.Errorf("failed to create result directory: %w", err)
	}

	// Write metadata
	metadataPath := filepath.Join(composePath, "metadata.json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// Write initial status
	if err := UpdateComposeStatus(cacheDir, metadata.ID, "WAITING"); err != nil {
		return err
	}

	return nil
}

// UpdateComposeStatus writes the status file
func UpdateComposeStatus(cacheDir, uuid, status string) error {
	statusPath := filepath.Join(ComposePath(cacheDir, uuid), "status")

	// Atomic write: write to temp file, then rename
	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(status+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to save status: %w", err)
	}

	return nil
}

// LoadComposeStatus reads the status file
func LoadComposeStatus(cacheDir, uuid string) (string, error) {
	statusPath := filepath.Join(ComposePath(cacheDir, uuid), "status")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return "", fmt.Errorf("failed to read status: %w", err)
	}
	if len(data) == 0 {
		return "", nil
	}
	return string(data[:len(data)-1]), nil // strip newline
}

// LoadComposeMetadata reads the metadata file
func LoadComposeMetadata(cacheDir, uuid string) (*ComposeMetadata, error) {
	metadataPath := filepath.Join(ComposePath(cacheDir, uuid), "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata ComposeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// ListComposes returns all compose UUIDs sorted by creation time
func ListComposes(cacheDir string) ([]string, error) {
	composesDir := filepath.Join(cacheDir, "composes")
	entries, err := os.ReadDir(composesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list composes: %w", err)
	}

	// Extract directory entries (UUIDs)
	uuids := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			uuids = append(uuids, entry.Name())
		}
	}

	// Sort by creation time (filesystem modification time of directory)
	type composeEntry struct {
		uuid string
		time time.Time
	}

	entries2 := make([]composeEntry, 0, len(uuids))
	for _, uuid := range uuids {
		info, err := os.Stat(ComposePath(cacheDir, uuid))
		if err == nil {
			entries2 = append(entries2, composeEntry{uuid, info.ModTime()})
		}
	}

	// Sort by time
	sort.Slice(entries2, func(i, j int) bool {
		return entries2[i].time.Before(entries2[j].time)
	})

	result := make([]string, len(entries2))
	for i, e := range entries2 {
		result[i] = e.uuid
	}

	return result, nil
}

// DeleteCompose removes a compose directory
func DeleteCompose(cacheDir, uuid string) error {
	composePath := ComposePath(cacheDir, uuid)
	return os.RemoveAll(composePath)
}

// SaveRunningPID writes the process ID file
func SaveRunningPID(cacheDir, uuid string, pid int) error {
	pidPath := filepath.Join(ComposePath(cacheDir, uuid), "pid")
	return os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", pid), 0644)
}

// GetRunningPID reads the process ID file
func GetRunningPID(cacheDir, uuid string) (int, error) {
	pidPath := filepath.Join(ComposePath(cacheDir, uuid), "pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}

	return pid, nil
}

// GetComposeImagePath finds the image file in the result directory
func GetComposeImagePath(cacheDir, uuid string) (string, int64, error) {
	resultDir := filepath.Join(ComposePath(cacheDir, uuid), "result")
	entries, err := os.ReadDir(resultDir)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read result directory: %w", err)
	}

	// Find the first file (should only be one)
	for _, entry := range entries {
		if !entry.IsDir() {
			path := filepath.Join(resultDir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				return "", 0, fmt.Errorf("failed to get file info: %w", err)
			}
			return path, info.Size(), nil
		}
	}

	return "", 0, fmt.Errorf("no image file found in result directory")
}
