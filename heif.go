package heifmeta

import (
	"errors"
	"io"
	"sort"
)

// ErrUnsupportedBrand is returned when the HEIF major brand is not mapped to a
// supported image format by this package.
var ErrUnsupportedBrand = errors.New("heifmeta: unsupported brand")

// Format is the normalized HEIF family format.
type Format string

const (
	// FormatHEIC is a HEIF still image encoded with HEVC.
	FormatHEIC Format = "heic"
	// FormatHEIF is a HEIF still image with a generic image coding brand.
	FormatHEIF Format = "heif"
	// FormatHEICSequence is a HEIF image sequence encoded with HEVC.
	FormatHEICSequence Format = "heic-sequence"
	// FormatHEIFSequence is a HEIF image sequence with a generic coding brand.
	FormatHEIFSequence Format = "heif-sequence"
)

// Config contains the high-level metadata commonly needed before image
// processing. Width and Height include HEIF rotation metadata when present.
type Config struct {
	Width            int
	Height           int
	FrameCount       int
	Format           Format
	MIME             string
	MainBrand        string
	CompatibleBrands []string
}

// Rotation is the HEIF irot property value. HEIF stores rotation as quarter
// turns counter-clockwise.
type Rotation uint8

const (
	Rotation0   Rotation = 0
	Rotation90  Rotation = 1
	Rotation180 Rotation = 2
	Rotation270 Rotation = 3
)

// Degrees returns the rotation in degrees.
func (r Rotation) Degrees() int {
	return int(r) * 90
}

// Image contains metadata for one image item in a HEIF file.
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

// RotatedDimensions returns Width and Height after applying HEIF rotation
// metadata.
func (img Image) RotatedDimensions() (uint32, uint32) {
	if img.Rotation == Rotation90 || img.Rotation == Rotation270 {
		return img.Height, img.Width
	}
	return img.Width, img.Height
}

// File contains the high-level metadata parsed from a HEIF file.
type File struct {
	MainBrand        string
	CompatibleBrands []string
	Images           []Image
	TopLevelImages   []Image
	PrimaryImage     *Image
}

// Parse reads HEIF metadata and returns the parsed file description.
func Parse(r io.Reader) (*File, error) {
	ctx, err := Open(r).parseContext()
	if err != nil {
		return nil, err
	}
	return fileFromContext(ctx), nil
}

// DecodeConfig reads HEIF metadata and returns the normalized image
// configuration. It does not decode pixel data.
func DecodeConfig(r io.Reader) (Config, error) {
	file, err := Parse(r)
	if err != nil {
		return Config{}, err
	}

	format, mime, ok := brandFormat(file.MainBrand)
	if !ok {
		return Config{}, ErrUnsupportedBrand
	}
	if file.PrimaryImage == nil {
		return Config{}, errors.New("heifmeta: no primary item")
	}

	width, height := file.PrimaryImage.RotatedDimensions()

	return Config{
		Width:            int(width),
		Height:           int(height),
		FrameCount:       len(file.TopLevelImages),
		Format:           format,
		MIME:             mime,
		MainBrand:        file.MainBrand,
		CompatibleBrands: append([]string(nil), file.CompatibleBrands...),
	}, nil
}

// IsSupportedBrand reports whether brand can be mapped to one of this package's
// normalized formats.
func IsSupportedBrand(brand string) bool {
	_, _, ok := brandFormat(brand)
	return ok
}

func fileFromContext(ctx parseContext) *File {
	file := &File{
		MainBrand:        ctx.MainBrand,
		CompatibleBrands: append([]string(nil), ctx.CompatibleBrands...),
	}

	allIDs := sortedImageIDs(ctx.AllImageItems)
	topLevelIDs := sortedImageIDs(ctx.TopLevelImageItems)

	for _, id := range allIDs {
		img := imageFromItem(ctx.AllImageItems[id], ctx.PrimaryItem)
		file.Images = append(file.Images, img)
		if img.Primary {
			primary := img
			file.PrimaryImage = &primary
		}
	}
	for _, id := range topLevelIDs {
		file.TopLevelImages = append(file.TopLevelImages, imageFromItem(ctx.TopLevelImageItems[id], ctx.PrimaryItem))
	}

	return file
}

func sortedImageIDs(items map[uint32]*imageItem) []uint32 {
	ids := make([]uint32, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func imageFromItem(item *imageItem, primary *imageItem) Image {
	img := Image{
		ID:        item.id,
		Width:     item.Width,
		Height:    item.Height,
		Rotation:  Rotation(item.Rotation),
		Hidden:    item.isHidden,
		Thumbnail: item.isThumbnail,
		Auxiliary: item.isAuxiliary,
		Mask:      item.isMask,
	}
	if item.originInfeBox != nil {
		img.ItemType = item.originInfeBox.ItemType
		img.Name = item.originInfeBox.Name
	}
	if primary != nil && item.id == primary.id {
		img.Primary = true
	}
	return img
}

func brandFormat(brand string) (Format, string, bool) {
	switch brand {
	case "heic", "heix", "heim", "heis":
		return FormatHEIC, "image/heic", true
	case "mif1":
		return FormatHEIF, "image/heif", true
	case "hevc", "hevx", "hevm", "hevs":
		return FormatHEICSequence, "image/heic-sequence", true
	case "msf1":
		return FormatHEIFSequence, "image/heif-sequence", true
	default:
		return "", "", false
	}
}
