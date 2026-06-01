# heif-meta-parser

`heif-meta-parser` is a small Go library for reading HEIF/HEIC metadata without
decoding image pixels.

It parses enough ISO BMFF/HEIF structure to report the primary image dimensions,
rotation-adjusted dimensions, frame count, major brand, compatible brands,
format, and MIME type.

## Install

```sh
go get github.com/turtlemeow/heif-meta-parser
```

## Usage

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

For detailed item metadata, use `Parse`:

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

## CLI

```sh
go install github.com/turtlemeow/heif-meta-parser/cmd/heifmeta@latest
heifmeta image.heic
```

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
