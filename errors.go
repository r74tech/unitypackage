package unitypackage

import (
	"errors"

	"github.com/r74tech/unitypackage/internal/archive"
)

var (
	// ErrInvalidArchive identifies malformed or internally inconsistent input.
	ErrInvalidArchive = archive.ErrInvalidArchive
	// ErrUnsafePath identifies an archive pathname that is unsafe or not
	// portable across supported Unity platforms.
	ErrUnsafePath = archive.ErrUnsafePath
	// ErrConflict identifies an existing destination or incompatible topology.
	ErrConflict = errors.New("unitypackage destination conflict")
	// ErrMissingMeta identifies source content without required Unity metadata.
	ErrMissingMeta = errors.New("missing Unity metadata")
	// ErrNoAssets identifies a pack request that selected no assets.
	ErrNoAssets = errors.New("no assets selected")
)
