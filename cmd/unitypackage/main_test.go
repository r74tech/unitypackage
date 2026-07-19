package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	unitypackage "github.com/r74tech/unitypackage"
)

func TestRunPackUPM(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, filepath.Join(root, "package.json"), `{"name":"com.example.cli","version":"1.0.0"}`)
	writeCLIFile(t, filepath.Join(root, "package.json.meta"), "guid: 77777777777777777777777777777777\n")
	output := filepath.Join(t.TempDir(), "cli.unitypackage")
	if err := run([]string{
		"pack-upm", "--root", root, "--destination", "Assets/CLI", "--output", output,
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := unitypackage.Inspect(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "Assets/CLI/package.json" {
		t.Fatalf("unexpected CLI output: %#v", entries)
	}
}

func TestRunPackUPMRejectsPositionalAssets(t *testing.T) {
	err := run([]string{"pack-upm", "--output", "out.unitypackage", "unexpected"})
	if err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestCLIProjectPackListVerifyAndExtract(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "Assets", "Tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool.meta"), "guid: 88888888888888888888888888888888\nfolderAsset: yes\n")
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool", "File.txt"), "hello\n")
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool", "File.txt.meta"), "guid: 99999999999999999999999999999999\n")
	filesFrom := filepath.Join(t.TempDir(), "files.txt")
	writeCLIFile(t, filesFrom, "# generated list\n\nAssets/Tool/File.txt.meta\n")
	output := filepath.Join(t.TempDir(), "project.unitypackage")
	if err := run([]string{"pack", "--root", project, "--files-from", filesFrom, "--output", output}); err != nil {
		t.Fatal(err)
	}
	text := captureStdout(t, func() error { return run([]string{"list", output}) })
	if text == "" {
		t.Fatal("plain list produced no output")
	}
	jsonOutput := captureStdout(t, func() error { return run([]string{"list", "--json", output}) })
	if len(jsonOutput) == 0 || jsonOutput[0] != '[' {
		t.Fatalf("unexpected JSON list output: %q", jsonOutput)
	}
	if output := captureStdout(t, func() error { return run([]string{"verify", output}) }); output == "" {
		t.Fatal("verify produced no output")
	}
	destination := t.TempDir()
	if err := run([]string{"extract", "--root", destination, output}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "Assets", "Tool", "File.txt"))
	if err != nil || string(data) != "hello\n" {
		t.Fatalf("unexpected extracted file: %q, %v", data, err)
	}
}

func TestRunCommandErrorsAndVersion(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("expected missing command error")
	}
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown command error")
	}
	if output := captureStdout(t, func() error { return run([]string{"version"}) }); output == "" {
		t.Fatal("version produced no output")
	}
}

func TestCLIReadLimits(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "Assets", "Tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool.meta"), "guid: 88888888888888888888888888888888\nfolderAsset: yes\n")
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool", "File.txt"), "hello\n")
	writeCLIFile(t, filepath.Join(project, "Assets", "Tool", "File.txt.meta"), "guid: 99999999999999999999999999999999\n")
	output := filepath.Join(t.TempDir(), "project.unitypackage")
	if err := run([]string{"pack", "--root", project, "--output", output, "Assets/Tool"}); err != nil {
		t.Fatal(err)
	}

	tests := map[string][]string{
		"list":    {"list", "--max-archive-uncompressed-size", "1", output},
		"verify":  {"verify", "--max-archive-uncompressed-size", "1", output},
		"extract": {"extract", "--max-archive-uncompressed-size", "1", "--root", t.TempDir(), output},
	}
	for name, arguments := range tests {
		t.Run(name, func(t *testing.T) {
			if err := run(arguments); !errors.Is(err, unitypackage.ErrInvalidArchive) {
				t.Fatalf("expected archive limit error, got %v", err)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	temporary, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = temporary
	defer func() { os.Stdout = previous }()
	if err := fn(); err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(temporary.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeCLIFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
