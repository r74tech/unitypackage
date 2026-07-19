package unitypackage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/r74tech/unitypackage/internal/archive"
)

// Pack creates a deterministic legacy unitypackage from selected project
// assets. Relative paths and glob patterns are resolved against options.Root.
func Pack(options PackOptions) error {
	if options.Output == "" {
		return fmt.Errorf("output path is required")
	}
	if !strings.EqualFold(filepath.Ext(options.Output), ".unitypackage") {
		return fmt.Errorf("output path %q must end with .unitypackage", options.Output)
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(options.PathPrefix), "/")
	if prefix != "" {
		prefix, err = archive.ValidatePathname(prefix)
		if err != nil {
			return fmt.Errorf("invalid path prefix: %w", err)
		}
	}

	paths, err := selectPaths(root, options.Paths, options.Includes, options.Excludes)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return ErrNoAssets
	}

	entries := make(map[string]archive.Entry)
	for _, assetPath := range paths {
		if err := addAsset(root, prefix, assetPath, entries); err != nil {
			return err
		}
		if err := addParents(root, prefix, assetPath, entries); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(options.Output), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	return archive.WriteFile(options.Output, entries)
}

func selectPaths(root string, explicit, includes, excludes []string) ([]string, error) {
	selected := make(map[string]struct{})
	add := func(pathname string) error {
		if strings.HasSuffix(pathname, ".meta") {
			pathname = strings.TrimSuffix(pathname, ".meta")
		}
		if !filepath.IsAbs(pathname) {
			pathname = filepath.Join(root, pathname)
		}
		absolute, err := filepath.Abs(pathname)
		if err != nil {
			return err
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return fmt.Errorf("asset %q: %w", pathname, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("asset %q: symbolic links are not supported", pathname)
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return fmt.Errorf("resolve asset %q: %w", pathname, err)
		}
		if err := ensureInside(root, resolved); err != nil {
			return err
		}
		absolute = resolved
		if !info.IsDir() {
			selected[absolute] = struct{}{}
			return nil
		}
		if _, err := os.Stat(absolute + ".meta"); err == nil {
			selected[absolute] = struct{}{}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read directory meta %q: %w", absolute+".meta", err)
		} else if absolute != root && !isProjectContainer(root, absolute) {
			return fmt.Errorf("%w: directory %q has no .meta file", ErrMissingMeta, absolute)
		}
		return filepath.WalkDir(absolute, func(child string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if child == absolute || strings.HasSuffix(child, ".meta") {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("asset %q: symbolic links are not supported", child)
			}
			if entry.IsDir() {
				if _, err := os.Stat(child + ".meta"); err == nil {
					selected[child] = struct{}{}
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("read directory meta %q: %w", child+".meta", err)
				}
				return nil
			}
			selected[child] = struct{}{}
			return nil
		})
	}

	for _, pathname := range explicit {
		if err := add(pathname); err != nil {
			return nil, err
		}
	}
	if len(includes) > 0 {
		err := filepath.WalkDir(root, func(pathname string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if pathname == root || strings.HasSuffix(pathname, ".meta") {
				return nil
			}
			relative, err := filepath.Rel(root, pathname)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if !anyMatch(includes, relative) || anyMatch(excludes, relative) {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("asset %q: symbolic links are not supported", pathname)
			}
			if entry.IsDir() {
				if _, err := os.Stat(pathname + ".meta"); err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
			}
			selected[pathname] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("expand include patterns: %w", err)
		}
	}

	result := make([]string, 0, len(selected))
	for pathname := range selected {
		result = append(result, pathname)
	}
	sort.Strings(result)
	return result, nil
}

func anyMatch(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if archive.MatchGlob(pattern, name) {
			return true
		}
	}
	return false
}

func addParents(root, prefix, assetPath string, entries map[string]archive.Entry) error {
	for parent := filepath.Dir(assetPath); parent != root; parent = filepath.Dir(parent) {
		if err := ensureInside(root, parent); err != nil {
			return err
		}
		metaPath := parent + ".meta"
		if _, err := os.Stat(metaPath); err == nil {
			if err := addAsset(root, prefix, parent, entries); err != nil {
				return err
			}
		} else if os.IsNotExist(err) && isProjectContainer(root, parent) {
			continue
		} else if os.IsNotExist(err) {
			return fmt.Errorf("%w: parent directory %q has no .meta file", ErrMissingMeta, parent)
		} else {
			return fmt.Errorf("read parent meta %q: %w", metaPath, err)
		}
	}
	return nil
}

func isProjectContainer(root, directory string) bool {
	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return false
	}
	relative = filepath.ToSlash(relative)
	return relative == "Assets" || relative == "Packages"
}

func addAsset(root, prefix, assetPath string, entries map[string]archive.Entry) error {
	if err := ensureInside(root, assetPath); err != nil {
		return err
	}
	info, err := os.Lstat(assetPath)
	if err != nil {
		return fmt.Errorf("read asset %q: %w", assetPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("asset %q: symbolic links are not supported", assetPath)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("asset %q is not a regular file", assetPath)
	}
	metaPath := assetPath + ".meta"
	metaInfo, err := os.Lstat(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %q", ErrMissingMeta, metaPath)
		}
		return fmt.Errorf("read meta %q: %w", metaPath, err)
	}
	if !metaInfo.Mode().IsRegular() {
		return fmt.Errorf("meta %q is not a regular file", metaPath)
	}
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read meta %q: %w", metaPath, err)
	}
	meta, err := archive.ParseMeta(metaData)
	if err != nil {
		return fmt.Errorf("parse meta %q: %w", metaPath, err)
	}
	if info.IsDir() != meta.IsFolder {
		return fmt.Errorf("meta %q folderAsset does not match the asset type", metaPath)
	}
	relative, err := filepath.Rel(root, assetPath)
	if err != nil {
		return err
	}
	archivePath := filepath.ToSlash(relative)
	if prefix != "" {
		archivePath = pathJoin(prefix, archivePath)
	}
	archivePath, err = archive.ValidatePathname(archivePath)
	if err != nil {
		return fmt.Errorf("asset %q: %w", assetPath, err)
	}
	entry := archive.Entry{
		GUID: meta.GUID, Path: archivePath, IsFolder: info.IsDir(),
		HasAsset: !info.IsDir(), Meta: metaData,
	}
	if !info.IsDir() {
		entry.AssetPath = assetPath
	}
	if previous, exists := entries[meta.GUID]; exists && previous.Path != entry.Path {
		return fmt.Errorf("duplicate GUID %s for %q and %q", meta.GUID, previous.Path, entry.Path)
	}
	entries[meta.GUID] = entry
	return nil
}

func pathJoin(prefix, name string) string {
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(name, "/")
}
