package testfixture

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	unityarchive "github.com/r74tech/unitypackage/internal/archive"
)

const (
	// These limits follow the Go module ecosystem's 500 MiB uncompressed
	// archive ceiling while also bounding per-entry processing overhead.
	DefaultMaxZIPEntries          = 100_000
	DefaultMaxZIPUncompressedSize = uint64(500 << 20)
	// Increment when extraction validation or materialization semantics change.
	extractionCacheVersion = "1"
)

// ZIPLimits bounds the resources used while materializing a ZIP fixture. Zero
// values select the defaults; callers may raise or lower either limit.
type ZIPLimits struct {
	MaxEntries          int
	MaxUncompressedSize uint64
}

func (limits ZIPLimits) withDefaults() ZIPLimits {
	if limits.MaxEntries == 0 {
		limits.MaxEntries = DefaultMaxZIPEntries
	}
	if limits.MaxUncompressedSize == 0 {
		limits.MaxUncompressedSize = DefaultMaxZIPUncompressedSize
	}
	return limits
}

func extractZIP(root string, artifact Artifact, blob string, limits ZIPLimits) (string, error) {
	limits = limits.withDefaults()
	extractedRoot := filepath.Join(root, "extracted")
	finalRoot := filepath.Join(extractedRoot, artifact.SHA256)
	finalContent := filepath.Join(finalRoot, "content")
	marker := filepath.Join(finalRoot, "complete")
	expectedMarker := extractionMarker(artifact, limits)
	if err := ensureCacheDirectory(extractedRoot); err != nil {
		return "", err
	}
	if info, err := os.Lstat(finalRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to replace symbolic link %s", finalRoot)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if data, err := os.ReadFile(marker); err == nil && string(data) == expectedMarker {
		return finalContent, nil
	}
	temporaryRoot, err := os.MkdirTemp(extractedRoot, ".extract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporaryRoot)
	temporaryContent := filepath.Join(temporaryRoot, "content")
	if err := os.Mkdir(temporaryContent, 0o700); err != nil {
		return "", err
	}
	reader, err := zip.OpenReader(blob)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if err := validateZIP(reader.File, temporaryContent, limits); err != nil {
		return "", err
	}
	for _, file := range reader.File {
		if err := extractZIPEntry(temporaryContent, file); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "complete"), []byte(expectedMarker), 0o600); err != nil {
		return "", err
	}
	if data, readErr := os.ReadFile(marker); readErr == nil && string(data) == expectedMarker {
		return finalContent, nil
	}
	if publishErr := publishPath(temporaryRoot, finalRoot); publishErr != nil {
		return "", fmt.Errorf("publish extracted fixture: %w", publishErr)
	}
	return finalContent, nil
}

func extractionMarker(artifact Artifact, limits ZIPLimits) string {
	return fmt.Sprintf(
		"version=%s\nsha256=%s\nmaxEntries=%d\nmaxUncompressedSize=%d\n",
		extractionCacheVersion, artifact.SHA256, limits.MaxEntries, limits.MaxUncompressedSize,
	)
}

func validateZIP(files []*zip.File, root string, limits ZIPLimits) error {
	limits = limits.withDefaults()
	if len(files) > limits.MaxEntries {
		return fmt.Errorf("ZIP contains too many entries: got %d, maximum %d", len(files), limits.MaxEntries)
	}
	type entryType struct {
		name      string
		directory bool
	}
	entries := make(map[string]entryType, len(files))
	var total uint64
	for _, file := range files {
		name, _, err := validateZIPEntry(root, file)
		if err != nil {
			return err
		}
		if file.UncompressedSize64 >= 1<<63-1 {
			return fmt.Errorf("ZIP entry %q is too large to extract safely", file.Name)
		}
		key := strings.ToLower(name)
		if previous, exists := entries[key]; exists {
			return fmt.Errorf("duplicate or case-conflicting ZIP paths %q and %q", previous.name, file.Name)
		}
		entries[key] = entryType{name: name, directory: file.FileInfo().IsDir()}
		if file.UncompressedSize64 > limits.MaxUncompressedSize-total {
			return fmt.Errorf("ZIP expands beyond %d bytes", limits.MaxUncompressedSize)
		}
		total += file.UncompressedSize64
	}
	for key, entry := range entries {
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if ancestor, exists := entries[parent]; exists && !ancestor.directory {
				return fmt.Errorf("ZIP file %q is an ancestor of %q", ancestor.name, entry.name)
			}
		}
	}
	return nil
}

func validateZIPEntry(root string, file *zip.File) (string, string, error) {
	isDirectory := file.FileInfo().IsDir()
	name := file.Name
	if isDirectory {
		if !strings.HasSuffix(name, "/") {
			return "", "", fmt.Errorf("ZIP directory lacks trailing slash: %q", file.Name)
		}
		name = strings.TrimSuffix(name, "/")
	} else if strings.HasSuffix(name, "/") {
		return "", "", fmt.Errorf("ZIP non-directory has trailing slash: %q", file.Name)
	}
	clean, err := unityarchive.ValidatePathname(name)
	if err != nil {
		return "", "", fmt.Errorf("unsafe ZIP path %q: %w", file.Name, err)
	}
	if file.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("ZIP symlink is not supported: %q", file.Name)
	}
	if !isDirectory && !file.Mode().IsRegular() {
		return "", "", fmt.Errorf("unsupported ZIP entry type: %q", file.Name)
	}
	if isDirectory && file.UncompressedSize64 != 0 {
		return "", "", fmt.Errorf("ZIP directory contains a payload: %q", file.Name)
	}
	destination := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", "", fmt.Errorf("unsafe ZIP path %q", file.Name)
	}
	return clean, destination, nil
}

func extractZIPEntry(root string, file *zip.File) error {
	_, destination, err := validateZIPEntry(root, file)
	if err != nil {
		return err
	}
	if file.FileInfo().IsDir() {
		return os.MkdirAll(destination, 0o700)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	input, err := file.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.CopyN(output, input, int64(file.UncompressedSize64)+1)
	closeErr := output.Close()
	if copyErr != nil && copyErr != io.EOF {
		return copyErr
	}
	if written != int64(file.UncompressedSize64) {
		return fmt.Errorf("ZIP entry %q size mismatch: got %d, want %d", file.Name, written, file.UncompressedSize64)
	}
	return closeErr
}
