package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	unitypackage "github.com/r74tech/unitypackage"
	"github.com/r74tech/unitypackage/upm"
)

var version = "dev"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "unitypackage: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("a command is required")
	}
	switch args[0] {
	case "pack":
		return runPack(args[1:])
	case "pack-upm", "pack-vpm":
		return runPackUPM(args[1:])
	case "list":
		return runList(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "extract":
		return runExtract(args[1:])
	case "version", "--version", "-version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: unitypackage <command> [options]

Commands:
  pack       Create a deterministic .unitypackage
  pack-upm   Convert a UPM/VPM package directory to a .unitypackage
  list       List package contents
  verify     Validate package structure and safety
  extract    Extract into a Unity project
  version    Print the version`)
}

func runPack(args []string) error {
	flags := flag.NewFlagSet("pack", flag.ContinueOnError)
	root := flags.String("root", ".", "Unity project or package root")
	output := flags.String("output", "", "destination .unitypackage")
	prefix := flags.String("prefix", "", "archive pathname prefix")
	filesFrom := flags.String("files-from", "", "newline-delimited asset or .meta paths")
	var includes, excludes stringList
	flags.Var(&includes, "include", "glob to include; repeatable and supports **")
	flags.Var(&excludes, "exclude", "glob to exclude; repeatable and supports **")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	paths := flags.Args()
	if *filesFrom != "" {
		fromFile, err := readLines(*filesFrom)
		if err != nil {
			return err
		}
		paths = append(paths, fromFile...)
	}
	return unitypackage.Pack(unitypackage.PackOptions{Root: *root, Output: *output, PathPrefix: *prefix, Paths: paths, Includes: includes, Excludes: excludes})
}

func runPackUPM(args []string) error {
	flags := flag.NewFlagSet("pack-upm", flag.ContinueOnError)
	root := flags.String("root", ".", "UPM/VPM package root containing package.json")
	output := flags.String("output", "", "destination .unitypackage")
	destination := flags.String("destination", "", "import path (default Packages/<manifest.name>)")
	var includes, excludes stringList
	flags.Var(&includes, "include", "package-relative glob to include; repeatable")
	flags.Var(&excludes, "exclude", "package-relative glob to exclude; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pack-upm does not accept positional paths")
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	return upm.Pack(upm.PackOptions{
		Root: *root, Output: *output, Destination: *destination,
		Includes: includes, Excludes: excludes,
	})
}

func runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	readLimits := addReadLimitFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("list requires one .unitypackage path")
	}
	entries, err := unitypackage.InspectWithLimits(flags.Arg(0), readLimits.limits())
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(entries)
	}
	for _, entry := range entries {
		kind := "file"
		if entry.IsFolder {
			kind = "folder"
		}
		fmt.Printf("%s\t%s\t%s\n", entry.GUID, kind, entry.Path)
	}
	return nil
}

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	readLimits := addReadLimitFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("verify requires one .unitypackage path")
	}
	if err := unitypackage.VerifyWithLimits(flags.Arg(0), readLimits.limits()); err != nil {
		return err
	}
	fmt.Printf("valid unitypackage: %s\n", flags.Arg(0))
	return nil
}

func runExtract(args []string) error {
	flags := flag.NewFlagSet("extract", flag.ContinueOnError)
	root := flags.String("root", ".", "destination Unity project root")
	overwrite := flags.Bool("overwrite", false, "replace existing assets")
	readLimits := addReadLimitFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("extract requires one .unitypackage path")
	}
	return unitypackage.Extract(unitypackage.ExtractOptions{
		Input: flags.Arg(0), Root: *root, Overwrite: *overwrite, Limits: readLimits.limits(),
	})
}

type readLimitFlags struct {
	maxEntries          *int
	maxUncompressedSize *uint64
}

func addReadLimitFlags(flags *flag.FlagSet) readLimitFlags {
	return readLimitFlags{
		maxEntries: flags.Int(
			"max-archive-entries", unitypackage.DefaultMaxArchiveEntries,
			"maximum archive entry count",
		),
		maxUncompressedSize: flags.Uint64(
			"max-archive-uncompressed-size", unitypackage.DefaultMaxArchiveUncompressedSize,
			"maximum total archive expansion in bytes",
		),
	}
}

func (flags readLimitFlags) limits() unitypackage.ReadLimits {
	return unitypackage.ReadLimits{
		MaxEntries: *flags.maxEntries, MaxUncompressedSize: *flags.maxUncompressedSize,
	}
}

func readLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open files list: %w", err)
	}
	defer f.Close()
	var result []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			result = append(result, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read files list: %w", err)
	}
	return result, nil
}
