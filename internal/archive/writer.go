package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// WriteFile writes entries as a deterministic legacy unitypackage archive.
func WriteFile(output string, entries map[string]Entry) (returnErr error) {
	entryList := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		entryList = append(entryList, entry)
	}
	if err := validateEntrySet(entryList); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".unitypackage-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()

	gz, err := gzip.NewWriterLevel(temporary, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.Name = ""
	gz.Comment = ""
	gz.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)

	guids := make([]string, 0, len(entries))
	for guid := range entries {
		guids = append(guids, guid)
	}
	sort.Strings(guids)
	for _, guid := range guids {
		entry := entries[guid]
		if err := writeDirectory(tw, guid+"/", 0o755); err != nil {
			return err
		}
		if entry.HasAsset {
			asset, err := os.Open(entry.AssetPath)
			if err != nil {
				return err
			}
			info, statErr := asset.Stat()
			if statErr == nil {
				statErr = writeStream(tw, guid+"/asset", asset, info.Size(), 0o644)
			}
			closeErr := asset.Close()
			if statErr != nil {
				return statErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if err := writeBytes(tw, guid+"/asset.meta", entry.Meta, 0o644); err != nil {
			return err
		}
		if err := writeBytes(tw, guid+"/pathname", []byte(entry.Path), 0o644); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("finish tar archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finish gzip archive: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := publish(temporaryName, output); err != nil {
		return fmt.Errorf("publish output: %w", err)
	}
	return nil
}

func writeBytes(tw *tar.Writer, name string, data []byte, mode int64) error {
	return writeStream(tw, name, bytes.NewReader(data), int64(len(data)), mode)
}

func writeStream(tw *tar.Writer, name string, reader io.Reader, size, mode int64) error {
	header := &tar.Header{
		Name: name, Mode: mode, Size: size, ModTime: time.Unix(0, 0),
		AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0, Format: tar.FormatGNU,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write %s header: %w", name, err)
	}
	if _, err := io.CopyN(tw, reader, size); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func writeDirectory(tw *tar.Writer, name string, mode int64) error {
	header := &tar.Header{
		Name: name, Typeflag: tar.TypeDir, Mode: mode, ModTime: time.Unix(0, 0),
		AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0, Format: tar.FormatGNU,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write %s header: %w", name, err)
	}
	return nil
}

func publish(temporary, output string) error {
	renameErr := os.Rename(temporary, output)
	if renameErr == nil {
		return nil
	}
	info, statErr := os.Lstat(output)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return renameErr
		}
		return statErr
	}
	if info.IsDir() {
		return fmt.Errorf("output path is a directory: %s", output)
	}
	backup, err := os.CreateTemp(filepath.Dir(output), ".unitypackage-backup-*")
	if err != nil {
		return err
	}
	backupName := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupName)
		return err
	}
	if err := os.Remove(backupName); err != nil {
		return err
	}
	if err := os.Rename(output, backupName); err != nil {
		return err
	}
	if err := os.Rename(temporary, output); err != nil {
		if rollbackErr := os.Rename(backupName, output); rollbackErr != nil {
			return fmt.Errorf("replace output: %w (restore previous output: %v)", err, rollbackErr)
		}
		return err
	}
	_ = os.Remove(backupName)
	return nil
}
