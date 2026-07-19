package archive

import "errors"

var (
	ErrInvalidArchive = errors.New("invalid unitypackage archive")
	ErrUnsafePath     = errors.New("unsafe unitypackage path")
)
