package testfixture

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Manifest declares immutable external test artifacts.
type Manifest struct {
	Artifacts []Artifact `json:"artifacts"`
}

// Artifact describes one content-addressed fixture source.
type Artifact struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Publisher string         `json:"publisher"`
	Source    string         `json:"source"`
	URL       string         `json:"url"`
	Filename  string         `json:"filename"`
	Kind      string         `json:"kind"`
	Size      int64          `json:"size"`
	SHA256    string         `json:"sha256"`
	Expected  map[string]int `json:"expected"`
}

// LoadManifest reads and validates a fixture manifest.
func LoadManifest(filename string) (Manifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode fixture manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Manifest{}, err
	}
	if len(manifest.Artifacts) == 0 {
		return Manifest{}, fmt.Errorf("fixture manifest has no artifacts")
	}
	seen := make(map[string]bool)
	for index, artifact := range manifest.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return Manifest{}, fmt.Errorf("artifact %d: %w", index, err)
		}
		if seen[artifact.ID] {
			return Manifest{}, fmt.Errorf("duplicate artifact id %q", artifact.ID)
		}
		seen[artifact.ID] = true
	}
	return manifest, nil
}

func validateArtifact(artifact Artifact) error {
	if !isSafeIdentifier(artifact.ID) {
		return fmt.Errorf("invalid id %q", artifact.ID)
	}
	parsedURL, err := url.Parse(artifact.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return fmt.Errorf("invalid URL %q", artifact.URL)
	}
	if artifact.Filename == "" || artifact.Filename == "." || artifact.Filename == ".." ||
		strings.ContainsRune(artifact.Filename, '\x00') || filepath.Base(artifact.Filename) != artifact.Filename {
		return fmt.Errorf("invalid filename %q", artifact.Filename)
	}
	if artifact.Kind != "file" && artifact.Kind != "zip" {
		return fmt.Errorf("unsupported kind %q", artifact.Kind)
	}
	if artifact.Size <= 0 {
		return fmt.Errorf("invalid size %d", artifact.Size)
	}
	if len(artifact.SHA256) != 64 {
		return fmt.Errorf("invalid SHA-256 %q", artifact.SHA256)
	}
	if _, err := hex.DecodeString(artifact.SHA256); err != nil {
		return fmt.Errorf("invalid SHA-256 %q", artifact.SHA256)
	}
	if artifact.SHA256 != strings.ToLower(artifact.SHA256) {
		return fmt.Errorf("SHA-256 must use lowercase hexadecimal")
	}
	return nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("fixture manifest contains trailing JSON")
		}
		return fmt.Errorf("decode fixture manifest: %w", err)
	}
	return nil
}

func isSafeIdentifier(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
