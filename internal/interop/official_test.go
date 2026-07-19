//go:build interop

// Package interop_test verifies interoperability with immutable artifacts
// published by Unity Technologies and VRChat.
package interop_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	unitypackage "github.com/r74tech/unitypackage"
	"github.com/r74tech/unitypackage/internal/testfixture"
	"github.com/r74tech/unitypackage/upm"
)

const fixtureRootEnvironment = "UNITYPACKAGE_TESTDATA_DIR"

func TestUnityOfficialPackage(t *testing.T) {
	fixture, artifact := requireFixture(t, "unity-postprocessing-1.0.4")
	entries, err := unitypackage.Inspect(fixture)
	if err != nil {
		t.Fatal(err)
	}
	folders, files := 0, 0
	foundRuntime := false
	for _, entry := range entries {
		if entry.IsFolder {
			folders++
		} else {
			files++
		}
		if entry.GUID == "ff26db721962cdf4a8edcdfa9a767d2a" && entry.Path == "Assets/PostProcessing/Runtime/PostProcessingBehaviour.cs" {
			foundRuntime = true
		}
	}
	if folders != artifact.Expected["folders"] || files != artifact.Expected["files"] || !foundRuntime {
		t.Fatalf("unexpected Unity official package: folders=%d files=%d runtime=%v", folders, files, foundRuntime)
	}
}

func TestUnityOfficialPackageRoundTrip(t *testing.T) {
	fixture, artifact := requireFixture(t, "unity-postprocessing-1.0.4")
	project := t.TempDir()
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: fixture, Root: project}); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "roundtrip.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{
		Root: project, Output: output, Paths: []string{"Assets/PostProcessing"},
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != artifact.Expected["records"] {
		t.Fatalf("official package round-trip lost records: got %d, want %d", len(entries), artifact.Expected["records"])
	}
}

func TestVRChatOfficialVPMPackageConversion(t *testing.T) {
	fixture, artifact := requireFixture(t, "vrchat-vpm-resolver-0.1.9")
	manifest, err := upm.ReadManifest(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "com.vrchat.core.vpm-resolver" || manifest.Version != "0.1.9" {
		t.Fatalf("unexpected VRChat manifest: %#v", manifest)
	}
	output := filepath.Join(t.TempDir(), "vrchat.unitypackage")
	if err := upm.Pack(upm.PackOptions{Root: fixture, Output: output}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != artifact.Expected["convertedRecords"] {
		t.Fatalf("unexpected converted entry count: got %d, want %d", len(entries), artifact.Expected["convertedRecords"])
	}
}

func requireFixture(t *testing.T, id string) (string, testfixture.Artifact) {
	t.Helper()
	root := os.Getenv(fixtureRootEnvironment)
	if root == "" {
		t.Fatalf("%s is required for interoperability tests; run go run ./internal/cmd/fetchtestdata -dir <cache>", fixtureRootEnvironment)
	}
	manifest, err := testfixture.LoadManifest(filepath.Join(repositoryRoot(t), "testdata", "official", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		t.Fatalf("read fixture index: %v", err)
	}
	var index testfixture.Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("decode fixture index: %v", err)
	}
	fixture := index.Fixtures[id]
	if fixture == "" {
		t.Fatalf("fixture %q is absent from index", id)
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.ID == id {
			return fixture, artifact
		}
	}
	t.Fatalf("fixture %q is absent from manifest", id)
	return "", testfixture.Artifact{}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(fmt.Errorf("locate repository root: %w", err))
	}
	return root
}
