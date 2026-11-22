// Package dexparser implements bounded parsing of standard little-endian DEX files.
package dexparser

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/mutf8"
)

const (
	headerSize       = 0x70
	maxEncodedDepth  = 64
	maxVariableBytes = 5
)

var supportedDEXVersions = map[string]bool{"035": true, "037": true, "038": true, "039": true, "040": true}

// ParseOptions controls recoverable validation exceptions during DEX parsing.
// Zero-value ParseOptions enforces strict integrity and metadata membership.
type ParseOptions struct {
	TolerateSignatureMismatch       bool
	TolerateOrphanAnnotationDefault bool
}

type parser struct {
	data             []byte
	entry            string
	options          ParseOptions
	file             *model.File
	class            string
	method           string
	hasCallSites     bool
	callSiteOff      uint32
	callSiteSize     uint32
	hasMethodHandles bool
	methodHandleOff  uint32
	methodHandleSize uint32
}

func Parse(data []byte, entry string) (*model.File, error) {
	return ParseWithOptions(data, entry, ParseOptions{})
}

// ParseWithOptions decodes a DEX file while applying explicit recovery
// tolerances when configured.
func ParseWithOptions(data []byte, entry string, options ParseOptions) (*model.File, error) {
	p := &parser{data: data, entry: entry, options: options}
	return p.parse()
}

func (p *parser) fail(off int, op, format string, args ...any) error {
	return report.New(p.entry, p.class, p.method, int64(off), op, fmt.Errorf(format, args...))
}

func (p *parser) parse() (*model.File, error) {
	if len(p.data) < headerSize {
		return nil, p.fail(0, "header", "truncated: have %d bytes, need %d", len(p.data), headerSize)
	}
	if string(p.data[:4]) != "dex\n" || p.data[7] != 0 || p.data[4] < '0' || p.data[4] > '9' || p.data[5] < '0' || p.data[5] > '9' || p.data[6] < '0' || p.data[6] > '9' {
		return nil, p.fail(0, "header", "invalid DEX magic")
	}
	version := string(p.data[4:7])
	if !supportedDEXVersions[version] {
		return nil, p.fail(4, "header", "unsupported DEX version %q; supported versions are 035, 037, 038, 039, and 040", version)
	}
	h := model.Header{
		Version:       version,
		Checksum:      binary.LittleEndian.Uint32(p.data[8:12]),
		FileSize:      binary.LittleEndian.Uint32(p.data[32:36]),
		HeaderSize:    binary.LittleEndian.Uint32(p.data[36:40]),
		EndianTag:     binary.LittleEndian.Uint32(p.data[40:44]),
		MapOffset:     binary.LittleEndian.Uint32(p.data[52:56]),
		StringIDsSize: binary.LittleEndian.Uint32(p.data[56:60]),
		StringIDsOff:  binary.LittleEndian.Uint32(p.data[60:64]),
		TypeIDsSize:   binary.LittleEndian.Uint32(p.data[64:68]),
		TypeIDsOff:    binary.LittleEndian.Uint32(p.data[68:72]),
		ProtoIDsSize:  binary.LittleEndian.Uint32(p.data[72:76]),
		ProtoIDsOff:   binary.LittleEndian.Uint32(p.data[76:80]),
		FieldIDsSize:  binary.LittleEndian.Uint32(p.data[80:84]),
		FieldIDsOff:   binary.LittleEndian.Uint32(p.data[84:88]),
		MethodIDsSize: binary.LittleEndian.Uint32(p.data[88:92]),
		MethodIDsOff:  binary.LittleEndian.Uint32(p.data[92:96]),
		ClassDefsSize: binary.LittleEndian.Uint32(p.data[96:100]),
		ClassDefsOff:  binary.LittleEndian.Uint32(p.data[100:104]),
		DataSize:      binary.LittleEndian.Uint32(p.data[104:108]),
		DataOff:       binary.LittleEndian.Uint32(p.data[108:112]),
	}
	copy(h.Signature[:], p.data[12:32])
	if h.HeaderSize != headerSize {
		return nil, p.fail(36, "header", "header_size=%d, want %d", h.HeaderSize, headerSize)
	}
	if h.EndianTag != 0x12345678 {
		return nil, p.fail(40, "header", "unsupported endian_tag %#x", h.EndianTag)
	}
	if uint64(h.FileSize) != uint64(len(p.data)) {
		return nil, p.fail(32, "header", "file_size=%d, actual=%d", h.FileSize, len(p.data))
	}
	if err := p.rangeCheck(h.DataOff, h.DataSize, "data section"); err != nil {
		return nil, err
	}
	if version != "035" && h.MapOffset == 0 {
		return nil, p.fail(52, "header", "DEX version %q requires map_list evidence for feature admission", version)
	}
	p.file = &model.File{Entry: p.entry, Header: h}
	if h.MapOffset != 0 {
		if err := p.validateMapList(h.MapOffset, version); err != nil {
			return nil, err
		}
	}
	if err := p.parseStrings(); err != nil {
		return nil, err
	}
	if err := p.parseTypes(); err != nil {
		return nil, err
	}
	if err := p.parseProtos(); err != nil {
		return nil, err
	}
	if err := p.parseFields(); err != nil {
		return nil, err
	}
	if err := p.parseMethods(); err != nil {
		return nil, err
	}
	if err := p.parseMethodHandles(); err != nil {
		return nil, err
	}
	if err := p.parseCallSites(); err != nil {
		return nil, err
	}
	if err := p.parseClasses(); err != nil {
		return nil, err
	}
	p.fillInnerClassOuters()
	if err := p.checkAnnotationMethodMembership(); err != nil {
		return nil, err
	}
	wantChecksum := adler32.Checksum(p.data[12:])
	if h.Checksum != wantChecksum {
		return nil, p.fail(8, "header checksum", "Adler-32 checksum=%08x, computed=%08x", h.Checksum, wantChecksum)
	}
	wantSignature := sha1.Sum(p.data[32:])
	if !bytes.Equal(h.Signature[:], wantSignature[:]) {
		if !p.options.TolerateSignatureMismatch {
			return nil, p.fail(12, "header signature", "SHA-1 signature mismatch")
		}
		p.file.ParseWarnings = append(p.file.ParseWarnings, model.ParseWarning{
			Entry:   p.entry,
			Code:    "header.signature_mismatch",
			Action:  "ignore_header_signature",
			Feature: "dex-header",
			Message: fmt.Sprintf("SHA-1 signature=%x, computed=%x (Adler-32 checksum valid)", h.Signature, wantSignature),
		})
	}
	return p.file, nil
}

func (p *parser) validateMapList(off uint32, version string) error {
	if err := p.rangeCheck(off, 4, "map_list"); err != nil {
		return err
	}
	count := binary.LittleEndian.Uint32(p.data[off : off+4])
	if uint64(count)*12 > math.MaxUint32 {
		return p.fail(int(off), "map_list", "item count %d overflows", count)
	}
	if err := p.rangeCheck(off+4, count*12, "map_list"); err != nil {
		return err
	}
	for i := uint32(0); i < count; i++ {
		itemOff := int(off + 4 + i*12)
		itemType := binary.LittleEndian.Uint16(p.data[itemOff : itemOff+2])
		if !knownMapItem(itemType) {
			return p.fail(itemOff, "map_list", "unsupported section type=%#04x in DEX version %q", itemType, version)
		}
		itemSize := binary.LittleEndian.Uint32(p.data[itemOff+4 : itemOff+8])
		itemDataOff := binary.LittleEndian.Uint32(p.data[itemOff+8 : itemOff+12])
		switch itemType {
		case 0x0007:
			if p.hasCallSites {
				return p.fail(itemOff, "map_list", "duplicate call_site_id section (type=0x0007)")
			}
			p.hasCallSites = true
			p.callSiteSize = itemSize
			p.callSiteOff = itemDataOff
		case 0x0008:
			if p.hasMethodHandles {
				return p.fail(itemOff, "map_list", "duplicate method_handle section (type=0x0008)")
			}
			p.hasMethodHandles = true
			p.methodHandleSize = itemSize
			p.methodHandleOff = itemDataOff
		}
	}
	return nil
}

func knownMapItem(itemType uint16) bool {
	switch itemType {
	case 0x0000, 0x0001, 0x0002, 0x0003, 0x0004, 0x0005, 0x0006, 0x0007, 0x0008,
		0x1000, 0x1001, 0x1002, 0x1003,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006,
		0xf000:
		return true
	default:
		return false
	}
}

func (p *parser) parseMethodHandles() error {
	if !p.hasMethodHandles || p.methodHandleSize == 0 {
		return nil
	}
	if err := p.tableRange(p.methodHandleOff, p.methodHandleSize, 8, "method_handles"); err != nil {
		return err
	}
	p.file.MethodHandles = make([]model.MethodHandle, p.methodHandleSize)
	for i := range p.file.MethodHandles {
		off := int(p.methodHandleOff) + i*8
		kind := binary.LittleEndian.Uint16(p.data[off : off+2])
		unused1 := binary.LittleEndian.Uint16(p.data[off+2 : off+4])
		targetIdx := binary.LittleEndian.Uint16(p.data[off+4 : off+6])
		unused2 := binary.LittleEndian.Uint16(p.data[off+6 : off+8])
		if unused1 != 0 || unused2 != 0 {
			return p.fail(off, "method_handles", "nonzero unused padding (%#x, %#x)", unused1, unused2)
		}
		if kind > uint16(model.MethodHandleInvokeInterface) {
			return p.fail(off, "method_handles", "unsupported method_handle_type %#x", kind)
		}
		mhKind := model.MethodHandleKind(kind)
		if mhKind <= model.MethodHandleInstanceGet {
			if int(targetIdx) >= len(p.file.Fields) {
				return p.fail(off+4, "method_handles", "field_id=%d out of range", targetIdx)
			}
		} else {
			if int(targetIdx) >= len(p.file.Methods) {
				return p.fail(off+4, "method_handles", "method_id=%d out of range", targetIdx)
			}
			methodName := p.file.Methods[targetIdx].Name
			if methodName == "<clinit>" {
				return p.fail(off+4, "method_handles", "method_handle cannot reference <clinit>")
			}
			if mhKind == model.MethodHandleInvokeConstructor && methodName != "<init>" {
				return p.fail(off+4, "method_handles", "constructor method_handle must target <init>, got %q", methodName)
			}
		}
		p.file.MethodHandles[i] = model.MethodHandle{
			Kind:        mhKind,
			TargetIndex: targetIdx,
		}
	}
	return nil
}

func (p *parser) parseCallSites() error {
	if !p.hasCallSites || p.callSiteSize == 0 {
		return nil
	}
	if err := p.tableRange(p.callSiteOff, p.callSiteSize, 4, "call_site_ids"); err != nil {
		return err
	}
	p.file.CallSites = make([]model.CallSite, p.callSiteSize)
	for i := range p.file.CallSites {
		idOff := int(p.callSiteOff) + i*4
		csOff := binary.LittleEndian.Uint32(p.data[idOff : idOff+4])
		if err := p.rangeCheck(csOff, 1, "call_site"); err != nil {
			return p.fail(idOff, "call_site_ids", "%v", err)
		}
		cur := int(csOff)
		arr, err := p.encodedArray(&cur, 0)
		if err != nil {
			return err
		}
		if len(arr) < 3 {
			return p.fail(int(csOff), "call_site", "call site array has %d entries; need at least 3", len(arr))
		}
		if arr[0].Type != model.ValueMethodHandle || arr[0].Unsigned >= uint64(len(p.file.MethodHandles)) {
			return p.fail(int(csOff), "call_site", "invalid bootstrap method handle element type=%#x index=%d", arr[0].Type, arr[0].Unsigned)
		}
		if arr[1].Type != model.ValueString || arr[1].Unsigned >= uint64(len(p.file.Strings)) {
			return p.fail(int(csOff), "call_site", "invalid call site method name element type=%#x index=%d", arr[1].Type, arr[1].Unsigned)
		}
		if arr[2].Type != model.ValueMethodType || arr[2].Unsigned >= uint64(len(p.file.Protos)) {
			return p.fail(int(csOff), "call_site", "invalid call site method type element type=%#x index=%d", arr[2].Type, arr[2].Unsigned)
		}
		nameIdx := uint32(arr[1].Unsigned)
		var extraArgs []model.EncodedValue
		if len(arr) > 3 {
			extraArgs = append([]model.EncodedValue(nil), arr[3:]...)
		}
		p.file.CallSites[i] = model.CallSite{
			BootstrapMethodHandle: uint32(arr[0].Unsigned),
			MethodNameIndex:       nameIdx,
			MethodName:            p.file.Strings[nameIdx],
			MethodTypeIndex:       uint32(arr[2].Unsigned),
			Arguments:             extraArgs,
		}
	}
	return nil
}

func (p *parser) rangeCheck(off, size uint32, what string) error {
	end := uint64(off) + uint64(size)
	if end > uint64(len(p.data)) {
		return p.fail(int(off), what, "range [%#x,%#x) exceeds file size %#x", off, end, len(p.data))
	}
	return nil
}

func (p *parser) tableRange(off, count, width uint32, what string) error {
	if count == 0 {
		if off != 0 {
			return p.fail(int(off), what, "nonzero offset for empty table")
		}
		return nil
	}
	if off == 0 {
		return p.fail(0, what, "zero offset for %d items", count)
	}
	size := uint64(count) * uint64(width)
	if size > math.MaxUint32 {
		return p.fail(int(off), what, "table byte size overflows uint32")
	}
	return p.rangeCheck(off, uint32(size), what)
}

func (p *parser) parseStrings() error {
	h := p.file.Header
	if err := p.tableRange(h.StringIDsOff, h.StringIDsSize, 4, "string_ids"); err != nil {
		return err
	}
	p.file.Strings = make([]string, h.StringIDsSize)
	for i := range p.file.Strings {
		idOff := int(h.StringIDsOff) + i*4
		dataOff := binary.LittleEndian.Uint32(p.data[idOff : idOff+4])
		s, err := p.stringData(dataOff)
		if err != nil {
			return err
		}
		p.file.Strings[i] = s
	}
	return nil
}

func (p *parser) stringData(off uint32) (string, error) {
	cur := int(off)
	units, err := p.uleb(&cur)
	if err != nil {
		return "", p.fail(int(off), "string_data", "%v", err)
	}
	start := cur
	for cur < len(p.data) && p.data[cur] != 0 {
		cur++
	}
	if cur >= len(p.data) {
		return "", p.fail(start, "string_data", "missing NUL terminator")
	}
	decoded, err := mutf8.DecodeBounded(p.data[start:cur], int(units))
	if err != nil {
		return "", p.fail(start, "string_data", "%v", err)
	}
	if uint32(len(decoded)) != units {
		return "", p.fail(int(off), "string_data", "utf16_size=%d, decoded=%d", units, len(decoded))
	}
	if unit, kind, ok := unpairedSurrogate(decoded); ok {
		return "", p.fail(start+mutf8UnitOffset(p.data[start:cur], unit), "string_data", "unpaired %s surrogate at UTF-16 unit %d", kind, unit)
	}
	return string(utf16.Decode(decoded)), nil
}

func unpairedSurrogate(units []uint16) (int, string, bool) {
	for i := 0; i < len(units); i++ {
		switch {
		case units[i] >= 0xd800 && units[i] <= 0xdbff:
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return i, "high", true
			}
			i++
		case units[i] >= 0xdc00 && units[i] <= 0xdfff:
			return i, "low", true
		}
	}
	return 0, "", false
}

func mutf8UnitOffset(raw []byte, unit int) int {
	off := 0
	for i := 0; i < unit; i++ {
		switch {
		case raw[off] < 0x80:
			off++
		case raw[off] < 0xe0:
			off += 2
		default:
			off += 3
		}
	}
	return off
}

func (p *parser) parseTypes() error {
	h := p.file.Header
	if err := p.tableRange(h.TypeIDsOff, h.TypeIDsSize, 4, "type_ids"); err != nil {
		return err
	}
	p.file.Types = make([]model.Type, h.TypeIDsSize)
	for i := range p.file.Types {
		off := int(h.TypeIDsOff) + i*4
		idx := binary.LittleEndian.Uint32(p.data[off : off+4])
		if idx >= uint32(len(p.file.Strings)) {
			return p.fail(off, "type_ids", "descriptor_idx=%d out of range", idx)
		}
		p.file.Types[i] = model.Type{DescriptorIndex: idx, Descriptor: p.file.Strings[idx]}
	}
	return nil
}

func (p *parser) parseProtos() error {
	h := p.file.Header
	if err := p.tableRange(h.ProtoIDsOff, h.ProtoIDsSize, 12, "proto_ids"); err != nil {
		return err
	}
	p.file.Protos = make([]model.Proto, h.ProtoIDsSize)
	for i := range p.file.Protos {
		off := int(h.ProtoIDsOff) + i*12
		shorty := binary.LittleEndian.Uint32(p.data[off : off+4])
		ret := binary.LittleEndian.Uint32(p.data[off+4 : off+8])
		paramsOff := binary.LittleEndian.Uint32(p.data[off+8 : off+12])
		if shorty >= uint32(len(p.file.Strings)) || ret >= uint32(len(p.file.Types)) {
			return p.fail(off, "proto_ids", "invalid shorty_idx=%d or return_type_idx=%d", shorty, ret)
		}
		indices, names, err := p.typeList(paramsOff)
		if err != nil {
			return err
		}
		shortyValue := p.file.Strings[shorty]
		wantShorty := protoShorty(p.file.Types[ret].Descriptor, names)
		if shortyValue != wantShorty {
			return p.fail(off, "proto_ids", "shorty %q does not match prototype; want %q", shortyValue, wantShorty)
		}
		p.file.Protos[i] = model.Proto{ShortyIndex: shorty, Shorty: shortyValue, ReturnTypeIndex: ret, ReturnType: p.file.Types[ret].Descriptor, ParameterTypes: indices, Parameters: names}
	}
	return nil
}

func protoShorty(returnType string, parameters []string) string {
	var out strings.Builder
	out.WriteByte(shortyType(returnType))
	for _, parameter := range parameters {
		out.WriteByte(shortyType(parameter))
	}
	return out.String()
}

func shortyType(descriptor string) byte {
	if descriptor == "" {
		return 0
	}
	if descriptor[0] == 'L' || descriptor[0] == '[' {
		return 'L'
	}
	return descriptor[0]
}

func (p *parser) typeList(off uint32) ([]uint32, []string, error) {
	if off == 0 {
		return nil, nil, nil
	}
	if err := p.rangeCheck(off, 4, "type_list"); err != nil {
		return nil, nil, err
	}
	count := binary.LittleEndian.Uint32(p.data[int(off) : int(off)+4])
	if uint64(count)*2 > math.MaxUint32 {
		return nil, nil, p.fail(int(off), "type_list", "size overflows")
	}
	if uint64(off)+4 > math.MaxUint32 {
		return nil, nil, p.fail(int(off), "type_list", "offset overflows")
	}
	if err := p.rangeCheck(off+4, count*2, "type_list"); err != nil {
		return nil, nil, err
	}
	indices := make([]uint32, count)
	names := make([]string, count)
	for i := range indices {
		pos := int(off) + 4 + i*2
		idx := uint32(binary.LittleEndian.Uint16(p.data[pos : pos+2]))
		if idx >= uint32(len(p.file.Types)) {
			return nil, nil, p.fail(pos, "type_list", "type_idx=%d out of range", idx)
		}
		indices[i], names[i] = idx, p.file.Types[idx].Descriptor
	}
	return indices, names, nil
}

func (p *parser) parseFields() error {
	h := p.file.Header
	if err := p.tableRange(h.FieldIDsOff, h.FieldIDsSize, 8, "field_ids"); err != nil {
		return err
	}
	p.file.Fields = make([]model.FieldID, h.FieldIDsSize)
	for i := range p.file.Fields {
		off := int(h.FieldIDsOff) + i*8
		classIdx := binary.LittleEndian.Uint16(p.data[off : off+2])
		typeIdx := binary.LittleEndian.Uint16(p.data[off+2 : off+4])
		nameIdx := binary.LittleEndian.Uint32(p.data[off+4 : off+8])
		if uint32(classIdx) >= uint32(len(p.file.Types)) || uint32(typeIdx) >= uint32(len(p.file.Types)) || nameIdx >= uint32(len(p.file.Strings)) {
			return p.fail(off, "field_ids", "invalid class_idx=%d type_idx=%d name_idx=%d", classIdx, typeIdx, nameIdx)
		}
		p.file.Fields[i] = model.FieldID{ClassIndex: classIdx, TypeIndex: typeIdx, NameIndex: nameIdx, Class: p.file.Types[classIdx].Descriptor, Type: p.file.Types[typeIdx].Descriptor, Name: p.file.Strings[nameIdx]}
	}
	return nil
}

func (p *parser) parseMethods() error {
	h := p.file.Header
	if err := p.tableRange(h.MethodIDsOff, h.MethodIDsSize, 8, "method_ids"); err != nil {
		return err
	}
	p.file.Methods = make([]model.MethodID, h.MethodIDsSize)
	for i := range p.file.Methods {
		off := int(h.MethodIDsOff) + i*8
		classIdx := binary.LittleEndian.Uint16(p.data[off : off+2])
		protoIdx := binary.LittleEndian.Uint16(p.data[off+2 : off+4])
		nameIdx := binary.LittleEndian.Uint32(p.data[off+4 : off+8])
		if uint32(classIdx) >= uint32(len(p.file.Types)) || uint32(protoIdx) >= uint32(len(p.file.Protos)) || nameIdx >= uint32(len(p.file.Strings)) {
			return p.fail(off, "method_ids", "invalid class_idx=%d proto_idx=%d name_idx=%d", classIdx, protoIdx, nameIdx)
		}
		p.file.Methods[i] = model.MethodID{ClassIndex: classIdx, ProtoIndex: protoIdx, NameIndex: nameIdx, Class: p.file.Types[classIdx].Descriptor, Name: p.file.Strings[nameIdx]}
	}
	return nil
}
