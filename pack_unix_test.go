//go:build darwin || linux

package unitypackage_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	unitypackage "github.com/r74tech/unitypackage"
)

func TestPackRejectsNamedPipeAsset(t *testing.T) {
	project := makeProject(t)
	assetPath := filepath.Join(project, "Assets", "Package", "Test.txt")
	if err := os.Remove(assetPath); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(assetPath, 0o600); err != nil {
		t.Fatal(err)
	}
	holdOpen, err := os.OpenFile(assetPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer holdOpen.Close()

	err = unitypackage.Pack(unitypackage.PackOptions{
		Root: project, Output: filepath.Join(t.TempDir(), "out.unitypackage"),
		Paths: []string{"Assets/Package/Test.txt"},
	})
	if err == nil {
		t.Fatal("expected named pipe asset to be rejected")
	}
}
