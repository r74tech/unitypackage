package testfixture

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFetcherDownloadsVerifiesAndReusesBlob(t *testing.T) {
	payload := []byte("official fixture")
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("User-Agent") != "fixture-test/1" {
			t.Errorf("unexpected User-Agent %q", request.Header.Get("User-Agent"))
		}
		return responseWith(request, http.StatusOK, payload), nil
	})}

	artifact := artifactFor("fixture", "fixture.bin", "file", "https://example.test/fixture.bin", payload)
	fetcher := Fetcher{Client: client, UserAgent: "fixture-test/1", Retries: 1}
	cache := t.TempDir()
	for run := 0; run < 2; run++ {
		results, err := fetcher.FetchAll(context.Background(), Manifest{Artifacts: []Artifact{artifact}}, cache)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Fatalf("unexpected results: %#v", results)
		}
		got, err := os.ReadFile(results[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("downloaded data = %q", got)
		}
		assertPrivatePermissions(t, results[0].Path)
	}
	if requests != 1 {
		t.Fatalf("server requests = %d, want 1", requests)
	}
}

func TestFetcherRejectsWrongDigest(t *testing.T) {
	payload := []byte("fixture")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return responseWith(request, http.StatusOK, payload), nil
	})}
	artifact := artifactFor("fixture", "fixture.bin", "file", "https://example.test/fixture.bin", payload)
	artifact.SHA256 = strings.Repeat("0", 64)
	_, err := (Fetcher{Client: client, Retries: 1}).FetchAll(
		context.Background(), Manifest{Artifacts: []Artifact{artifact}}, t.TempDir(),
	)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetcherExtractsSafeZIP(t *testing.T) {
	payload := makeZIP(t, zipEntry{name: "Package/package.json", content: "{}"})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return responseWith(request, http.StatusOK, payload), nil
	})}
	artifact := artifactFor("fixture", "fixture.zip", "zip", "https://example.test/fixture.zip", payload)
	cache := t.TempDir()
	results, err := (Fetcher{Client: client, Retries: 1}).FetchAll(
		context.Background(), Manifest{Artifacts: []Artifact{artifact}}, cache,
	)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(results[0].Path, "Package", "package.json")
	if data, err := os.ReadFile(filename); err != nil || string(data) != "{}" {
		t.Fatalf("extracted file: data=%q err=%v", data, err)
	}
	assertPrivatePermissions(t, filename)

	// A marker written by an older extractor must not bypass newer checks.
	finalRoot := filepath.Dir(results[0].Path)
	if err := os.WriteFile(filepath.Join(finalRoot, "complete"), []byte(artifact.SHA256+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(results[0].Path, "stale")
	if err := os.WriteFile(stale, []byte("old extraction"), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err = (Fetcher{Client: client, Retries: 1}).FetchAll(
		context.Background(), Manifest{Artifacts: []Artifact{artifact}}, cache,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(results[0].Path, "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale extracted content survived cache-version replacement: %v", err)
	}
}

func TestExtractZIPRejectsUnsafeEntries(t *testing.T) {
	tests := map[string]zipEntry{
		"parent traversal": {name: "../escape", content: "bad"},
		"backslash":        {name: `..\\escape`, content: "bad"},
		"absolute":         {name: "/escape", content: "bad"},
		"Windows drive":    {name: "C:/escape", content: "bad"},
		"reserved name":    {name: "CON", content: "bad"},
		"non-canonical":    {name: "a/../escape", content: "bad"},
		"symlink":          {name: "link", content: "target", mode: os.ModeSymlink | 0o600},
	}
	for name, entry := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			payload := makeZIP(t, entry)
			blob := filepath.Join(root, "fixture.zip")
			if err := os.WriteFile(blob, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			artifact := artifactFor("fixture", "fixture.zip", "zip", "https://example.test/fixture.zip", payload)
			if _, err := extractZIP(root, artifact, blob, ZIPLimits{}); err == nil {
				t.Fatal("extractZIP accepted unsafe entry")
			}
			finalRoot := filepath.Join(root, "extracted", artifact.SHA256)
			if _, err := os.Lstat(finalRoot); !os.IsNotExist(err) {
				t.Fatalf("unsafe ZIP published output at %s: %v", finalRoot, err)
			}
			for _, candidate := range []string{
				filepath.Join(root, "escape"),
				filepath.Join(root, "link"),
				filepath.Join(filepath.Dir(root), "escape"),
			} {
				if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
					t.Fatalf("unsafe ZIP wrote %s: %v", candidate, err)
				}
			}
		})
	}
}

func TestExtractZIPRejectsConflictingEntries(t *testing.T) {
	tests := map[string][]zipEntry{
		"duplicate": {
			{name: "Package/file.txt", content: "first"},
			{name: "Package/file.txt", content: "second"},
		},
		"case conflict": {
			{name: "Package/File.txt", content: "first"},
			{name: "package/file.txt", content: "second"},
		},
		"file ancestor": {
			{name: "Package", content: "file"},
			{name: "Package/file.txt", content: "child"},
		},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			payload := makeZIP(t, entries...)
			blob := filepath.Join(root, "fixture.zip")
			if err := os.WriteFile(blob, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			artifact := artifactFor("fixture", "fixture.zip", "zip", "https://example.test/fixture.zip", payload)
			if _, err := extractZIP(root, artifact, blob, ZIPLimits{}); err == nil {
				t.Fatal("extractZIP accepted conflicting entries")
			}
			if _, err := os.Lstat(filepath.Join(root, "extracted", artifact.SHA256)); !os.IsNotExist(err) {
				t.Fatalf("conflicting ZIP published output: %v", err)
			}
		})
	}
}

func TestExtractZIPRejectsChecksumMismatch(t *testing.T) {
	payload := makeZIP(t, zipEntry{name: "Package/file.txt", content: "checksum payload"})
	contentOffset := bytes.Index(payload, []byte("checksum payload"))
	if contentOffset < 0 {
		t.Fatal("could not locate stored ZIP payload")
	}
	payload[contentOffset] ^= 0xff
	root := t.TempDir()
	blob := filepath.Join(root, "fixture.zip")
	if err := os.WriteFile(blob, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := artifactFor("fixture", "fixture.zip", "zip", "https://example.test/fixture.zip", payload)
	if _, err := extractZIP(root, artifact, blob, ZIPLimits{}); err == nil {
		t.Fatal("extractZIP accepted an entry with a bad CRC-32 checksum")
	}
}

func TestValidateZIPRejectsExpansionLimits(t *testing.T) {
	t.Run("entry count", func(t *testing.T) {
		files := make([]*zip.File, DefaultMaxZIPEntries+1)
		if err := validateZIP(files, t.TempDir(), ZIPLimits{}); err == nil {
			t.Fatal("validateZIP accepted too many entries")
		}
	})
	t.Run("uncompressed size", func(t *testing.T) {
		file := &zip.File{FileHeader: zip.FileHeader{
			Name: "bomb", UncompressedSize64: DefaultMaxZIPUncompressedSize + 1,
		}}
		if err := validateZIP([]*zip.File{file}, t.TempDir(), ZIPLimits{}); err == nil {
			t.Fatal("validateZIP accepted excessive expansion")
		}
	})
	t.Run("int64 overflow", func(t *testing.T) {
		file := &zip.File{FileHeader: zip.FileHeader{
			Name: "overflow", UncompressedSize64: 1 << 63,
		}}
		limits := ZIPLimits{MaxEntries: 1, MaxUncompressedSize: ^uint64(0)}
		if err := validateZIP([]*zip.File{file}, t.TempDir(), limits); err == nil {
			t.Fatal("validateZIP accepted a size that cannot be copied safely")
		}
	})
}

func TestFetcherHonorsCustomZIPLimits(t *testing.T) {
	payload := makeZIP(t, zipEntry{name: "file.txt", content: "too large"})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return responseWith(request, http.StatusOK, payload), nil
	})}
	artifact := artifactFor("fixture", "fixture.zip", "zip", "https://example.test/fixture.zip", payload)
	_, err := (Fetcher{
		Client: client, Retries: 1,
		ZIPLimits: ZIPLimits{MaxEntries: 1, MaxUncompressedSize: 1},
	}).FetchAll(context.Background(), Manifest{Artifacts: []Artifact{artifact}}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "expands beyond 1 bytes") {
		t.Fatalf("custom ZIP expansion limit was not enforced: %v", err)
	}
}

func TestWriteIndexUsesPrivatePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	if err := WriteIndex(root, []Result{{ID: "fixture", Path: "/fixture"}}); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "index.json")
	assertPrivatePermissions(t, root)
	assertPrivatePermissions(t, filename)
	if err := WriteIndex(root, []Result{{ID: "fixture", Path: "/new-fixture"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/new-fixture") {
		t.Fatalf("index was not replaced: %s", data)
	}
}

func TestVerifyBlobRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires extra privileges on Windows")
	}
	payload := []byte("fixture")
	artifact := artifactFor("fixture", "fixture.bin", "file", "https://example.test/fixture.bin", payload)
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "fixture.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := verifyBlob(link, artifact); err == nil {
		t.Fatal("verifyBlob accepted a symbolic link")
	}
}

func TestFetcherRejectsSymlinkedCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires extra privileges on Windows")
	}
	payload := []byte("fixture")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return responseWith(request, http.StatusOK, payload), nil
	})}
	artifact := artifactFor("fixture", "fixture.bin", "file", "https://example.test/fixture.bin", payload)
	cache := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cache, "blobs")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Fetcher{Client: client, Retries: 1}).FetchAll(
		context.Background(), Manifest{Artifacts: []Artifact{artifact}}, cache,
	); err == nil {
		t.Fatal("Fetcher accepted a symlinked cache directory")
	}
	outsideBlob := filepath.Join(outside, artifact.SHA256, artifact.Filename)
	if _, err := os.Lstat(outsideBlob); !os.IsNotExist(err) {
		t.Fatalf("Fetcher wrote outside its cache root: %v", err)
	}
}

type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
}

func makeZIP(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func artifactFor(id, filename, kind, url string, payload []byte) Artifact {
	hash := sha256.Sum256(payload)
	return Artifact{
		ID: id, URL: url, Filename: filename, Kind: kind,
		Size: int64(len(payload)), SHA256: fmt.Sprintf("%x", hash),
	}
}

func assertPrivatePermissions(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permission := info.Mode().Perm(); permission&0o077 != 0 {
		t.Fatalf("%s permissions = %04o, group/other permissions must be zero", path, permission)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func responseWith(request *http.Request, status int, payload []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
		Request:       request,
	}
}
