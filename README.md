# heif-meta-parser

[简体中文](README.zh-CN.md)

`heif-meta-parser` is a small Go library for reading HEIF/HEIC metadata without
decoding image pixels.

It parses enough ISO BMFF/HEIF structure to report the primary image dimensions,
rotation-adjusted dimensions, frame count, major brand, compatible brands,
format, and MIME type.

## What It Parses

The parser reads HEIF metadata from the BMFF box structure. It currently extracts:

- File type metadata: major brand and compatible brands from `ftyp`
- Primary image item: primary item ID from `pitm`
- Image item records: item ID, item type, item name, and hidden flag from `iinf` / `infe`
- Image dimensions: width and height from the `ispe` item property
- Rotation metadata: quarter-turn rotation from the `irot` item property
- Item-property links: property associations from `iprp` / `ipco` / `ipma`
- Item references: thumbnail, auxiliary image, and mask relationships from `iref`
- Top-level images: image items excluding hidden, thumbnail, auxiliary, and mask items

The library does not parse or decode the compressed media payload in `mdat`.

## Install

```sh
go get github.com/turtlemeow/heif-meta-parser
```

## Core API

### `DecodeConfig`

Use `DecodeConfig` when you only need the common image configuration.

```go
package main

import (
	"fmt"
	"os"

	heifmeta "github.com/turtlemeow/heif-meta-parser"
)

func main() {
	f, err := os.Open("image.heic")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	cfg, err := heifmeta.DecodeConfig(f)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%dx%d %s %s frames=%d\n",
		cfg.Width, cfg.Height, cfg.Format, cfg.MainBrand, cfg.FrameCount)
}
```

It returns:

```go
type Config struct {
	Width            int
	Height           int
	FrameCount       int
	Format           Format
	MIME             string
	MainBrand        string
	CompatibleBrands []string
}
```

`Width` and `Height` are adjusted with HEIF rotation metadata when an `irot`
property is present.

### `Parse`

Use `Parse` when you need item-level metadata:

```go
file, err := heifmeta.Parse(f)
if err != nil {
	panic(err)
}

for _, img := range file.Images {
	w, h := img.RotatedDimensions()
	fmt.Printf("id=%d primary=%t type=%s size=%dx%d\n",
		img.ID, img.Primary, img.ItemType, w, h)
}
```

It returns:

```go
type File struct {
	MainBrand        string
	CompatibleBrands []string
	Images           []Image
	TopLevelImages   []Image
	PrimaryImage     *Image
}

type Image struct {
	ID        uint32
	Width     uint32
	Height    uint32
	Rotation  Rotation
	ItemType  string
	Name      string
	Primary   bool
	Hidden    bool
	Thumbnail bool
	Auxiliary bool
	Mask      bool
}
```

`Images` contains all parsed image items. `TopLevelImages` excludes hidden,
thumbnail, auxiliary, and mask items. `PrimaryImage` points to the item selected
by the HEIF `pitm` box.

### `IsSupportedBrand`

Use `IsSupportedBrand` if you already have a HEIF major brand and only need to
check whether this package maps it to a normalized format.

## CLI

```sh
go install github.com/turtlemeow/heif-meta-parser/cmd/heifmeta@latest
heifmeta testdata/sample.heic
```

The repository includes `testdata/sample.heic`, a GPS-stripped HEIC file that
can be used with the CLI and tests.

## Scope

Supported metadata boxes include:

- `ftyp`
- `meta`
- `pitm`
- `iinf` / `infe`
- `iprp` / `ipco` / `ipma`
- `ispe`
- `irot`
- `iref`

Supported major brands:

- HEIC still images: `heic`, `heix`, `heim`, `heis`
- HEIF still images: `mif1`
- HEIC sequences: `hevc`, `hevx`, `hevm`, `hevs`
- HEIF sequences: `msf1`

This package does not decode pixel data, validate every HEIF feature, or parse
the media payload in `mdat`.

## License

MIT
