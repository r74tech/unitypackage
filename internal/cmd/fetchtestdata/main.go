package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/r74tech/unitypackage/internal/testfixture"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fetchtestdata: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	return runWithFetcher(ctx, args, testfixture.Fetcher{})
}

func runWithFetcher(ctx context.Context, args []string, fetcher testfixture.Fetcher) error {
	flags := flag.NewFlagSet("fetchtestdata", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "testdata/official/manifest.json", "fixture manifest")
	destination := flags.String("dir", "", "content-addressed fixture cache")
	maxZIPEntries := flags.Int("max-zip-entries", testfixture.DefaultMaxZIPEntries, "maximum ZIP entry count")
	maxZIPSize := flags.Uint64("max-zip-uncompressed-size", testfixture.DefaultMaxZIPUncompressedSize, "maximum total ZIP expansion in bytes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *destination == "" {
		return fmt.Errorf("-dir is required")
	}
	if *maxZIPEntries <= 0 || *maxZIPSize == 0 {
		return fmt.Errorf("ZIP limits must be greater than zero")
	}
	fetcher.ZIPLimits = testfixture.ZIPLimits{
		MaxEntries: *maxZIPEntries, MaxUncompressedSize: *maxZIPSize,
	}
	manifest, err := testfixture.LoadManifest(*manifestPath)
	if err != nil {
		return err
	}
	results, err := fetcher.FetchAll(ctx, manifest, *destination)
	if err != nil {
		return err
	}
	if err := testfixture.WriteIndex(*destination, results); err != nil {
		return err
	}
	for _, result := range results {
		fmt.Printf("%s\t%s\n", result.ID, result.Path)
	}
	return nil
}
