package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// prefixWriter wraps log output with a prefix and writes line by line
type prefixWriter struct {
	prefix string
	buffer []byte
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
	// Append to buffer
	pw.buffer = append(pw.buffer, p...)

	// Process complete lines
	for {
		idx := strings.IndexByte(string(pw.buffer), '\n')
		if idx == -1 {
			break
		}

		// Extract line (without newline)
		line := string(pw.buffer[:idx])
		pw.buffer = pw.buffer[idx+1:]

		// Log with prefix
		log.Printf("%s%s", pw.prefix, line)
	}

	return len(p), nil
}

// processQueue runs the build queue processor
func processQueue(cacheDir string) {
	log.Printf("Queue processor started")

	for {
		// Find next waiting compose
		composeID, err := findNextWaitingCompose(cacheDir)
		if err != nil {
			log.Printf("Error finding waiting compose: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if composeID == "" {
			// No waiting composes, sleep and retry
			time.Sleep(5 * time.Second)
			continue
		}

		// Execute the compose
		if err := executeCompose(cacheDir, composeID); err != nil {
			log.Printf("Error executing compose %s: %v", composeID, err)
		}
	}
}

// findNextWaitingCompose returns the UUID of the oldest waiting compose
func findNextWaitingCompose(cacheDir string) (string, error) {
	uuids, err := ListComposes(cacheDir)
	if err != nil {
		return "", err
	}

	// Composes are already sorted by creation time (oldest first)
	for _, uuid := range uuids {
		status, err := LoadComposeStatus(cacheDir, uuid)
		if err != nil {
			continue
		}

		if status == "WAITING" {
			return uuid, nil
		}
	}

	return "", nil
}

// executeCompose builds the image using image-builder
func executeCompose(cacheDir, uuid string) error {
	log.Printf("[%s] Starting compose", uuid)

	// Load metadata
	metadata, err := LoadComposeMetadata(cacheDir, uuid)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	log.Printf("[%s] Loaded metadata: blueprint=%s, type=%s, distro=%s, arch=%s",
		uuid, metadata.BlueprintName, metadata.ComposeType, metadata.Distro, metadata.Arch)

	// Update status to RUNNING
	log.Printf("[%s] Setting status to RUNNING", uuid)
	if err := UpdateComposeStatus(cacheDir, uuid, "RUNNING"); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Update metadata with started time
	metadata.Started = time.Now()
	if err := updateComposeMetadata(cacheDir, metadata); err != nil {
		log.Printf("[%s] Warning: failed to update metadata: %v", uuid, err)
	}

	// Build command
	var args []string

	// Check for MANIFEST_ONLY mode
	manifestOnly := os.Getenv("MANIFEST_ONLY") != ""

	if manifestOnly {
		args = []string{
			"manifest",
			"--verbose",
			metadata.ComposeType,
		}
	} else {
		args = []string{
			"build",
			"--verbose",
			metadata.ComposeType,
		}
	}

	// Add common arguments
	args = append(args,
		"--blueprint", BlueprintPath(cacheDir, metadata.BlueprintName),
		"--distro", metadata.Distro,
		"--arch", metadata.Arch,
		"--output-dir", filepath.Join(ComposePath(cacheDir, uuid), "result"),
	)

	// Add cache directory for build mode
	if !manifestOnly {
		args = append(args, "--cache", filepath.Join(cacheDir, "store"))
	}

	// Save command line to metadata
	metadata.Command = imageBuilderBinary + " " + joinArgs(args)
	if err := updateComposeMetadata(cacheDir, metadata); err != nil {
		log.Printf("[%s] Warning: failed to update metadata: %v", uuid, err)
	}

	// Create command
	cmd := exec.Command(imageBuilderBinary, args...)

	// Create log writer with UUID prefix
	logWriter := &prefixWriter{prefix: fmt.Sprintf("[%s] ", uuid)}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	log.Printf("[%s] Executing: %s %s", uuid, imageBuilderBinary, joinArgs(args))

	// Start the process
	if err := cmd.Start(); err != nil {
		log.Printf("[%s] Failed to start: %v", uuid, err)
		UpdateComposeStatus(cacheDir, uuid, "FAILED")
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Save PID
	log.Printf("[%s] Process started with PID %d", uuid, cmd.Process.Pid)
	if err := SaveRunningPID(cacheDir, uuid, cmd.Process.Pid); err != nil {
		log.Printf("[%s] Warning: failed to save PID: %v", uuid, err)
	}

	// Wait for completion
	log.Printf("[%s] Waiting for process to complete...", uuid)
	err = cmd.Wait()

	// Remove PID file
	pidPath := filepath.Join(ComposePath(cacheDir, uuid), "pid")
	os.Remove(pidPath)

	// Update status based on result
	metadata.Finished = time.Now()
	updateComposeMetadata(cacheDir, metadata)

	if err != nil {
		log.Printf("[%s] Process failed: %v", uuid, err)
		UpdateComposeStatus(cacheDir, uuid, "FAILED")
		return err
	}

	log.Printf("[%s] Process completed successfully", uuid)
	UpdateComposeStatus(cacheDir, uuid, "FINISHED")

	return nil
}

// joinArgs joins command arguments for display
func joinArgs(args []string) string {
	var result strings.Builder
	for i, arg := range args {
		if i > 0 {
			result.WriteString(" ")
		}
		// Quote arguments with spaces
		if strings.Contains(arg, " ") {
			fmt.Fprintf(&result, "%q", arg)
		} else {
			result.WriteString(arg)
		}
	}
	return result.String()
}

// updateComposeMetadata writes updated metadata back to disk
func updateComposeMetadata(cacheDir string, metadata *ComposeMetadata) error {
	metadataPath := filepath.Join(ComposePath(cacheDir, metadata.ID), "metadata.json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, data, 0644)
}
