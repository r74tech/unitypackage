package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileRejectsCaseConflictingPaths(t *testing.T) {
	asset := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(asset, []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := map[string]Entry{
		"11111111111111111111111111111111": {
			GUID: "11111111111111111111111111111111", Path: "Assets/File.txt",
			HasAsset: true, Meta: []byte("guid: 11111111111111111111111111111111\n"), AssetPath: asset,
		},
		"22222222222222222222222222222222": {
			GUID: "22222222222222222222222222222222", Path: "assets/file.txt",
			HasAsset: true, Meta: []byte("guid: 22222222222222222222222222222222\n"), AssetPath: asset,
		},
	}
	if err := WriteFile(filepath.Join(t.TempDir(), "out.unitypackage"), entries); err == nil {
		t.Fatal("expected case-conflicting paths to be rejected")
	}
}
