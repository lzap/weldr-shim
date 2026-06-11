package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
)

// Global image types cache loaded on startup.
// SAFETY: Must only be written during single-threaded startup (via LoadImageTypesCache)
// before serving HTTP requests. All query functions are safe for concurrent reads.
var imageTypesCache []DiscoveryEntry

// imageBuilderBinary holds the detected binary name (image-builder or image-builder-cli)
var imageBuilderBinary string

// DetectImageBuilder finds which image-builder binary is available
func DetectImageBuilder() error {
	// Try new name first
	if _, err := exec.LookPath("image-builder"); err == nil {
		imageBuilderBinary = "image-builder"
		return nil
	}

	// Try old name
	if _, err := exec.LookPath("image-builder-cli"); err == nil {
		imageBuilderBinary = "image-builder-cli"
		return nil
	}

	return fmt.Errorf("neither 'image-builder' nor 'image-builder-cli' found in PATH")
}

// LoadImageTypesCache calls image-builder and caches the results.
// If this fails, the cache remains empty and all query functions will return
// empty results. Callers should check the returned error and fail startup if needed.
func LoadImageTypesCache() error {
	cmd := exec.Command(imageBuilderBinary, "list", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run %s: %w\nOutput: %s", imageBuilderBinary, err, output)
	}

	// Parse the JSON output
	var rawEntries []struct {
		Distro struct {
			Name string `json:"name"`
		} `json:"distro"`
		Arch struct {
			Name string `json:"name"`
		} `json:"arch"`
		ImageType struct {
			Name string `json:"name"`
		} `json:"image_type"`
	}

	if err := json.Unmarshal(output, &rawEntries); err != nil {
		return fmt.Errorf("failed to parse %s output: %w", imageBuilderBinary, err)
	}

	// Convert to our internal format
	imageTypesCache = make([]DiscoveryEntry, len(rawEntries))
	for i, entry := range rawEntries {
		imageTypesCache[i] = DiscoveryEntry{
			Distro:    entry.Distro.Name,
			Arch:      entry.Arch.Name,
			ImageType: entry.ImageType.Name,
		}
	}

	return nil
}

// GetDistros returns unique distro names, optionally filtered by arch
func GetDistros(arch string) []string {
	distros := make(map[string]bool)

	for _, entry := range imageTypesCache {
		if arch == "" || entry.Arch == arch {
			distros[entry.Distro] = true
		}
	}

	// Convert to sorted slice
	result := make([]string, 0, len(distros))
	for distro := range distros {
		result = append(result, distro)
	}
	sort.Strings(result)

	return result
}

// GetImageTypes returns unique image types for the given distro and arch
func GetImageTypes(distro, arch string) []string {
	imageTypes := make(map[string]bool)

	for _, entry := range imageTypesCache {
		if (distro == "" || entry.Distro == distro) &&
			(arch == "" || entry.Arch == arch) {
			imageTypes[entry.ImageType] = true
		}
	}

	// Convert to sorted slice
	result := make([]string, 0, len(imageTypes))
	for imageType := range imageTypes {
		result = append(result, imageType)
	}
	sort.Strings(result)

	return result
}

// ValidateImageType checks if an image type exists for the given distro and arch
func ValidateImageType(distro, arch, imageType string) bool {
	for _, entry := range imageTypesCache {
		if entry.Distro == distro &&
			entry.Arch == arch &&
			entry.ImageType == imageType {
			return true
		}
	}
	return false
}
