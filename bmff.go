package heifmeta

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrReachMaxItem   = errors.New("reach max item")
	ErrInvalidVersion = errors.New("invalid version")
	MaxItem           = uint32(1000)
	MaxChildren       = uint32(150)
)

func InitHeifLimit(maxItem, maxChildren uint32) {
	MaxItem = maxItem
	MaxChildren = maxChildren
}

func NewReader(r io.Reader) *Reader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &Reader{br: bufReader{Reader: br}}
}

type Reader struct {
	br          bufReader
	lastBox     Box
	noMoreBoxes bool // a box with size 0 (the final box) was seen
}

type BoxType [4]byte

// Common box types.
var (
	TypeFtyp = BoxType{'f', 't', 'y', 'p'}
	TypeMeta = BoxType{'m', 'e', 't', 'a'}
	TypeInfe = BoxType{'i', 'n', 'f', 'e'}
)

func (t BoxType) String() string { return string(t[:]) }

func (t BoxType) EqualString(s string) bool {
	// Could be cleaner, but see https://github.com/golang/go/issues/24765.
	return len(s) == 4 && s[0] == t[0] && s[1] == t[1] && s[2] == t[2] && s[3] == t[3]
}

// Box represents a BMFF box.
type Box interface {
	Size() uint64 // 0 means unknown (will read to end of file)
	Type() BoxType

	// Parses parses the box, populating the fields
	// in the returned concrete type.
	//
	// If Parse has already been called, Parse returns nil.
	// If the box type is unknown, the returned error is ErrUnknownBox
	// and it's guaranteed that no bytes have been read from the box.
	Parse() (Box, error)

	// Body returns the child bytes of the box, ignoring the header.
	// The body may start with the 4 byte header of a "Full Box" if the
	// box's type derives from a full box. Most users will use Parse
	// instead.
	// Body will return a new reader at the beginning of the box if the
	// outer box has already been parsed.
	Body() io.Reader
}

// ErrUnknownBox is returned by Box.Parse for unrecognized box types.
var ErrUnknownBox = errors.New("heif: unknown box")

type parserFunc func(b *box, br *bufReader) (Box, error)

func boxType(s string) BoxType {
	if len(s) != 4 {
		panic("bogus boxType length")
	}
	return BoxType{s[0], s[1], s[2], s[3]}
}

var parsers = map[BoxType]parserFunc{
	boxType("iref"): parseItemReferenceBox,
	boxType("ftyp"): parseFileTypeBox,
	boxType("iinf"): parseItemInfoBox,
	boxType("infe"): parseItemInfoEntry,
	boxType("ipco"): parseItemPropertyContainerBox,
	boxType("ipma"): parseItemPropertyAssociation,
	boxType("iprp"): parseItemPropertiesBox,
	boxType("irot"): parseImageRotation,
	boxType("ispe"): parseImageSpatialExtentsProperty,
	boxType("meta"): parseMetaBox,
	boxType("pitm"): parsePrimaryItemBox,
}

type box struct {
	headerSize uint64 // 8 or 16 bytes
	size       uint64 // 0 means unknown, will read to end of file (box container)
	boxType    BoxType
	body       io.Reader
	parsed     Box    // if non-nil, the Parsed result
	slurp      []byte // if non-nil, the contents slurped to memory
}

func (b *box) Size() uint64  { return b.size }
func (b *box) Type() BoxType { return b.boxType }

func (b *box) Body() io.Reader {
	if b.slurp != nil {
		return bytes.NewReader(b.slurp)
	}
	return b.body
}

func (b *box) Parse() (Box, error) {
	if b.parsed != nil {
		return b.parsed, nil
	}
	parser, ok := parsers[b.Type()]
	if !ok {
		return nil, ErrUnknownBox
	}
	// Use a fresh reader so parsing a child box does not disturb the parent reader.
	v, err := parser(b, &bufReader{Reader: bufio.NewReader(b.Body())})
	if err != nil {
		return nil, err
	}
	b.parsed = v
	return v, nil
}

type FullBox struct {
	*box
	Version uint8
	Flags   uint32 // 24 bits
}

// ReadBox reads the next box.
//
// If the previously read box was not read to completion, ReadBox consumes
// the rest of its data.
//
// At the end, the error is io.EOF.
func (r *Reader) ReadBox() (Box, error) {
	if r.noMoreBoxes {
		return nil, io.EOF
	}
	if r.lastBox != nil {
		if _, err := io.Copy(io.Discard, r.lastBox.Body()); err != nil {
			return nil, err
		}
	}
	var buf [8]byte

	_, err := io.ReadFull(r.br, buf[:4])
	if err != nil {
		return nil, err
	}
	box := &box{
		size:       uint64(binary.BigEndian.Uint32(buf[:4])),
		headerSize: 8,
	}

	_, err = io.ReadFull(r.br, box.boxType[:]) // 4 more bytes for the type
	if err != nil {
		return nil, err
	}

	var remain uint64
	switch box.size {
	case 1:
		// A size value of 1 means the actual box size is stored in the next 64 bits.
		_, err = io.ReadFull(r.br, buf[:8])
		if err != nil {
			return nil, err
		}
		box.size = uint64(binary.BigEndian.Uint64(buf[:8]))
		box.headerSize += 8
	case 0:
		// 0 means unknown & to read to end of file. No more boxes.
		r.noMoreBoxes = true
	default:
	}
	if !r.noMoreBoxes {
		if box.size < box.headerSize {
			return nil, fmt.Errorf("box header for %q has size %d, smaller than header size %d", box.boxType, box.size, box.headerSize)
		}
		remain = box.size - box.headerSize
	}
	box.body = io.LimitReader(r.br, int64(remain))
	r.lastBox = box
	return box, nil
}

// ReadAndParseBox wraps the ReadBox method, ensuring that the read box is of type typ
// and parses successfully. It returns the parsed box.
func (r *Reader) ReadAndParseBox(typ BoxType, jumpOtherBox bool) (Box, error) {
	box, err := r.ReadBox()
	if err != nil {
		return nil, fmt.Errorf("error reading %q box: %w", typ, err)
	}
	if box.Type() != typ {
		if !jumpOtherBox {
			return nil, fmt.Errorf("error reading %q box: got box type %q instead", typ, box.Type())
		}
		for {
			box, err = r.ReadBox()
			if err != nil {
				return nil, fmt.Errorf("error reading %q box: %w", typ, err)
			}
			if box.Type() == typ {
				break
			}
		}
	}

	pbox, err := box.Parse()
	if err != nil {
		return nil, fmt.Errorf("error parsing read %q box: %w", typ, err)
	}
	return pbox, nil
}

func readFullBox(outer *box, br *bufReader) (fb FullBox, err error) {
	fb.box = outer
	// Parse FullBox header.
	buf, err := br.Peek(4)
	if err != nil {
		return FullBox{}, fmt.Errorf("failed to read 4 bytes of FullBox: %w", err)
	}
	fb.Version = buf[0]
	buf[0] = 0
	fb.Flags = binary.BigEndian.Uint32(buf[:4])
	_, _ = br.Discard(4)
	return fb, nil
}

type FileTypeBox struct {
	*box
	MajorBrand   string   // 4 bytes
	MinorVersion string   // 4 bytes
	Compatible   []string // all 4 bytes
}

func parseFileTypeBox(outer *box, br *bufReader) (Box, error) {
	buf, err := br.Peek(8)
	if err != nil {
		return nil, err
	}
	ft := &FileTypeBox{
		box:          outer,
		MajorBrand:   string(buf[:4]),
		MinorVersion: string(buf[4:8]),
	}
	_, err = br.Discard(8)
	if err != nil {
		return nil, err
	}
	for {
		buf, err := br.Peek(4)
		if err == io.EOF {
			return ft, nil
		}
		if err != nil {
			return nil, err
		}
		ft.Compatible = append(ft.Compatible, string(buf[:4]))
		_, _ = br.Discard(4)
	}
}

type MetaBox struct {
	FullBox
	Children []Box
}

func parseMetaBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	mb := &MetaBox{FullBox: fb}
	br.parseChildBoxes(&mb.Children)
	return mb, br.err
}

func (br *bufReader) parseChildBoxes(dst *[]Box) {
	if br.err != nil {
		return
	}
	boxr := NewReader(br.Reader)
	childrenNum := uint32(0)
	for {
		// Limit child boxes except for infe entries, which are already limited by iinf.
		if childrenNum > MaxChildren && boxr.lastBox != nil && boxr.lastBox.Type() != TypeInfe {
			br.err = fmt.Errorf("box type: %s over %d: %w", boxr.lastBox.Type(), MaxChildren, ErrReachMaxItem)
			return
		}
		child, err := boxr.ReadBox()
		if err == io.EOF {
			return
		}
		if err != nil {
			br.err = err
			return
		}
		slurp, err := io.ReadAll(child.Body())
		if err != nil {
			br.err = err
			return
		}
		child.(*box).slurp = slurp
		*dst = append(*dst, child)
		childrenNum++
	}
}

// ItemInfoEntry represents an "infe" box.
type ItemInfoEntry struct {
	FullBox

	ItemID          uint32
	ProtectionIndex uint16
	ItemType        string
	IsImage         bool
	Hidden          bool

	Name string

	// If Type == "mime":
	ContentType     string
	ContentEncoding string

	// If Type == "uri ":
	ItemURIType string
}

// parseItemInfoEntry parses an infe box.
/*
 *                     version <= 1    version 2   version > 2    mime     uri
 * -----------------------------------------------------------------------------------------------
 * item id               16               16           32          16/32   16/32
 * protection index      16               16           16          16      16
 * item type             -                yes          yes         yes     yes
 * item name             yes              yes          yes         yes     yes
 * content type          yes              -            -           yes     -
 * content encoding      yes              -            -           yes     -
 * hidden item           -                yes          yes         yes     yes
 * item uri type         -                -            -           -       yes
 *
 * Note: HEIF does not allow version 0 and version 1 boxes ! (see 23008-12, 10.2.1)
 */
func parseItemInfoEntry(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ie := &ItemInfoEntry{FullBox: fb}
	ie.IsImage = false
	if fb.Version != 2 && fb.Version != 3 {
		return nil, fmt.Errorf("infe box: %w", ErrInvalidVersion)
	}
	if fb.Flags&1 == 1 {
		ie.Hidden = true
	}

	if fb.Version == 2 {
		itemID, _ := br.readUint16()
		ie.ItemID = uint32(itemID)
	} else {
		ie.ItemID, _ = br.readUint32()
	}
	ie.ProtectionIndex, _ = br.readUint16()
	if !br.ok() {
		return nil, br.err
	}
	buf, err := br.Peek(4)
	if err != nil {
		return nil, err
	}
	ie.ItemType = string(buf[:4])
	_, err = br.Discard(4)
	if err != nil {
		return nil, err
	}
	ie.Name, _ = br.readString()

	switch ie.ItemType {
	case "mime":
		ie.ContentType, _ = br.readString()
		if br.anyRemain() {
			ie.ContentEncoding, _ = br.readString()
		}
	case "uri ":
		ie.ItemURIType, _ = br.readString()
	case "jpeg", "hvc1", "av01", "vvc1", "avc1", "unci", "j2k1", "mski", "grid", "iovl", "iden", "tili":
		ie.IsImage = true
	}
	if !br.ok() {
		return nil, br.err
	}
	return ie, nil
}

// ItemInfoBox represents an "iinf" box.
type ItemInfoBox struct {
	FullBox
	EntryCount uint32
	ItemInfos  []*ItemInfoEntry
}

// parseItemInfoBox parses an "iinf" box. It contains one or more "infe" boxes,
// each describing an item with fields such as item ID, item type, and item name.
func parseItemInfoBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	if fb.Version > 2 {
		return nil, fmt.Errorf("iinf box version %d: %w", fb.Version, ErrInvalidVersion)
	}
	ib := &ItemInfoBox{FullBox: fb}

	if fb.Version > 0 {
		ib.EntryCount, _ = br.readUint32()
	} else {
		entryCount, _ := br.readUint16()
		ib.EntryCount = uint32(entryCount)
	}
	if ib.EntryCount == 0 {
		return ib, errors.New("iinf box with 0 entries")
	}

	if ib.EntryCount > MaxItem {
		return ib, fmt.Errorf("iinf box: %d, max item: %d: %w", ib.EntryCount, MaxItem, ErrReachMaxItem)
	}

	var itemInfos []Box
	br.parseChildBoxes(&itemInfos)
	if !br.ok() {
		return FullBox{}, br.err
	}
	for _, box := range itemInfos {
		pb, err := box.Parse()
		if err != nil {
			return nil, fmt.Errorf("error parsing infe box: %w", err)
		}
		if iie, ok := pb.(*ItemInfoEntry); ok {
			ib.ItemInfos = append(ib.ItemInfos, iie)
		}
	}
	if len(ib.ItemInfos) != int(ib.EntryCount) {
		return nil, fmt.Errorf("expected %d infe boxes, got %d", ib.EntryCount, len(ib.ItemInfos))
	}
	return ib, nil
}

// bufReader adds some HEIF/BMFF-specific methods around a *bufio.Reader.
type bufReader struct {
	*bufio.Reader
	err error // sticky error
}

// ok reports whether all previous reads have been error-free.
func (br *bufReader) ok() bool { return br.err == nil }

func (br *bufReader) anyRemain() bool {
	if br.err != nil {
		return false
	}
	_, err := br.Peek(1)
	return err == nil
}

func (br *bufReader) readUint8() (uint8, error) {
	if br.err != nil {
		return 0, br.err
	}
	v, err := br.ReadByte()
	if err != nil {
		br.err = err
		return 0, err
	}
	return v, nil
}

func (br *bufReader) readUint16() (uint16, error) {
	if br.err != nil {
		return 0, br.err
	}
	buf, err := br.Peek(2)
	if err != nil {
		br.err = err
		return 0, err
	}
	v := binary.BigEndian.Uint16(buf[:2])
	_, err = br.Discard(2)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (br *bufReader) readUint32() (uint32, error) {
	if br.err != nil {
		return 0, br.err
	}
	buf, err := br.Peek(4)
	if err != nil {
		br.err = err
		return 0, err
	}
	v := binary.BigEndian.Uint32(buf[:4])
	_, _ = br.Discard(4)
	return v, nil
}

func (br *bufReader) readString() (string, error) {
	if br.err != nil {
		return "", br.err
	}
	s0, err := br.ReadString(0)
	if err != nil {
		br.err = err
		return "", br.err
	}
	s := strings.TrimSuffix(s0, "\x00")
	if len(s) == len(s0) {
		err = errors.New("unexpected non-null terminated string")
		br.err = err
		return "", br.err
	}
	return s, nil
}

type ItemPropertyContainerBox struct {
	*box
	AllProperties    []Box
	ParsedProperties []Box // ParsedProperties[i] is the parsed version of AllProperties[i], or nil if not yet parsed, for now only ipse and irot are parsed
}

func parseItemPropertyContainerBox(outer *box, br *bufReader) (Box, error) {
	ipc := &ItemPropertyContainerBox{box: outer}
	br.parseChildBoxes(&ipc.AllProperties)
	if !br.ok() {
		return FullBox{}, br.err
	}
	for _, b := range ipc.AllProperties {
		pb, err := b.Parse()
		if err == ErrUnknownBox {
			ipc.ParsedProperties = append(ipc.ParsedProperties, nil)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q box in ipco box: %w", b.Type(), err)
		}
		ipc.ParsedProperties = append(ipc.ParsedProperties, pb)
	}
	return ipc, nil
}

// HEIF: iprp
type ItemPropertiesBox struct {
	*box
	PropertyContainer *ItemPropertyContainerBox
	Associations      []*ItemPropertyAssociation
}

// parseItemPropertiesBox parses an iprp box containing one ipco box and one or
// more ipma boxes.
func parseItemPropertiesBox(outer *box, br *bufReader) (Box, error) {
	ip := &ItemPropertiesBox{
		box: outer,
	}

	var boxes []Box
	br.parseChildBoxes(&boxes)
	if !br.ok() {
		return FullBox{}, br.err
	}
	if len(boxes) < 2 {
		return nil, fmt.Errorf("expect at least 2 boxes in children; got %d", len(boxes))
	}

	cb, err := boxes[0].Parse()
	if err != nil {
		return FullBox{}, fmt.Errorf("failed to parse first box, %q: %w", boxes[0].Type(), err)
	}

	var ok bool
	ip.PropertyContainer, ok = cb.(*ItemPropertyContainerBox)
	if !ok {
		return FullBox{}, fmt.Errorf("unexpected box %q instead of ItemPropertyContainerBox", cb.Type())
	}

	// Association boxes
	ip.Associations = make([]*ItemPropertyAssociation, 0, len(boxes)-1)
	for _, box := range boxes[1:] {
		boxp, err := box.Parse()
		if err != nil {
			return FullBox{}, fmt.Errorf("failed to parse association box: %w", err)
		}
		ipa, ok := boxp.(*ItemPropertyAssociation)
		if !ok {
			return FullBox{}, fmt.Errorf("unexpected box %q instead of ItemPropertyAssociation", box.Type())
		}
		ip.Associations = append(ip.Associations, ipa)
	}
	return ip, nil
}

// ItemPropertyAssociation represents an "ipma" box.
type ItemPropertyAssociation struct {
	FullBox
	EntryCount uint32
	Entries    []ItemPropertyAssociationItem
}

// ItemProperty describes one property association.
type ItemProperty struct {
	Essential bool
	Index     uint16 // start from 1
}

// ItemPropertyAssociationItem describes one ipma item association.
type ItemPropertyAssociationItem struct {
	ItemID            uint32
	AssociationsCount int            // as declared
	Associations      []ItemProperty // as parsed
}

func parseItemPropertyAssociation(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ipa := &ItemPropertyAssociation{FullBox: fb}
	count, _ := br.readUint32()
	ipa.EntryCount = count

	for i := uint64(0); i < uint64(count) && br.ok(); i++ {
		var itemID uint32
		if fb.Version < 1 {
			itemID16, _ := br.readUint16()
			itemID = uint32(itemID16)
		} else {
			itemID, _ = br.readUint32()
		}
		assocCount, _ := br.readUint8()
		ipai := ItemPropertyAssociationItem{
			ItemID:            itemID,
			AssociationsCount: int(assocCount),
		}
		for j := 0; j < int(assocCount) && br.ok(); j++ {
			first, _ := br.readUint8()
			essential := first&(1<<7) != 0
			first &^= byte(1 << 7)

			var index uint16
			if fb.Flags&1 != 0 {
				second, _ := br.readUint8()
				index = uint16(first)<<8 | uint16(second)
			} else {
				index = uint16(first)
			}
			ipai.Associations = append(ipai.Associations, ItemProperty{
				Essential: essential,
				Index:     index,
			})
		}
		ipa.Entries = append(ipa.Entries, ipai)
	}
	if !br.ok() {
		return nil, br.err
	}
	return ipa, nil
}

type ImageSpatialExtentsProperty struct {
	FullBox
	ImageWidth  uint32
	ImageHeight uint32
}

func parseImageSpatialExtentsProperty(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	w, err := br.readUint32()
	if err != nil {
		return nil, err
	}
	h, err := br.readUint32()
	if err != nil {
		return nil, err
	}
	return &ImageSpatialExtentsProperty{
		FullBox:     fb,
		ImageWidth:  w,
		ImageHeight: h,
	}, nil
}

type itemReferenceEntry struct {
	Box
	typ   string   // type e.g. "dimg", "thmb", "auxl"
	from  uint32   // from item ID - infe box itemID
	to    []uint32 // to item IDs
	nRefs uint16
}
type ItemReferenceBox struct {
	FullBox
	entrys []*itemReferenceEntry
}

// parseItemReferenceBox parses an iref box. Each reference entry has a source
// item ID and one or more target item IDs. For example:
// type="dimg", from=1, to=[2,3,4] means item 1 is derived from items 2, 3, and 4.
// type="thmb", from=5, to=[1] means item 5 is a thumbnail for item 1.
// type="auxl", from=6, to=[1] means item 6 is auxiliary data for item 1.
func parseItemReferenceBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return FullBox{}, err
	}
	if fb.Version > 1 {
		return nil, fmt.Errorf("iref box: %w", ErrInvalidVersion)
	}
	drb := &ItemReferenceBox{FullBox: fb}
	var entrys []Box
	br.parseChildBoxes(&entrys)
	if !br.ok() {
		return FullBox{}, br.err
	}
	for _, box := range entrys {
		irBox := &itemReferenceEntry{Box: box}
		irBox.typ = box.Type().String()
		br := bufReader{Reader: bufio.NewReader(box.Body())}
		//irBox := box.(*itemReferenceEntry)
		if fb.Version == 0 {
			fromId, _ := br.readUint16()
			irBox.from = uint32(fromId)
		} else {
			fromId, _ := br.readUint32()
			irBox.from = fromId
		}
		irBox.nRefs, _ = br.readUint16()
		if irBox.nRefs == 0 {
			return FullBox{}, errors.New("iref box with 0 references")
		}
		for i := uint16(0); i < irBox.nRefs; i++ {
			if fb.Version == 0 {
				toId, err := br.readUint16()
				if err != nil {
					return FullBox{}, fmt.Errorf("reading iref box to IDs: %w", err)
				}
				irBox.to = append(irBox.to, uint32(toId))
			} else {
				toId, err := br.readUint32()
				if err != nil {
					return FullBox{}, fmt.Errorf("reading iref box to IDs: %w", err)
				}
				irBox.to = append(irBox.to, toId)
			}
		}
		drb.entrys = append(drb.entrys, irBox)
	}
	return drb, nil
}

// "pitm" box
type PrimaryItemBox struct {
	FullBox
	ItemID uint16
}

func parsePrimaryItemBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return nil, err
	}
	pib := &PrimaryItemBox{FullBox: fb}
	pib.ItemID, _ = br.readUint16()
	if !br.ok() {
		return nil, br.err
	}
	return pib, nil
}

// ImageRotation is a HEIF "irot" Rotation property.
type ImageRotation struct {
	*box
	Angle uint8 // 1 means 90 degrees counter-clockwise, 2 means 180 counter-clockwise
}

func parseImageRotation(gen *box, br *bufReader) (Box, error) {
	v, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	return &ImageRotation{box: gen, Angle: v & 3}, nil
}
