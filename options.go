package unitypackage

import "github.com/r74tech/unitypackage/internal/archive"

const (
	// DefaultMaxArchiveEntries is the default maximum number of tar entries
	// processed while inspecting, verifying, or extracting an archive.
	DefaultMaxArchiveEntries = archive.DefaultMaxEntries
	// DefaultMaxArchiveUncompressedSize is the default maximum total size of
	// the tar stream after gzip decompression.
	DefaultMaxArchiveUncompressedSize = archive.DefaultMaxUncompressedSize
)

// ReadLimits bounds resources used while reading an archive. Zero values use
// the defaults.
type ReadLimits struct {
	MaxEntries          int
	MaxUncompressedSize uint64
}

func (limits ReadLimits) archiveLimits() archive.Limits {
	return archive.Limits{
		MaxEntries: limits.MaxEntries, MaxUncompressedSize: limits.MaxUncompressedSize,
	}
}

// PackOptions configures creation of a unitypackage from a Unity project tree.
type PackOptions struct {
	// Root is the Unity project or source tree against which relative paths and
	// glob patterns are resolved.
	Root string
	// Output is the destination .unitypackage filename.
	Output string
	// PathPrefix is prepended to every pathname stored in the archive.
	PathPrefix string
	// Paths lists explicit asset or .meta paths relative to Root.
	Paths []string
	// Includes and Excludes are slash-separated glob patterns. ** matches zero
	// or more path segments.
	Includes []string
	Excludes []string
}

// ExtractOptions configures transactional extraction of a unitypackage.
type ExtractOptions struct {
	// Input is the source .unitypackage filename.
	Input string
	// Root is the destination Unity project root.
	Root string
	// Overwrite permits replacement of existing asset and .meta files.
	Overwrite bool
	// Limits bounds archive entries and expanded payload bytes. Zero values use
	// safe defaults.
	Limits ReadLimits
}
