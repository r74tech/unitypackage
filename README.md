# unitypackage

Dependency-free Go library and CLI for creating, inspecting, validating, and
extracting legacy Unity `.unitypackage` archives without Unity Editor.

## Install

```sh
go install github.com/r74tech/unitypackage/cmd/unitypackage@latest
```

## CLI

```sh
unitypackage pack --root . --output dist/package.unitypackage Assets/MyPackage
unitypackage pack --root . --output dist/package.unitypackage \
  --include 'Assets/MyPackage/**' --exclude '**/*.tmp'
unitypackage pack-upm --root Packages/com.example.tool \
  --output dist/com.example.tool.unitypackage
unitypackage pack-upm --root ./downloaded-vpm-package \
  --destination Assets/ExampleTool \
  --output dist/example-tool-legacy.unitypackage
unitypackage verify dist/package.unitypackage
unitypackage list --json dist/package.unitypackage
unitypackage extract --root ./Imported dist/package.unitypackage
```

`list`, `verify`, and `extract` process at most 100,000 archive entries and
1 GiB of gzip-expanded archive data by default. Override either ceiling
explicitly for a trusted larger package:

```sh
unitypackage verify \
  --max-archive-entries 2000000 \
  --max-archive-uncompressed-size 8589934592 \
  dist/large-package.unitypackage
```

`pack` accepts asset paths or `.meta` paths. A newline-delimited list can be
passed with `--files-from`.

### UPM and VPM conversion

`pack-upm` and its `pack-vpm` alias read a package directory containing
`package.json`. The default archive path is `Packages/<manifest.name>`, which
preserves the package as an embedded-package layout. Use `--destination
Assets/...` only when a legacy Assets layout is intended.

This is a filesystem conversion, not a package-manager migration:

- `dependencies` and `vpmDependencies` are reported by `upm.ReadManifest`
  but are not downloaded or bundled.
- A package can contain code or resources that depend on its original mount
  path. Remapping it under `Assets` does not rewrite those references.
- Every converted file and nested directory must have its committed `.meta`;
  conversion fails instead of silently omitting untracked package content.
- VRChat SDK packages have product-specific legacy layouts. Converting a
  current SDK mechanically is not a replacement for VRChat's supported VCC
  [migration flow](https://vcc.docs.vrchat.com/vpm/migrating/).

Resolve and pin dependencies first, then convert each package explicitly when
a self-contained legacy distribution is required.

## Library

```go
import (
    "github.com/r74tech/unitypackage"
    "github.com/r74tech/unitypackage/upm"
)

err := unitypackage.Pack(unitypackage.PackOptions{
    Root:   ".",
    Output: "dist/package.unitypackage",
    Paths:  []string{"Assets/MyPackage"},
})

err = upm.Pack(upm.PackOptions{
    Root:        "Packages/com.example.tool",
    Destination: "Assets/ExampleTool",
    Output:      "dist/example-tool.unitypackage",
})

err = unitypackage.Extract(unitypackage.ExtractOptions{
    Input: "dist/example-tool.unitypackage",
    Root:  "./Imported",
    Limits: unitypackage.ReadLimits{
        MaxUncompressedSize: 8 << 30,
    },
})
```

The root package exposes the legacy archive operations `Pack`, `Inspect`,
`Verify`, and `Extract`. UPM/VPM manifest reading and conversion live in the
separate `upm` package so package-manager concerns do not expand the archive
API.

## Design guarantees

- Reads only `guid` and `folderAsset` from Unity's YAML-like `.meta` format.
- Never creates or modifies source `.meta` files.
- Uses only Go's standard `archive/tar` and `compress/gzip` packages.
- Produces deterministic archives.
- Rejects missing metadata, duplicate GUIDs, unsafe paths, symlinks,
  non-regular source files, and malformed package records.
- Detects path, `.meta`, ancestor, and case-folding collisions before extract.
- Bounds archive entry count and the entire gzip-expanded tar stream; callers
  can override both defaults.
- Stages extraction and rolls back installed files if commit fails.
- Creates local output directories with `0700` and writes archive, asset, and
  `.meta` files with `0600`; existing destination directory permissions are
  not changed.

Creating an archive does not compile or import its contents. Use a separate
Unity Editor test when import compatibility with a specific Unity version must
be proven.

## Test data

Unit tests build only the minimal projects and malformed archives needed for a
specific behavior. Interoperability is tested independently against immutable
official artifacts:

- [Unity Technologies' Post Processing
  1.0.4](https://github.com/Unity-Technologies/PostProcessing/releases/tag/1.0.4)
  `.unitypackage` (233 records).
- VRChat's `com.vrchat.core.vpm-resolver` package from its
  [official VPM listing](https://vrchat.github.io/packages/index.json).

Their source URLs, sizes, and SHA-256 digests are pinned in
`testdata/official/manifest.json`. The repository stores only this small
manifest; downloaded binaries live in a private, content-addressed cache.
Run the same suite as CI with:

```sh
go run ./internal/cmd/fetchtestdata -dir /tmp/unitypackage-testdata
UNITYPACKAGE_TESTDATA_DIR=/tmp/unitypackage-testdata \
  go test -tags=interop ./internal/interop
```

For an intentionally larger official fixture, override the ZIP ceilings in
bytes explicitly:

```sh
go run ./internal/cmd/fetchtestdata \
  -dir /tmp/unitypackage-testdata \
  -max-zip-entries 200000 \
  -max-zip-uncompressed-size 1073741824
```

The fetcher uses `0700` cache directories and `0600` cache files, verifies
size and SHA-256 before publishing a blob, and rejects unsafe ZIP paths and
symlinks, including symlinked managed cache directories. ZIP extraction is
preflighted for portable paths, duplicate and topology conflicts, at most
100,000 entries, and at most 500 MiB of expanded data. Re-running it reuses
verified blobs; an extraction cache version forces revalidation when those
rules change.

## Development

Go 1.26.5 or newer is required. The patch version is intentional because this
tool processes archives and performs HTTPS fixture downloads.

```sh
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

## Release

Releases use semantic versions with a `v` prefix, such as `v1.2.3`. The Git
tag is the published version of the Go module; do not create a GitHub Release
manually first.

From an up-to-date `main` branch, run the release checks:

```sh
git switch main
git pull --ff-only
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

Create an annotated tag on the checked commit, inspect it, and push only that
tag:

```sh
release_version=v1.2.3
git tag -a "$release_version" -m "$release_version"
git show --no-patch "$release_version"
git push origin "$release_version"
```

Pushing a `v*` tag starts the Release workflow. It repeats the tests, race
detector, vet, and vulnerability scan before building Linux, macOS, and
Windows binaries. It then generates `checksums.txt` and publishes one GitHub
Release only after every build artifact is available. The CLI version is set
to the tag name during these builds.

If the workflow fails, fix the workflow or transient failure and rerun the
failed GitHub Actions run. A draft left by a partial asset upload is resumed
and published after all assets have been replaced. Never move, delete, or reuse
a tag that has been pushed: Go module consumers may already have resolved it.
Publish a corrected release under a new version instead.

## License

MIT
