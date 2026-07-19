package upm

import (
	"fmt"
	"path/filepath"
	"strings"

	unitypackage "github.com/r74tech/unitypackage"
)

// PackOptions configures conversion of a UPM or VPM package directory.
type PackOptions struct {
	Root        string
	Output      string
	Destination string
	Includes    []string
	Excludes    []string
}

// Pack converts a UPM or VPM package directory into a legacy unitypackage. By
// default, paths are mounted beneath Packages/<manifest.name>. Dependencies are
// intentionally not resolved or bundled.
func Pack(options PackOptions) error {
	manifest, err := ReadManifest(options.Root)
	if err != nil {
		return err
	}
	destination := options.Destination
	if destination == "" {
		destination = "Packages/" + manifest.Name
	}
	destination = strings.TrimSuffix(filepath.ToSlash(destination), "/")
	if destination != "Assets" && destination != "Packages" &&
		!strings.HasPrefix(destination, "Assets/") && !strings.HasPrefix(destination, "Packages/") {
		return fmt.Errorf("destination %q must be under Assets or Packages", destination)
	}

	includes := options.Includes
	if len(includes) == 0 {
		includes = []string{"**"}
	}
	excludes := append([]string{
		".git/**", ".github/**", ".gitignore", ".gitattributes",
		".DS_Store", "**/.DS_Store",
	}, options.Excludes...)
	return unitypackage.Pack(unitypackage.PackOptions{
		Root: options.Root, Output: options.Output, PathPrefix: destination,
		Includes: includes, Excludes: excludes,
	})
}
