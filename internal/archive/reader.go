package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultMaxEntries          = 100_000
	DefaultMaxUncompressedSize = uint64(1 << 30)
	maxMetadataSize            = 4 << 20
)

type Limits struct {
	MaxEntries          int
	MaxUncompressedSize uint64
}

type boundedReader struct {
	reader    io.Reader
	limit     uint64
	remaining uint64
}

func (reader *boundedReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		var probe [1]byte
		count, err := reader.reader.Read(probe[:])
		if count != 0 {
			return 0, fmt.Errorf("package expands beyond %d bytes", reader.limit)
		}
		return 0, err
	}
	if uint64(len(buffer)) > reader.remaining {
		buffer = buffer[:int(reader.remaining)]
	}
	count, err := reader.reader.Read(buffer)
	reader.remaining -= uint64(count)
	return count, err
}

func (limits Limits) withDefaults() (Limits, error) {
	if limits.MaxEntries < 0 {
		return Limits{}, fmt.Errorf("maximum archive entry count must not be negative")
	}
	if limits.MaxEntries == 0 {
		limits.MaxEntries = DefaultMaxEntries
	}
	if limits.MaxUncompressedSize == 0 {
		limits.MaxUncompressedSize = DefaultMaxUncompressedSize
	}
	return limits, nil
}

type rawRecord struct {
	guid      string
	kinds     map[string]bool
	meta      []byte
	pathname  []byte
	assetPath string
}

// Open validates a unitypackage and optionally stages asset payloads for
// extraction. The caller must close the returned Package.
func Open(filename string, stageAssets bool, limits Limits) (*Package, error) {
	limits, err := limits.withDefaults()
	if err != nil {
		return nil, err
	}
	input, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open package: %w", err)
	}
	defer input.Close()
	gz, err := gzip.NewReader(input)
	if err != nil {
		return nil, invalidf("read gzip: %v", err)
	}
	defer gz.Close()

	staging := ""
	if stageAssets {
		staging, err = os.MkdirTemp("", "unitypackage-read-*")
		if err != nil {
			return nil, err
		}
	}
	fail := func(err error) (*Package, error) {
		if staging != "" {
			_ = os.RemoveAll(staging)
		}
		return nil, err
	}

	recordsByGUID := make(map[string]*rawRecord)
	expanded := &boundedReader{
		reader: gz, limit: limits.MaxUncompressedSize, remaining: limits.MaxUncompressedSize,
	}
	tw := tar.NewReader(expanded)
	var totalSize uint64
	for count := 0; ; count++ {
		header, err := tw.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(invalidf("read tar: %v", err))
		}
		if count >= limits.MaxEntries {
			return fail(invalidf("package has too many entries"))
		}
		if header.Size < 0 {
			return fail(invalidf("package entry %q has a negative size", header.Name))
		}
		entrySize := uint64(header.Size)
		if entrySize > limits.MaxUncompressedSize-totalSize {
			return fail(invalidf("package expands beyond %d bytes", limits.MaxUncompressedSize))
		}
		totalSize += entrySize

		name := strings.TrimPrefix(header.Name, "./")
		clean := path.Clean(name)
		if clean == "." && header.Typeflag == tar.TypeDir {
			continue
		}
		if isRootMetadata(clean) {
			if clean != name || !isRegular(header.Typeflag) {
				return fail(invalidf("invalid package metadata entry %q", header.Name))
			}
			if header.Size > maxMetadataSize {
				return fail(invalidf("package metadata %q is too large", header.Name))
			}
			if _, err := io.CopyN(io.Discard, tw, header.Size); err != nil {
				return fail(invalidf("read package metadata %q: %v", header.Name, err))
			}
			continue
		}

		parts := strings.Split(clean, "/")
		if len(parts) == 1 && guidPattern.MatchString(parts[0]) && header.Typeflag == tar.TypeDir {
			continue
		}
		if clean != name || len(parts) != 2 || !guidPattern.MatchString(parts[0]) {
			return fail(invalidf("unsafe or invalid archive path %q", header.Name))
		}
		guid, kind := strings.ToLower(parts[0]), parts[1]
		if !isRecordKind(kind) {
			return fail(invalidf("unexpected package entry %q", header.Name))
		}
		if !isRegular(header.Typeflag) {
			return fail(invalidf("package entry %q is not a regular file", header.Name))
		}
		record := recordsByGUID[guid]
		if record == nil {
			record = &rawRecord{guid: guid, kinds: make(map[string]bool)}
			recordsByGUID[guid] = record
		}
		if record.kinds[kind] {
			return fail(invalidf("duplicate package entry %q", header.Name))
		}
		record.kinds[kind] = true

		switch kind {
		case "asset":
			if !stageAssets {
				if _, err := io.CopyN(io.Discard, tw, header.Size); err != nil {
					return fail(invalidf("read asset %q: %v", header.Name, err))
				}
				continue
			}
			directory := filepath.Join(staging, guid)
			if err := os.MkdirAll(directory, 0o700); err != nil {
				return fail(err)
			}
			record.assetPath = filepath.Join(directory, "asset")
			output, err := os.OpenFile(record.assetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return fail(err)
			}
			_, copyErr := io.CopyN(output, tw, header.Size)
			closeErr := output.Close()
			if copyErr != nil {
				return fail(invalidf("read asset %q: %v", header.Name, copyErr))
			}
			if closeErr != nil {
				return fail(closeErr)
			}
		case "asset.meta", "pathname":
			if header.Size > maxMetadataSize {
				return fail(invalidf("package entry %q is too large", header.Name))
			}
			data := make([]byte, header.Size)
			if _, err := io.ReadFull(tw, data); err != nil {
				return fail(invalidf("read package entry %q: %v", header.Name, err))
			}
			if kind == "asset.meta" {
				record.meta = data
			} else {
				record.pathname = data
			}
		default:
			if header.Size > maxMetadataSize {
				return fail(invalidf("package entry %q is too large", header.Name))
			}
			if _, err := io.CopyN(io.Discard, tw, header.Size); err != nil {
				return fail(invalidf("read package entry %q: %v", header.Name, err))
			}
		}
	}
	// Drain the gzip stream so checksum and trailer corruption are detected.
	if _, err := io.Copy(io.Discard, expanded); err != nil {
		return fail(invalidf("finish gzip stream: %v", err))
	}

	entries := make([]Entry, 0, len(recordsByGUID))
	for guid, record := range recordsByGUID {
		if !record.kinds["asset.meta"] || !record.kinds["pathname"] {
			return fail(invalidf("GUID %s is missing asset.meta or pathname", guid))
		}
		meta, err := ParseMeta(record.meta)
		if err != nil {
			return fail(invalidf("GUID %s has invalid asset.meta: %v", guid, err))
		}
		if meta.GUID != guid {
			return fail(invalidf("directory GUID %s does not match meta GUID %s", guid, meta.GUID))
		}
		assetPath, err := ValidatePathname(string(record.pathname))
		if err != nil {
			return fail(fmt.Errorf("%w: GUID %s: %w", ErrInvalidArchive, guid, err))
		}
		if meta.IsFolder && record.kinds["asset"] {
			return fail(invalidf("folder %q unexpectedly contains an asset payload", assetPath))
		}
		if !meta.IsFolder && !record.kinds["asset"] {
			return fail(invalidf("file %q is missing its asset payload", assetPath))
		}
		entries = append(entries, Entry{
			GUID: guid, Path: assetPath, HasAsset: record.kinds["asset"],
			IsFolder: meta.IsFolder, Meta: record.meta, AssetPath: record.assetPath,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if err := validateEntrySet(entries); err != nil {
		return fail(err)
	}
	return &Package{Entries: entries, staging: staging}, nil
}

func isRootMetadata(name string) bool {
	switch name {
	case ".icon.png", ".icon.jpg", "packagemanagermanifest", "projectversion":
		return true
	default:
		return false
	}
}

func isRecordKind(kind string) bool {
	switch kind {
	case "asset", "asset.meta", "pathname", "preview.png", "preview.asset":
		return true
	default:
		return false
	}
}

func isRegular(typeflag byte) bool {
	return typeflag == tar.TypeReg || typeflag == tar.TypeRegA
}

func validateEntrySet(entries []Entry) error {
	paths := make(map[string]Entry, len(entries))
	outputs := make(map[string]string, len(entries)*2)
	for _, entry := range entries {
		key := strings.ToLower(entry.Path)
		if previous, ok := paths[key]; ok {
			return invalidf("duplicate or case-conflicting pathname %q for GUIDs %s and %s", entry.Path, previous.GUID, entry.GUID)
		}
		paths[key] = entry
		for _, output := range []string{entry.Path, entry.Path + ".meta"} {
			outputKey := strings.ToLower(output)
			if previous, ok := outputs[outputKey]; ok {
				return invalidf("package path collision between %q and %q", previous, output)
			}
			outputs[outputKey] = output
		}
	}
	for _, entry := range entries {
		for parent := path.Dir(entry.Path); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if ancestor, ok := paths[strings.ToLower(parent)]; ok && !ancestor.IsFolder {
				return invalidf("file %q is an ancestor of %q", ancestor.Path, entry.Path)
			}
		}
	}
	return nil
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArchive, fmt.Sprintf(format, args...))
}
