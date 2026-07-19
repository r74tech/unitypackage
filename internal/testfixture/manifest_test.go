package testfixture

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
	filename := filepath.Join(t.TempDir(), "manifest.json")
	writePrivateFile(t, filename, fmt.Sprintf(`{
  "artifacts": [{
    "id": "fixture",
    "name": "Fixture",
    "publisher": "Publisher",
    "source": "https://example.test/source",
    "url": "https://example.test/fixture.zip",
    "filename": "fixture.zip",
    "kind": "zip",
    "size": 7,
    "sha256": %q,
    "expected": {"records": 1}
  }]
}`, hash))

	manifest, err := LoadManifest(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Expected["records"] != 1 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestLoadManifestRejectsInvalidInput(t *testing.T) {
	hash := strings.Repeat("0", 64)
	tests := map[string]string{
		"unknown field":   fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":"https://example.test/a","filename":"a","kind":"file","size":1,"sha256":%q,"extra":true}]}`, hash),
		"duplicate id":    fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":"https://example.test/a","filename":"a","kind":"file","size":1,"sha256":%q},{"id":"fixture","url":"https://example.test/b","filename":"b","kind":"file","size":1,"sha256":%q}]}`, hash, hash),
		"unsafe filename": fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":"https://example.test/a","filename":"../a","kind":"file","size":1,"sha256":%q}]}`, hash),
		"invalid digest":  `{"artifacts":[{"id":"fixture","url":"https://example.test/a","filename":"a","kind":"file","size":1,"sha256":"nope"}]}`,
		"insecure URL":    fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":"http://example.test/a","filename":"a","kind":"file","size":1,"sha256":%q}]}`, hash),
		"trailing JSON":   fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":"https://example.test/a","filename":"a","kind":"file","size":1,"sha256":%q}]} {}`, hash),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "manifest.json")
			writePrivateFile(t, filename, input)
			if _, err := LoadManifest(filename); err == nil {
				t.Fatal("LoadManifest accepted invalid input")
			}
		})
	}
}

func writePrivateFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
