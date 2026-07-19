package archive

import (
	"fmt"
	"regexp"
	"strings"
)

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
var metaGUIDPattern = regexp.MustCompile(`(?m)^[\t ]*guid:[\t ]*([0-9a-fA-F]{32})[\t ]*\r?$`)
var folderAssetPattern = regexp.MustCompile(`(?mi)^[\t ]*folderAsset:[\t ]*yes[\t ]*\r?$`)

type Meta struct {
	GUID     string
	IsFolder bool
}

// ParseMeta extracts only the stable fields required by the archive format.
// Unity metadata is YAML-like but is not reliably valid general YAML.
func ParseMeta(data []byte) (Meta, error) {
	matches := metaGUIDPattern.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return Meta{}, fmt.Errorf("meta has no valid 32-character GUID")
	}
	if len(matches) > 1 {
		return Meta{}, fmt.Errorf("meta has multiple GUID fields")
	}
	return Meta{
		GUID:     strings.ToLower(string(matches[0][1])),
		IsFolder: folderAssetPattern.Match(data),
	}, nil
}
