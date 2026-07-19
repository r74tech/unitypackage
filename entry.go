package unitypackage

// Entry describes one asset or folder record in a unitypackage archive.
// It intentionally contains no staged payload or metadata implementation
// details and is safe to compare or use as a map key.
type Entry struct {
	GUID     string `json:"guid"`
	Path     string `json:"path"`
	HasAsset bool   `json:"hasAsset"`
	IsFolder bool   `json:"isFolder"`
}
