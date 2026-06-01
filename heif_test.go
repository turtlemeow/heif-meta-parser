package heifmeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodeConfig(t *testing.T) {
	data := minimalHEIF("heic", 1, 3024, 4032, 0)

	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Width != 3024 || cfg.Height != 4032 {
		t.Fatalf("unexpected dimensions: %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.FrameCount != 1 {
		t.Fatalf("unexpected frame count: %d", cfg.FrameCount)
	}
	if cfg.Format != FormatHEIC || cfg.MIME != "image/heic" || cfg.MainBrand != "heic" {
		t.Fatalf("unexpected format: %+v", cfg)
	}
}

func TestParseFile(t *testing.T) {
	data := minimalHEIF("mif1", 1, 1440, 960, 2)

	file, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if file.MainBrand != "mif1" {
		t.Fatalf("unexpected brand: %s", file.MainBrand)
	}
	if len(file.Images) != 1 || len(file.TopLevelImages) != 1 {
		t.Fatalf("unexpected image counts: all=%d top=%d", len(file.Images), len(file.TopLevelImages))
	}
	if file.PrimaryImage == nil {
		t.Fatal("missing primary image")
	}
	if *file.PrimaryImage != file.Images[0] {
		t.Fatalf("primary image mismatch: primary=%+v images=%+v", file.PrimaryImage, file.Images)
	}
	if !file.PrimaryImage.Primary || file.PrimaryImage.ID != 1 || file.PrimaryImage.ItemType != "hvc1" {
		t.Fatalf("unexpected primary image: %+v", file.PrimaryImage)
	}
	if file.PrimaryImage.Rotation != Rotation180 || file.PrimaryImage.Rotation.Degrees() != 180 {
		t.Fatalf("unexpected rotation: %d", file.PrimaryImage.Rotation)
	}
}

func TestDecodeConfigAppliesRotation(t *testing.T) {
	data := minimalHEIF("heic", 1, 3024, 4032, 1)

	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Width != 4032 || cfg.Height != 3024 {
		t.Fatalf("unexpected rotated dimensions: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestDecodeConfigUnsupportedBrand(t *testing.T) {
	data := minimalHEIF("avif", 1, 1920, 1080, 0)

	_, err := DecodeConfig(bytes.NewReader(data))
	if !errors.Is(err, ErrUnsupportedBrand) {
		t.Fatalf("expected ErrUnsupportedBrand, got %v", err)
	}
}

func minimalHEIF(brand string, itemID uint16, width, height uint32, rotation byte) []byte {
	return append(
		makeBox("ftyp", append([]byte(brand+"\x00\x00\x00\x00"), []byte("mif1heic")...)),
		makeBox("meta",
			append(
				[]byte{0, 0, 0, 0},
				append(
					makeBox("pitm", append([]byte{0, 0, 0, 0}, u16(itemID)...)),
					append(
						makeBox("iinf",
							append(
								[]byte{0, 0, 0, 0},
								append(u16(1), infe(itemID, "hvc1", false)...)...,
							),
						),
						makeBox("iprp",
							append(
								makeBox("ipco",
									append(
										makeBox("ispe", append(append([]byte{0, 0, 0, 0}, u32(width)...), u32(height)...)),
										makeBox("irot", []byte{rotation})...,
									),
								),
								ipma(itemID, 1, 2)...,
							),
						)...,
					)...,
				)...,
			),
		)...,
	)
}

func infe(itemID uint16, itemType string, hidden bool) []byte {
	flags := []byte{2, 0, 0, 0}
	if hidden {
		flags[3] = 1
	}
	body := append(flags, u16(itemID)...)
	body = append(body, u16(0)...)
	body = append(body, []byte(itemType)...)
	body = append(body, 0)
	return makeBox("infe", body)
}

func ipma(itemID uint16, propertyIndexes ...byte) []byte {
	body := append([]byte{0, 0, 0, 0}, u32(1)...)
	body = append(body, u16(itemID)...)
	body = append(body, byte(len(propertyIndexes)))
	body = append(body, propertyIndexes...)
	return makeBox("ipma", body)
}

func makeBox(typ string, body []byte) []byte {
	size := uint32(8 + len(body))
	out := u32(size)
	out = append(out, []byte(typ)...)
	out = append(out, body...)
	return out
}

func u16(v uint16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return b[:]
}

func u32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}
