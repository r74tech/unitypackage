package unitypackage

import (
	"fmt"
	"path/filepath"
	"strings"
)

func ensureInside(root, pathname string) error {
	relative, err := filepath.Rel(root, pathname)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%w: path %q is outside root %q", ErrUnsafePath, pathname, root)
	}
	return nil
}
