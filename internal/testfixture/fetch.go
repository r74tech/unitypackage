package testfixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Fetcher downloads and materializes immutable fixture artifacts.
type Fetcher struct {
	Client    *http.Client
	UserAgent string
	Retries   int
	ZIPLimits ZIPLimits
}

// Result maps a manifest artifact to its usable local path.
type Result struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// FetchAll downloads, verifies, and materializes every manifest artifact.
func (fetcher Fetcher) FetchAll(ctx context.Context, manifest Manifest, root string) ([]Result, error) {
	if fetcher.Client == nil {
		fetcher.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if fetcher.UserAgent == "" {
		fetcher.UserAgent = "r74tech-unitypackage-testfixture/1"
	}
	if fetcher.Retries <= 0 {
		fetcher.Retries = 3
	}
	if err := ensureCacheRoot(root); err != nil {
		return nil, fmt.Errorf("prepare fixture cache: %w", err)
	}
	results := make([]Result, 0, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		blob, err := fetcher.fetchBlob(ctx, root, artifact)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", artifact.ID, err)
		}
		fixturePath := blob
		if artifact.Kind == "zip" {
			fixturePath, err = extractZIP(root, artifact, blob, fetcher.ZIPLimits.withDefaults())
			if err != nil {
				return nil, fmt.Errorf("extract %s: %w", artifact.ID, err)
			}
		}
		absolute, err := filepath.Abs(fixturePath)
		if err != nil {
			return nil, err
		}
		results = append(results, Result{ID: artifact.ID, Path: absolute})
	}
	return results, nil
}

func (fetcher Fetcher) fetchBlob(ctx context.Context, root string, artifact Artifact) (string, error) {
	blobsRoot := filepath.Join(root, "blobs")
	if err := ensureCacheDirectory(blobsRoot); err != nil {
		return "", err
	}
	directory := filepath.Join(blobsRoot, artifact.SHA256)
	if err := ensureCacheDirectory(directory); err != nil {
		return "", err
	}
	filename := filepath.Join(directory, artifact.Filename)
	if err := verifyBlob(filename, artifact); err == nil {
		return filename, nil
	}
	var lastErr error
	for attempt := 0; attempt < fetcher.Retries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
		if err := fetcher.downloadOnce(ctx, artifact, directory, filename); err == nil {
			return filename, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}

func (fetcher Fetcher) downloadOnce(ctx context.Context, artifact Artifact, directory, filename string) error {
	temporary, err := os.CreateTemp(directory, ".download-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	request.Header.Set("User-Agent", fetcher.UserAgent)
	response, err := fetcher.Client.Do(request)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || response.Request.URL.Scheme != "https" {
		_ = temporary.Close()
		return fmt.Errorf("download ended at a non-HTTPS URL")
	}
	if response.StatusCode != http.StatusOK {
		_ = temporary.Close()
		return fmt.Errorf("GET %s: %s", artifact.URL, response.Status)
	}
	if response.ContentLength >= 0 && response.ContentLength != artifact.Size {
		_ = temporary.Close()
		return fmt.Errorf("Content-Length mismatch: got %d, want %d", response.ContentLength, artifact.Size)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, artifact.Size+1))
	if copyErr != nil {
		_ = temporary.Close()
		return copyErr
	}
	if written != artifact.Size {
		_ = temporary.Close()
		return fmt.Errorf("size mismatch: got %d, want %d", written, artifact.Size)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != artifact.SHA256 {
		_ = temporary.Close()
		return fmt.Errorf("SHA-256 mismatch: got %s, want %s", got, artifact.SHA256)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if publishErr := publishPath(temporaryName, filename); publishErr != nil {
		return fmt.Errorf("publish verified blob: %w", publishErr)
	}
	return verifyBlob(filename, artifact)
}

func verifyBlob(filename string, artifact Artifact) error {
	linkInfo, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 || !linkInfo.Mode().IsRegular() {
		return fmt.Errorf("cached blob is not a regular file")
	}
	input, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if info.Size() != artifact.Size {
		return fmt.Errorf("size mismatch")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != artifact.SHA256 {
		return fmt.Errorf("SHA-256 mismatch")
	}
	return nil
}
