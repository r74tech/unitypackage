package archive

import (
	"fmt"
	"path"
	"strings"
)

// ValidatePathname validates and normalizes a slash-separated pathname stored
// in a unitypackage.
func ValidatePathname(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	for _, segment := range strings.Split(clean, "/") {
		if !isPortablePathSegment(segment) {
			return "", fmt.Errorf("%w: pathname is not portable across Unity platforms: %q", ErrUnsafePath, name)
		}
	}
	return clean, nil
}

func isPortablePathSegment(segment string) bool {
	if segment == "" || strings.ContainsAny(segment, `<>:"|?*`) ||
		strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return false
	}
	for _, character := range segment {
		if character < 0x20 {
			return false
		}
	}
	base := strings.ToUpper(segment)
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}
