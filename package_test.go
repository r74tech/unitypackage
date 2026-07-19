package unitypackage_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	unitypackage "github.com/r74tech/unitypackage"
)

const (
	packageGUID = "11111111111111111111111111111111"
	fileGUID    = "22222222222222222222222222222222"
	emptyGUID   = "33333333333333333333333333333333"
)

func TestPackInspectExtractAndDeterminism(t *testing.T) {
	project := makeProject(t)
	first := filepath.Join(t.TempDir(), "first.unitypackage")
	second := filepath.Join(t.TempDir(), "second.unitypackage")
	options := unitypackage.PackOptions{Root: project, Paths: []string{"Assets/Package/Test.txt"}}
	options.Output = first
	if err := unitypackage.Pack(options); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	options.Output = second
	if err := unitypackage.Pack(options); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("packages are not deterministic")
	}

	entries, err := unitypackage.Inspect(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "Assets/Package" || entries[1].Path != "Assets/Package/Test.txt" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	destination := t.TempDir()
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: first, Root: destination}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "Assets", "Package", "Test.txt"))
	if err != nil || string(content) != "hello\n" {
		t.Fatalf("unexpected extracted asset: %q, %v", content, err)
	}
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: first, Root: destination}); err == nil {
		t.Fatal("expected overwrite protection")
	}
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: first, Root: destination, Overwrite: true}); err != nil {
		t.Fatal(err)
	}
}

func TestPackReplacesAnExistingOutput(t *testing.T) {
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "replace.unitypackage")
	options := unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package/Test.txt"}}
	if err := unitypackage.Pack(options); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "Assets", "Package", "Test.txt"), "updated\n")
	if err := unitypackage.Pack(options); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: output, Root: destination}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "Assets", "Package", "Test.txt"))
	if err != nil || string(content) != "updated\n" {
		t.Fatalf("output was not replaced: %q, %v", content, err)
	}
}

func TestPackAndExtractCreatePrivatePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not implement Unix permission bits")
	}
	project := makeProject(t)
	outputDirectory := filepath.Join(t.TempDir(), "output")
	output := filepath.Join(outputDirectory, "package.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{
		Root: project, Output: output, Paths: []string{"Assets/Package/Test.txt"},
	}); err != nil {
		t.Fatal(err)
	}
	assertPrivatePath(t, outputDirectory)
	assertPrivatePath(t, output)

	destination := filepath.Join(t.TempDir(), "project")
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: output, Root: destination}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		destination,
		filepath.Join(destination, "Assets"),
		filepath.Join(destination, "Assets", "Package"),
		filepath.Join(destination, "Assets", "Package.meta"),
		filepath.Join(destination, "Assets", "Package", "Test.txt"),
		filepath.Join(destination, "Assets", "Package", "Test.txt.meta"),
	} {
		assertPrivatePath(t, path)
	}
}

func TestPackDirectoryIncludesDirectoryMetadata(t *testing.T) {
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "directory.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].IsFolder {
		t.Fatalf("unexpected directory package: %#v", entries)
	}
}

func TestPackTreatsAssetsAsAContainerWithoutInventingAssetsMeta(t *testing.T) {
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "assets.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "Assets/Package" {
		t.Fatalf("Assets container produced unexpected records: %#v", entries)
	}
}

func TestPackRejectsNestedDirectoryWithoutMeta(t *testing.T) {
	project := makeProject(t)
	if err := os.Mkdir(filepath.Join(project, "Assets", "Package", "MissingMeta"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "Assets", "Package", "MissingMeta", "Asset.txt"), "asset")
	writeFile(t, filepath.Join(project, "Assets", "Package", "MissingMeta", "Asset.txt.meta"), "guid: "+emptyGUID+"\n")
	err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: filepath.Join(t.TempDir(), "bad.unitypackage"), Paths: []string{"Assets/Package"}})
	if !errors.Is(err, unitypackage.ErrMissingMeta) {
		t.Fatalf("expected missing metadata error, got %v", err)
	}
}

func TestPackDirectoryIncludesEmptyDirectoryMetadata(t *testing.T) {
	project := makeProject(t)
	if err := os.Mkdir(filepath.Join(project, "Assets", "Package", "Empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "Assets", "Package", "Empty.meta"), "guid: "+emptyGUID+"\nfolderAsset: yes\n")
	output := filepath.Join(t.TempDir(), "directory.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[1].Path != "Assets/Package/Empty" || !entries[1].IsFolder {
		t.Fatalf("empty directory was not packaged: %#v", entries)
	}
}

func TestExtractPreflightsAllDestinations(t *testing.T) {
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "package.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package/Test.txt"}}); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := os.MkdirAll(filepath.Join(destination, "Assets", "Package"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(destination, "Assets", "Package", "Test.txt"), "keep")
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: output, Root: destination}); !errors.Is(err, unitypackage.ErrConflict) {
		t.Fatalf("expected destination conflict, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "Assets", "Package.meta")); !os.IsNotExist(err) {
		t.Fatal("failed extraction wrote files before preflight completed")
	}
}

func TestExtractRejectsSymlinkDestinations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires extra privileges on Windows")
	}
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "package.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package/Test.txt"}}); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(destination, "Assets")); err != nil {
		t.Fatal(err)
	}
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: output, Root: destination, Overwrite: true}); err == nil {
		t.Fatal("expected symlink traversal rejection")
	}
}

func TestPackRejectsDuplicateGUID(t *testing.T) {
	project := makeProject(t)
	writeFile(t, filepath.Join(project, "Assets", "Package", "Other.txt"), "other")
	writeFile(t, filepath.Join(project, "Assets", "Package", "Other.txt.meta"), "guid: "+fileGUID+"\n")
	err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: filepath.Join(t.TempDir(), "out.unitypackage"), Paths: []string{"Assets/Package/Test.txt", "Assets/Package/Other.txt"}})
	if err == nil {
		t.Fatal("expected duplicate GUID error")
	}
}

func TestPackRejectsSymlinkMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires extra privileges on Windows")
	}
	project := makeProject(t)
	metaPath := filepath.Join(project, "Assets", "Package", "Test.txt.meta")
	outsideMeta := filepath.Join(t.TempDir(), "outside.meta")
	writeFile(t, outsideMeta, "guid: "+fileGUID+"\n")
	if err := os.Remove(metaPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideMeta, metaPath); err != nil {
		t.Fatal(err)
	}
	err := unitypackage.Pack(unitypackage.PackOptions{
		Root: project, Output: filepath.Join(t.TempDir(), "out.unitypackage"),
		Paths: []string{"Assets/Package/Test.txt"},
	})
	if err == nil {
		t.Fatal("expected symlink metadata to be rejected")
	}
}

func TestPackRejectsCaseConflictingSourcePaths(t *testing.T) {
	project := makeProject(t)
	upperPath := filepath.Join(project, "Assets", "Package", "Case.txt")
	lowerPath := filepath.Join(project, "Assets", "Package", "case.txt")
	writeFile(t, upperPath, "upper")
	writeFile(t, upperPath+".meta", "guid: "+emptyGUID+"\n")
	lower, err := os.OpenFile(lowerPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		t.Skip("test filesystem is case-insensitive")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lower.WriteString("lower"); err != nil {
		_ = lower.Close()
		t.Fatal(err)
	}
	if err := lower.Close(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, lowerPath+".meta", "guid: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")

	err = unitypackage.Pack(unitypackage.PackOptions{
		Root: project, Output: filepath.Join(t.TempDir(), "out.unitypackage"),
		Paths: []string{"Assets/Package/Case.txt", "Assets/Package/case.txt"},
	})
	if err == nil {
		t.Fatal("expected case-conflicting source paths to be rejected")
	}
}

func TestInspectRejectsTraversalPathname(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "bad.unitypackage")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	write := func(name, value string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(value))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	write(fileGUID+"/asset.meta", "guid: "+fileGUID+"\n")
	write(fileGUID+"/asset", "bad")
	write(fileGUID+"/pathname", "../outside")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := unitypackage.Inspect(filename); !errors.Is(err, unitypackage.ErrUnsafePath) {
		t.Fatalf("expected unsafe pathname error, got %v", err)
	}
}

func TestInspectRejectsNonPortablePathnames(t *testing.T) {
	for _, pathname := range []string{"C:/outside.txt", "Assets/CON.txt", "Assets/trailing."} {
		t.Run(pathname, func(t *testing.T) {
			filename := writeFixturePackage(t, []fixtureRecord{{guid: fileGUID, path: pathname}})
			if _, err := unitypackage.Inspect(filename); err == nil {
				t.Fatalf("expected non-portable pathname %q to be rejected", pathname)
			}
		})
	}
}

func TestInspectStreamsAssetsWithoutCreatingATemporaryTree(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{{guid: fileGUID, path: "Assets/File.txt"}})
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))
	entries, err := unitypackage.Inspect(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "Assets/File.txt" {
		t.Fatalf("unexpected streamed inspection: %#v", entries)
	}
}

func TestInspectRejectsCorruptedGzipTrailer(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{{guid: fileGUID, path: "Assets/File.txt"}})
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := unitypackage.Inspect(filename); err == nil {
		t.Fatal("expected corrupt gzip trailer to be rejected")
	}
}

func TestInspectHonorsArchiveLimits(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{{guid: fileGUID, path: "Assets/File.txt"}})
	tests := map[string]unitypackage.ReadLimits{
		"entry count":       {MaxEntries: 2},
		"uncompressed size": {MaxUncompressedSize: 1},
	}
	for name, limits := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := unitypackage.InspectWithLimits(filename, limits); err == nil {
				t.Fatal("InspectWithLimits accepted an archive beyond its limits")
			}
		})
	}
}

func TestInspectCountsDataAfterTarEndAgainstExpansionLimit(t *testing.T) {
	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	limit := tarData.Len()
	tarData.Write(make([]byte, 1024))

	filename := filepath.Join(t.TempDir(), "trailing.unitypackage")
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(output)
	if _, err := gz.Write(tarData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := unitypackage.InspectWithLimits(filename, unitypackage.ReadLimits{
		MaxUncompressedSize: uint64(limit),
	}); err == nil {
		t.Fatal("expected trailing expanded data to count against the archive limit")
	}
}

func TestInspectRejectsCaseConflictingPathnames(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{
		{guid: fileGUID, path: "Assets/Foo.txt"},
		{guid: emptyGUID, path: "assets/foo.txt"},
	})
	if _, err := unitypackage.Inspect(filename); err == nil {
		t.Fatal("expected case-conflicting pathnames to be rejected")
	}
}

func TestInspectRejectsAssetMetaPathCollision(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{
		{guid: fileGUID, path: "Assets/Foo"},
		{guid: emptyGUID, path: "Assets/Foo.meta"},
	})
	if _, err := unitypackage.Inspect(filename); err == nil {
		t.Fatal("expected asset/.meta destination collision to be rejected")
	}
}

func TestInspectRejectsFileAncestor(t *testing.T) {
	filename := writeFixturePackage(t, []fixtureRecord{
		{guid: fileGUID, path: "Assets/Foo"},
		{guid: emptyGUID, path: "Assets/Foo/Bar.txt"},
	})
	if _, err := unitypackage.Inspect(filename); err == nil {
		t.Fatal("expected file ancestor to be rejected")
	}
}

func TestExtractRejectsTopologyConflictBeforeWriting(t *testing.T) {
	project := makeProject(t)
	output := filepath.Join(t.TempDir(), "package.unitypackage")
	if err := unitypackage.Pack(unitypackage.PackOptions{Root: project, Output: output, Paths: []string{"Assets/Package"}}); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := os.Mkdir(filepath.Join(destination, "Assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(destination, "Assets", "Package"), "keep")
	if err := unitypackage.Extract(unitypackage.ExtractOptions{Input: output, Root: destination, Overwrite: true}); err == nil {
		t.Fatal("expected file/folder topology conflict")
	}
	content, err := os.ReadFile(filepath.Join(destination, "Assets", "Package"))
	if err != nil || string(content) != "keep" {
		t.Fatalf("preflight changed existing file: %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "Assets", "Package.meta")); !os.IsNotExist(err) {
		t.Fatal("preflight failure wrote package metadata")
	}
}

type fixtureRecord struct {
	guid   string
	path   string
	folder bool
}

func writeFixturePackage(t *testing.T, records []fixtureRecord) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "fixture.unitypackage")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	write := func(name, value string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(value))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range records {
		meta := "guid: " + record.guid + "\n"
		if record.folder {
			meta += "folderAsset: yes\n"
		} else {
			write(record.guid+"/asset", "asset")
		}
		write(record.guid+"/asset.meta", meta)
		write(record.guid+"/pathname", record.path)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func makeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Assets", "Package"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "Assets", "Package.meta"), "guid: "+packageGUID+"\nfolderAsset: yes\n")
	writeFile(t, filepath.Join(root, "Assets", "Package", "Test.txt"), "hello\n")
	writeFile(t, filepath.Join(root, "Assets", "Package", "Test.txt.meta"), "guid: "+fileGUID+"\nTextImporter:\n  value:\n    : invalid-yaml\n")
	return root
}

func writeFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPrivatePath(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permission := info.Mode().Perm(); permission&0o077 != 0 {
		t.Fatalf("%s permissions = %04o, group/other permissions must be zero", path, permission)
	}
}
