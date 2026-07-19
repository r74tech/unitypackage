package upm_test

import (
	"os"
	"path/filepath"
	"testing"

	unitypackage "github.com/r74tech/unitypackage"
	"github.com/r74tech/unitypackage/upm"
)

func TestPackUsesManifestNameAsEmbeddedPackageDestination(t *testing.T) {
	root := makePackage(t)
	output := filepath.Join(t.TempDir(), "package.unitypackage")
	if err := upm.Pack(upm.PackOptions{Root: root, Output: output}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"Packages/com.example.tool/Runtime":          true,
		"Packages/com.example.tool/Runtime/Thing.cs": true,
		"Packages/com.example.tool/package.json":     true,
	}
	if len(entries) != len(want) {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	for _, entry := range entries {
		if !want[entry.Path] {
			t.Errorf("unexpected package path %q", entry.Path)
		}
	}
}

func TestPackCanRemapUnderAssets(t *testing.T) {
	root := makePackage(t)
	output := filepath.Join(t.TempDir(), "package.unitypackage")
	if err := upm.Pack(upm.PackOptions{Root: root, Output: output, Destination: "Assets/Example"}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !stringsHasPrefix(entry.Path, "Assets/Example/") {
			t.Errorf("entry was not remapped: %q", entry.Path)
		}
	}
}

func TestPackDoesNotSilentlyDropAssetsWithoutMeta(t *testing.T) {
	root := makePackage(t)
	if err := os.WriteFile(filepath.Join(root, "Untracked.txt"), []byte("missing meta"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := upm.Pack(upm.PackOptions{Root: root, Output: filepath.Join(t.TempDir(), "package.unitypackage")})
	if err == nil {
		t.Fatal("expected missing .meta error")
	}
}

func TestReadManifestIncludesVPMDependencies(t *testing.T) {
	manifest, err := upm.ReadManifest(makePackage(t))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "com.example.tool" || manifest.Version != "1.2.3" || manifest.VPMDependencies["com.vrchat.base"] != ">=3.8.0" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func makePackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "package.json"), `{
  "name": "com.example.tool",
  "displayName": "Example Tool",
  "version": "1.2.3",
  "vpmDependencies": {"com.vrchat.base": ">=3.8.0"}
}`)
	writeFile(t, filepath.Join(root, "package.json.meta"), "guid: 44444444444444444444444444444444\n")
	writeFile(t, filepath.Join(root, "Runtime.meta"), "guid: 55555555555555555555555555555555\nfolderAsset: yes\n")
	writeFile(t, filepath.Join(root, "Runtime", "Thing.cs"), "public class Thing {}\n")
	writeFile(t, filepath.Join(root, "Runtime", "Thing.cs.meta"), "guid: 66666666666666666666666666666666\n")
	return root
}

func writeFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func stringsHasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
