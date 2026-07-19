package unitypackage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/r74tech/unitypackage/internal/archive"
)

// Extract transactionally extracts a unitypackage into a Unity project tree.
// All archive paths and existing destinations are validated before commit.
func Extract(options ExtractOptions) error {
	if options.Input == "" {
		return fmt.Errorf("input path is required")
	}
	packageFile, err := archive.Open(options.Input, true, options.Limits.archiveLimits())
	if err != nil {
		return err
	}
	defer packageFile.Close()
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}

	for _, entry := range packageFile.Entries {
		destination := filepath.Join(root, filepath.FromSlash(entry.Path))
		if err := ensureInside(root, destination); err != nil {
			return err
		}
		if err := validateDestination(root, destination, entry.IsFolder, options.Overwrite); err != nil {
			return err
		}
		if err := validateDestination(root, destination+".meta", false, options.Overwrite); err != nil {
			return err
		}
	}
	return commitExtraction(root, packageFile.Entries, options.Overwrite)
}

func validateDestination(root, destination string, wantDirectory, overwrite bool) error {
	for current := destination; current != root; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: destination traverses symbolic link: %s", ErrConflict, current)
			}
			if current != destination && !info.IsDir() {
				return fmt.Errorf("%w: destination parent is not a directory: %s", ErrConflict, current)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if wantDirectory {
		if !info.IsDir() {
			return fmt.Errorf("%w: destination type conflicts with package folder: %s", ErrConflict, destination)
		}
		return nil
	}
	if info.IsDir() {
		return fmt.Errorf("%w: destination type conflicts with package file: %s", ErrConflict, destination)
	}
	if !overwrite {
		return fmt.Errorf("%w: destination exists: %s (enable overwrite)", ErrConflict, destination)
	}
	return nil
}

type preparedFile struct {
	staged      string
	destination string
}

type installedFile struct {
	destination string
	backup      string
	installed   bool
}

func commitExtraction(root string, entries []archive.Entry, overwrite bool) error {
	transaction, err := os.MkdirTemp(root, ".unitypackage-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(transaction)
	backupDir := filepath.Join(transaction, "backups")
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		return err
	}

	prepared := make([]preparedFile, 0, len(entries)*2)
	for index, entry := range entries {
		destination := filepath.Join(root, filepath.FromSlash(entry.Path))
		if !entry.IsFolder {
			staged := filepath.Join(transaction, fmt.Sprintf("%06d-asset", index))
			if err := copyPath(entry.AssetPath, staged); err != nil {
				return err
			}
			prepared = append(prepared, preparedFile{staged: staged, destination: destination})
		}
		stagedMeta := filepath.Join(transaction, fmt.Sprintf("%06d-meta", index))
		if err := os.WriteFile(stagedMeta, entry.Meta, 0o600); err != nil {
			return err
		}
		prepared = append(prepared, preparedFile{staged: stagedMeta, destination: destination + ".meta"})
	}

	createdDirectories := make([]string, 0)
	rollbackDirectories := func() {
		for index := len(createdDirectories) - 1; index >= 0; index-- {
			_ = os.Remove(createdDirectories[index])
		}
	}
	for _, entry := range entries {
		destination := filepath.Join(root, filepath.FromSlash(entry.Path))
		directory := filepath.Dir(destination)
		if entry.IsFolder {
			directory = destination
		}
		if err := ensureDirectories(root, directory, &createdDirectories); err != nil {
			rollbackDirectories()
			return err
		}
	}

	installed := make([]installedFile, 0, len(prepared))
	rollback := func() {
		for index := len(installed) - 1; index >= 0; index-- {
			item := installed[index]
			if item.installed {
				_ = os.Remove(item.destination)
			}
			if item.backup != "" {
				_ = os.Rename(item.backup, item.destination)
			}
		}
		rollbackDirectories()
	}
	for index, item := range prepared {
		journal := installedFile{destination: item.destination}
		if _, err := os.Lstat(item.destination); err == nil {
			if !overwrite {
				rollback()
				return fmt.Errorf("%w: destination exists: %s", ErrConflict, item.destination)
			}
			journal.backup = filepath.Join(backupDir, fmt.Sprintf("%06d", index))
			if err := os.Rename(item.destination, journal.backup); err != nil {
				rollback()
				return err
			}
		} else if !os.IsNotExist(err) {
			rollback()
			return err
		}
		installed = append(installed, journal)
		if err := os.Rename(item.staged, item.destination); err != nil {
			rollback()
			return err
		}
		installed[len(installed)-1].installed = true
	}
	return nil
}

func ensureDirectories(root, directory string, created *[]string) error {
	if directory == root {
		return nil
	}
	if err := ensureInside(root, directory); err != nil {
		return err
	}
	if info, err := os.Lstat(directory); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: destination parent is not a directory: %s", ErrConflict, directory)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := ensureDirectories(root, filepath.Dir(directory), created); err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return err
	}
	*created = append(*created, directory)
	return nil
}

func copyPath(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
