package main

import (
	"github.com/osbuild/blueprint/pkg/blueprint"
)

// Import types from internal packages
// We'll add more imports as needed in later tasks

// DiscoveryEntry represents one distro+arch+image_type combination
type DiscoveryEntry struct {
	Distro    string `json:"distro"`
	Arch      string `json:"arch"`
	ImageType string `json:"image_type"`
}

// Placeholder to satisfy unused import (will be used in later tasks)
var _ *blueprint.Blueprint
