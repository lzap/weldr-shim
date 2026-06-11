package main

import (
	"fmt"
	"regexp"
)

// Valid blueprint name: alphanumeric, dash, underscore, dot (no path separators)
var blueprintNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// Valid UUID format (used for compose IDs)
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidateBlueprintName checks if a blueprint name is safe to use
func ValidateBlueprintName(name string) error {
	if name == "" {
		return fmt.Errorf("blueprint name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("blueprint name too long (max 255 characters)")
	}

	// Reject path separators and path traversal
	if !blueprintNameRegex.MatchString(name) {
		return fmt.Errorf("blueprint name contains invalid characters (only alphanumeric, dash, underscore, dot allowed)")
	}

	// Explicitly reject dangerous patterns
	if name == "." || name == ".." {
		return fmt.Errorf("invalid blueprint name")
	}

	return nil
}

// ValidateUUID checks if a string is a valid UUID
func ValidateUUID(uuid string) error {
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}
