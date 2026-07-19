package archive

import "os"

// Entry is the internal representation used while reading and writing an
// archive. It deliberately contains data that must not leak into the public
// unitypackage.Entry type.
type Entry struct {
	GUID      string
	Path      string
	HasAsset  bool
	IsFolder  bool
	Meta      []byte
	AssetPath string
}

// Package owns any temporary files produced while opening an archive.
type Package struct {
	Entries []Entry
	staging string
}

// Close removes temporary asset data staged by Open.
func (p *Package) Close() error {
	if p == nil || p.staging == "" {
		return nil
	}
	err := os.RemoveAll(p.staging)
	p.staging = ""
	return err
}
