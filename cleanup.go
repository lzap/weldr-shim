package main

import (
	"log"
	"os"
	"syscall"
)

// CleanupStaleComposes checks for composes stuck in RUNNING state
// due to killed processes and transitions them to FAILED
func CleanupStaleComposes(cacheDir string) {
	uuids, err := ListComposes(cacheDir)
	if err != nil {
		log.Printf("Warning: failed to list composes during cleanup: %v", err)
		return
	}

	for _, uuid := range uuids {
		status, err := LoadComposeStatus(cacheDir, uuid)
		if err != nil || status != "RUNNING" {
			continue
		}

		// Compose is marked as RUNNING - check if process is actually alive
		pid, err := GetRunningPID(cacheDir, uuid)
		if err != nil {
			// No PID file - process completed but didn't update status (crashed?)
			log.Printf("[%s] No PID file found for RUNNING compose - marking as FAILED", uuid)
			UpdateComposeStatus(cacheDir, uuid, "FAILED")
			continue
		}

		// Check if process with this PID exists and is running
		if !isProcessRunning(pid) {
			log.Printf("[%s] Process %d no longer running - marking as FAILED", uuid, pid)
			UpdateComposeStatus(cacheDir, uuid, "FAILED")
		}
	}
}

// isProcessRunning checks if a process with the given PID exists
func isProcessRunning(pid int) bool {
	// Send signal 0 to check if process exists (doesn't actually send a signal)
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist or we don't have permission
		return false
	}

	return true
}
