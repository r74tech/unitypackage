// Package upm converts Unity Package Manager and VRChat Package Manager
// package directories into legacy unitypackage archives. It does not resolve
// or download package dependencies.
package upm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is the subset of package.json used during conversion.
type Manifest struct {
	Name            string            `json:"name"`
	DisplayName     string            `json:"displayName"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	VPMDependencies map[string]string `json:"vpmDependencies"`
}

// ReadManifest reads and minimally validates package.json beneath root.
func ReadManifest(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read package.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse package.json: %w", err)
	}
	if manifest.Name == "" {
		return Manifest{}, errors.New("package.json has no name")
	}
	if strings.ContainsAny(manifest.Name, `/\\`) || manifest.Name == "." || manifest.Name == ".." {
		return Manifest{}, fmt.Errorf("package.json has invalid name %q", manifest.Name)
	}
	if manifest.Version == "" {
		return Manifest{}, errors.New("package.json has no version")
	}
	return manifest, nil
}
