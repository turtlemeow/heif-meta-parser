package heifmeta

import (
	"errors"
	"io"
)

// HeifFile represents a HEIF file.
//
// Methods on HeifFile should not be called concurrently.
type HeifFile struct {
	reader io.Reader
	meta   *BoxMeta
}

// BoxMeta contains the low-level BMFF metadata boxes.
type BoxMeta struct {
	FileType    *FileTypeBox
	PrimaryItem *PrimaryItemBox
	ItemInfo    *ItemInfoBox
	Properties  *ItemPropertiesBox
	ItemRefs    *ItemReferenceBox
}

type imageItem struct {
	Width    uint32
	Height   uint32
	Rotation uint8 // 0: 0, 1: 90, 2: 180, 3: 270 degrees.

	id          uint32
	isHidden    bool
	isThumbnail bool
	isAuxiliary bool
	isMask      bool

	originInfeBox *ItemInfoEntry
}
type parseContext struct {
	MainBrand          string
	CompatibleBrands   []string
	AllImageItems      map[uint32]*imageItem // itemID -> imageItem
	TopLevelImageItems map[uint32]*imageItem // itemID -> imageItem
	PrimaryItem        *imageItem
}

// Item represents an item in a HEIF file.
type Item struct {
	ID         uint32
	Info       *ItemInfoEntry
	Properties []Box
}

// Open returns a handle to access a HEIF file.
func Open(f io.Reader) *HeifFile {
	return &HeifFile{reader: f}
}

// ErrUnknownItem is returned by HeifFile.GetItemByID for unknown items.
var ErrUnknownItem = errors.New("heif: unknown item")

func (f *HeifFile) getMeta() (*BoxMeta, error) {
	if f.meta != nil {
		return f.meta, nil
	}
	bmr := NewReader(f.reader)
	meta := &BoxMeta{}

	pbox, err := bmr.ReadAndParseBox(TypeFtyp, false)
	if err != nil {
		return nil, err
	}
	meta.FileType = pbox.(*FileTypeBox)

	pbox, err = bmr.ReadAndParseBox(TypeMeta, true)
	if err != nil {
		return nil, err
	}
	metabox := pbox.(*MetaBox)

	for _, box := range metabox.Children {
		boxp, err := box.Parse()
		if errors.Is(err, ErrUnknownBox) {
			continue
		}
		if err != nil {
			return nil, err
		}
		switch v := boxp.(type) {
		case *PrimaryItemBox:
			meta.PrimaryItem = v
		case *ItemInfoBox:
			meta.ItemInfo = v
		case *ItemPropertiesBox:
			meta.Properties = v
		case *ItemReferenceBox:
			meta.ItemRefs = v
		}
	}

	f.meta = meta
	return f.meta, nil
}

func (f *HeifFile) parseContext() (parseContext, error) {
	meta, err := f.getMeta()
	if err != nil {
		return parseContext{}, err
	}
	hc := &parseContext{
		AllImageItems:      make(map[uint32]*imageItem),
		TopLevelImageItems: make(map[uint32]*imageItem),
	}
	// get main brand and compatible brands
	hc.MainBrand = meta.FileType.MajorBrand
	hc.CompatibleBrands = meta.FileType.Compatible
	if meta.Properties == nil {
		return parseContext{}, errors.New("heif: HEIF file lacks item properties box")
	}
	if meta.ItemInfo == nil {
		return parseContext{}, errors.New("heif: HEIF file lacks item info box")
	}

	primaryItemId := meta.PrimaryItem.ItemID
	allParsedProps := meta.Properties.PropertyContainer.ParsedProperties
	for _, iie := range meta.ItemInfo.ItemInfos {
		if !iie.IsImage {
			continue
		}
		item := &imageItem{
			originInfeBox: iie,
			id:            iie.ItemID,
			isHidden:      iie.Hidden,
		}

		for _, ipma := range meta.Properties.Associations {
			for _, ipai := range ipma.Entries {
				if ipai.ItemID != iie.ItemID {
					continue
				}
				for _, prop := range ipai.Associations {
					if prop.Index <= 0 || int(prop.Index) > len(allParsedProps) {
						continue
					}
					propItem := allParsedProps[prop.Index-1]
					if propItem == nil {
						continue
					}
					if p, ok := propItem.(*ImageSpatialExtentsProperty); ok {
						item.Width = p.ImageWidth
						item.Height = p.ImageHeight
					}
					if p, ok := propItem.(*ImageRotation); ok {
						item.Rotation = p.Angle
					}
				}
			}
		}
		hc.AllImageItems[item.id] = item
		if item.id == uint32(primaryItemId) {
			hc.PrimaryItem = item
		}

		if meta.ItemRefs != nil {
			for _, iref := range meta.ItemRefs.entrys {
				if iref.from != iie.ItemID {
					continue
				}
				switch iref.typ {
				case "thmb":
					item.isThumbnail = true
				case "auxl":
					item.isAuxiliary = true
				case "mask":
					item.isMask = true
				default:
				}
			}
		}
		if !item.isHidden && !item.isAuxiliary && !item.isThumbnail && !item.isMask {
			hc.TopLevelImageItems[item.id] = item
		}
	}
	return *hc, nil
}
