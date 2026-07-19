package testfixture

import (
	"fmt"
	"os"
	"path/filepath"
)

func ensureCacheRoot(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return validateCacheDirectory(path)
}

func ensureCacheDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	return validateCacheDirectory(path)
}

func validateCacheDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("cache path is not a regular directory: %s", path)
	}
	return nil
}

// publishPath installs staged at destination and safely replaces an existing
// cache entry. The old entry remains available for rollback until the new one
// has been installed.
func publishPath(staged, destination string) error {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return os.Rename(staged, destination)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace symbolic link %s", destination)
	}
	if !info.IsDir() {
		if err := os.Rename(staged, destination); err == nil {
			return nil
		}
	}

	backupRoot, err := os.MkdirTemp(filepath.Dir(destination), ".replace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backupRoot)
	backup := filepath.Join(backupRoot, "previous")
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, destination); err != nil {
		if rollbackErr := os.Rename(backup, destination); rollbackErr != nil {
			return fmt.Errorf("publish cache entry: %w (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}
	return nil
}
