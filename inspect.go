package unitypackage

import "github.com/r74tech/unitypackage/internal/archive"

// Inspect validates filename and returns its normalized asset records. Asset
// payloads are streamed and are not expanded into a temporary directory.
func Inspect(filename string) ([]Entry, error) {
	return InspectWithLimits(filename, ReadLimits{})
}

// InspectWithLimits validates filename using limits and returns its normalized
// asset records.
func InspectWithLimits(filename string, limits ReadLimits) ([]Entry, error) {
	packageFile, err := archive.Open(filename, false, limits.archiveLimits())
	if err != nil {
		return nil, err
	}
	defer packageFile.Close()
	entries := make([]Entry, len(packageFile.Entries))
	for index, entry := range packageFile.Entries {
		entries[index] = Entry{
			GUID: entry.GUID, Path: entry.Path,
			HasAsset: entry.HasAsset, IsFolder: entry.IsFolder,
		}
	}
	return entries, nil
}

// Verify validates filename without returning its records.
func Verify(filename string) error {
	_, err := Inspect(filename)
	return err
}

// VerifyWithLimits validates filename using limits without returning records.
func VerifyWithLimits(filename string, limits ReadLimits) error {
	_, err := InspectWithLimits(filename, limits)
	return err
}
