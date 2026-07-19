package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r74tech/unitypackage/internal/testfixture"
)

func TestRunFetchesManifestAndWritesIndex(t *testing.T) {
	payload := []byte("fixture")
	client := &http.Client{Transport: commandRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(string(payload))),
			ContentLength: int64(len(payload)),
			Request:       request,
		}, nil
	})}

	digest := sha256.Sum256(payload)
	manifest := fmt.Sprintf(`{"artifacts":[{"id":"fixture","url":%q,"filename":"fixture.bin","kind":"file","size":%d,"sha256":%q}]}`,
		"https://example.test/fixture.bin", len(payload), fmt.Sprintf("%x", digest))
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "cache")
	if err := runWithFetcher(context.Background(), []string{"-manifest", manifestPath, "-dir", cache}, testfixture.Fetcher{Client: client, Retries: 1}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cache, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"fixture"`) {
		t.Fatalf("unexpected index: %s", data)
	}
}

type commandRoundTripFunc func(*http.Request) (*http.Response, error)

func (function commandRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRunRequiresDestination(t *testing.T) {
	if err := run(context.Background(), nil); err == nil {
		t.Fatal("run accepted a missing -dir")
	}
}

func TestRunRejectsInvalidZIPLimits(t *testing.T) {
	tests := [][]string{
		{"-dir", t.TempDir(), "-max-zip-entries", "0"},
		{"-dir", t.TempDir(), "-max-zip-uncompressed-size", "0"},
	}
	for _, arguments := range tests {
		if err := run(context.Background(), arguments); err == nil {
			t.Fatalf("run accepted invalid arguments: %v", arguments)
		}
	}
}
