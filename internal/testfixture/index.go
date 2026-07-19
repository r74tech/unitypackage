package testfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Index is written into the fixture cache for interoperability tests.
type Index struct {
	Fixtures map[string]string `json:"fixtures"`
}

// WriteIndex atomically writes the resolved fixture paths beneath root.
func WriteIndex(root string, results []Result) error {
	index := Index{Fixtures: make(map[string]string, len(results))}
	for _, result := range results {
		index.Fixtures[result.ID] = result.Path
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := ensureCacheRoot(root); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(root, ".index-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := publishPath(temporaryName, filepath.Join(root, "index.json")); err != nil {
		return fmt.Errorf("publish fixture index: %w", err)
	}
	return nil
}
