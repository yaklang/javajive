// Package jimage reads the JDK modular runtime image format used by
// $JAVA_HOME/lib/modules and .jimage files. It exposes the image resources as
// a deterministic JAR view so the Java decompiler can consume JDK 9+ runtime
// images without an external jimage tool.
package jimage

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	imageMagic          = 0xCAFEDADA
	resourceHeaderMagic = 0xCAFEFAFA
	headerSize          = 28
	resourceHeaderSize  = 29

	// maxTableLength keeps a malicious header from allocating unbounded index
	// tables. Real JDK images have tens of thousands of resources.
	maxTableLength = 1 << 24

	defaultMaxResourceSize = 256 << 20

	attributeModule       = 1
	attributeParent       = 2
	attributeBase         = 3
	attributeExtension    = 4
	attributeOffset       = 5
	attributeCompressed   = 6
	attributeUncompressed = 7
	attributePreviewFlags = 8
	attributeCount        = 9
	modulesModuleName     = "modules"
	packagesModuleName    = "packages"
	maxDecompressorLayers = 16
)

var archiveTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// ErrUnsupportedCompressor reports a resource compressed with a decompressor
// this package does not implement. Compressed JDK images produced by jlink
// normally use "zip"; "compact-cp" is a class-file constant-pool transform.
var ErrUnsupportedCompressor = errors.New("jimage: unsupported compressor")

// UnsupportedCompressorError identifies the resource and compressor name.
type UnsupportedCompressorError struct {
	Entry      string
	Compressor string
}

func (e *UnsupportedCompressorError) Error() string {
	if e == nil {
		return ErrUnsupportedCompressor.Error()
	}
	return fmt.Sprintf("jimage: %q uses unsupported compressor %q", e.Entry, e.Compressor)
}

func (e *UnsupportedCompressorError) Unwrap() error { return ErrUnsupportedCompressor }

// Header is the fixed 28-byte image header. The version packs a major version
// in the high 16 bits and a minor version in the low 16 bits.
type Header struct {
	Magic         uint32
	Version       uint32
	Flags         uint32
	ResourceCount uint32
	TableLength   uint32
	LocationsSize uint32
	StringsSize   uint32
}

// Major is the image format major version.
func (h Header) Major() uint16 { return uint16(h.Version >> 16) }

// Minor is the image format minor version.
func (h Header) Minor() uint16 { return uint16(h.Version) }

// Entry is one resource location in a JImage.
type Entry struct {
	// Path is the full JImage path, for example
	// "/java.base/java/lang/Object.class".
	Path      string
	Module    string
	Parent    string
	Base      string
	Extension string

	Offset           uint64
	CompressedSize   uint64
	UncompressedSize uint64
	PreviewFlags     uint32

	// Directory marks the synthetic /modules and /packages namespace entries.
	Directory bool
}

// Name returns Path without the leading slash.
func (e Entry) Name() string { return strings.TrimPrefix(e.Path, "/") }

// Options controls resource bounds. The zero value uses defaults.
type Options struct {
	MaxResourceSize uint64
}

func (o Options) maxResourceSize() uint64 {
	if o.MaxResourceSize == 0 {
		return defaultMaxResourceSize
	}
	return o.MaxResourceSize
}

// Reader is a random-access JImage reader. The index tables are held in memory;
// resource bytes are read on demand through the underlying io.ReaderAt.
type Reader struct {
	r     io.ReaderAt
	size  int64
	close io.Closer

	endian  binary.ByteOrder
	header  Header
	offsets []uint32

	locations []byte
	strings   []byte
	indexSize uint64

	entries []Entry
	byName  map[string]int

	maxResourceSize uint64
}

// OpenFile opens a JImage file and keeps the file handle until Close.
func OpenFile(name string) (*Reader, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	reader, err := Open(file, info.Size(), Options{})
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	reader.close = file
	return reader, nil
}

// OpenBytes opens a JImage held in memory.
func OpenBytes(data []byte) (*Reader, error) {
	return Open(bytes.NewReader(data), int64(len(data)), Options{})
}

// Open reads the JImage header and index from r. The reader must remain valid
// until the returned Reader is no longer used.
func Open(r io.ReaderAt, size int64, options Options) (*Reader, error) {
	if r == nil {
		return nil, errors.New("jimage: nil reader")
	}
	if size < headerSize {
		return nil, fmt.Errorf("jimage: input size %d is smaller than the %d-byte header", size, headerSize)
	}

	var raw [headerSize]byte
	if _, err := r.ReadAt(raw[:], 0); err != nil {
		return nil, fmt.Errorf("jimage: read header: %w", err)
	}

	var endian binary.ByteOrder
	switch {
	case binary.LittleEndian.Uint32(raw[0:4]) == imageMagic:
		endian = binary.LittleEndian
	case binary.BigEndian.Uint32(raw[0:4]) == imageMagic:
		endian = binary.BigEndian
	default:
		return nil, fmt.Errorf("jimage: bad magic 0x%08x", endianOrLittle(raw[0:4]))
	}

	header := Header{
		Magic:         endian.Uint32(raw[0:4]),
		Version:       endian.Uint32(raw[4:8]),
		Flags:         endian.Uint32(raw[8:12]),
		ResourceCount: endian.Uint32(raw[12:16]),
		TableLength:   endian.Uint32(raw[16:20]),
		LocationsSize: endian.Uint32(raw[20:24]),
		StringsSize:   endian.Uint32(raw[24:28]),
	}
	if header.Major() != 1 || header.Minor() > 1 {
		return nil, fmt.Errorf("jimage: unsupported version %d.%d", header.Major(), header.Minor())
	}
	if header.TableLength > maxTableLength {
		return nil, fmt.Errorf("jimage: table length %d exceeds limit %d", header.TableLength, maxTableLength)
	}

	tableBytes := uint64(header.TableLength) * 4
	indexSize := uint64(headerSize) + tableBytes*2 + uint64(header.LocationsSize) + uint64(header.StringsSize)
	if indexSize < uint64(headerSize) || indexSize > uint64(size) {
		return nil, fmt.Errorf("jimage: index size %d exceeds input size %d", indexSize, size)
	}

	// The redirect table is used by the JDK's hash lookup. Enumeration and the
	// path-indexed API below do not need it, but it still occupies space in the
	// index before the offsets table.
	offsetsPos := int64(headerSize) + int64(tableBytes)
	offsetBytes := make([]byte, tableBytes)
	if _, err := r.ReadAt(offsetBytes, offsetsPos); err != nil {
		return nil, fmt.Errorf("jimage: read offsets table: %w", err)
	}
	offsets := make([]uint32, header.TableLength)
	for i := range offsets {
		offsets[i] = endian.Uint32(offsetBytes[i*4:])
	}

	locationsPos := offsetsPos + int64(tableBytes)
	locations := make([]byte, header.LocationsSize)
	if _, err := r.ReadAt(locations, locationsPos); err != nil {
		return nil, fmt.Errorf("jimage: read location attributes: %w", err)
	}

	stringsPos := locationsPos + int64(header.LocationsSize)
	stringsData := make([]byte, header.StringsSize)
	if _, err := r.ReadAt(stringsData, stringsPos); err != nil {
		return nil, fmt.Errorf("jimage: read strings table: %w", err)
	}

	reader := &Reader{
		r:               r,
		size:            size,
		endian:          endian,
		header:          header,
		offsets:         offsets,
		locations:       locations,
		strings:         stringsData,
		indexSize:       indexSize,
		maxResourceSize: options.maxResourceSize(),
	}
	if err := reader.buildEntries(); err != nil {
		return nil, err
	}
	return reader, nil
}

// Close releases the file handle when the reader was opened with OpenFile.
func (r *Reader) Close() error {
	if r == nil || r.close == nil {
		return nil
	}
	closer := r.close
	r.close = nil
	return closer.Close()
}

// Header returns the parsed image header.
func (r *Reader) Header() Header {
	if r == nil {
		return Header{}
	}
	return r.header
}

// Entries returns the sorted resource inventory. The returned slice must not be
// mutated by callers.
func (r *Reader) Entries() []Entry {
	if r == nil {
		return nil
	}
	return r.entries
}

// Entry looks up a resource by its JImage path. Both "/module/..." and
// "module/..." (and optional jrt-style "modules/" prefix) are accepted.
func (r *Reader) Entry(name string) (Entry, bool) {
	if r == nil {
		return Entry{}, false
	}
	index, ok := r.byName[normalizeName(name)]
	if !ok {
		return Entry{}, false
	}
	return r.entries[index], true
}

// Read returns the uncompressed bytes of one resource.
func (r *Reader) Read(name string) ([]byte, error) {
	if r == nil {
		return nil, errors.New("jimage: nil reader")
	}
	normalized := normalizeName(name)
	index, ok := r.byName[normalized]
	if !ok {
		return nil, fmt.Errorf("jimage: entry %q not found", name)
	}
	entry := r.entries[index]
	if entry.Directory {
		return nil, fmt.Errorf("jimage: entry %q is a directory", entry.Name())
	}
	return r.readEntry(entry)
}

// WriteZIP writes a deterministic JAR view of the image. Class resources are
// placed at their package paths so the Java decompiler can resolve siblings;
// module-info.class and non-class resources keep their module prefix.
func (r *Reader) WriteZIP(w io.Writer) error {
	if r == nil {
		return errors.New("jimage: nil reader")
	}
	writer := zip.NewWriter(w)
	seen := make(map[string]string, len(r.entries))
	for _, entry := range r.entries {
		if entry.Directory {
			continue
		}
		name, err := archiveName(entry)
		if err != nil {
			return err
		}
		if previous, exists := seen[name]; exists {
			return fmt.Errorf("jimage: JAR entry %q from %q collides with %q", name, entry.Name(), previous)
		}
		seen[name] = entry.Name()

		data, err := r.readEntry(entry)
		if err != nil {
			return fmt.Errorf("jimage: read %q: %w", entry.Name(), err)
		}
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetModTime(archiveTimestamp)
		header.SetMode(0o644)
		file, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return fmt.Errorf("jimage: create JAR entry %q: %w", name, err)
		}
		if _, err := file.Write(data); err != nil {
			_ = writer.Close()
			return fmt.Errorf("jimage: write JAR entry %q: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("jimage: finish JAR: %w", err)
	}
	return nil
}

// ZIPBytes is WriteZIP into memory. Prefer WriteZIP for large JDK images.
func (r *Reader) ZIPBytes() ([]byte, error) {
	var buffer bytes.Buffer
	if err := r.WriteZIP(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (r *Reader) buildEntries() error {
	if r.header.TableLength == 0 {
		r.entries = nil
		r.byName = make(map[string]int)
		return nil
	}
	seen := make(map[uint32]struct{}, r.header.ResourceCount)
	entries := make([]Entry, 0, r.header.ResourceCount)
	for _, offset := range r.offsets {
		if offset == 0 {
			continue
		}
		if _, exists := seen[offset]; exists {
			continue
		}
		seen[offset] = struct{}{}
		entry, err := r.parseEntry(offset)
		if err != nil {
			return err
		}
		if entry.Path == "" {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	r.entries = entries
	r.byName = make(map[string]int, len(entries))
	for index, entry := range entries {
		name := entry.Name()
		if _, exists := r.byName[name]; exists {
			return fmt.Errorf("jimage: duplicate entry %q", name)
		}
		r.byName[name] = index
	}
	return nil
}

func (r *Reader) parseEntry(offset uint32) (Entry, error) {
	attrs, err := r.parseAttributes(offset)
	if err != nil {
		return Entry{}, err
	}
	module, err := r.stringValue(attrs[attributeModule])
	if err != nil {
		return Entry{}, err
	}
	parent, err := r.stringValue(attrs[attributeParent])
	if err != nil {
		return Entry{}, err
	}
	base, err := r.stringValue(attrs[attributeBase])
	if err != nil {
		return Entry{}, err
	}
	extension, err := r.stringValue(attrs[attributeExtension])
	if err != nil {
		return Entry{}, err
	}

	var builder strings.Builder
	if module != "" {
		builder.WriteByte('/')
		builder.WriteString(module)
		builder.WriteByte('/')
	}
	if parent != "" {
		builder.WriteString(parent)
		builder.WriteByte('/')
	}
	builder.WriteString(base)
	if extension != "" {
		builder.WriteByte('.')
		builder.WriteString(extension)
	}

	return Entry{
		Path:             builder.String(),
		Module:           module,
		Parent:           parent,
		Base:             base,
		Extension:        extension,
		Offset:           attrs[attributeOffset],
		CompressedSize:   attrs[attributeCompressed],
		UncompressedSize: attrs[attributeUncompressed],
		PreviewFlags:     uint32(attrs[attributePreviewFlags]),
		Directory:        module == modulesModuleName || module == packagesModuleName || base == "",
	}, nil
}

func (r *Reader) parseAttributes(offset uint32) ([attributeCount]uint64, error) {
	var attrs [attributeCount]uint64
	pos := int(offset)
	if pos < 0 || pos >= len(r.locations) {
		return attrs, fmt.Errorf("jimage: location offset %d is out of range", offset)
	}
	for {
		if pos >= len(r.locations) {
			return attrs, fmt.Errorf("jimage: unterminated attribute stream at offset %d", offset)
		}
		header := r.locations[pos]
		pos++
		if header <= 7 { // ATTRIBUTE_END
			break
		}
		kind := int(header >> 3)
		if kind >= attributeCount {
			return attrs, fmt.Errorf("jimage: invalid attribute kind %d at offset %d", kind, offset)
		}
		length := int(header&0x7) + 1
		if pos+length > len(r.locations) {
			return attrs, fmt.Errorf("jimage: truncated attribute value at offset %d", offset)
		}
		var value uint64
		for i := 0; i < length; i++ {
			value = value<<8 | uint64(r.locations[pos+i])
		}
		pos += length
		attrs[kind] = value
	}
	return attrs, nil
}

func (r *Reader) stringValue(offset uint64) (string, error) {
	if offset > uint64(^uint32(0)) {
		return "", fmt.Errorf("jimage: string offset %d is out of range", offset)
	}
	return r.stringAt(uint32(offset))
}

func (r *Reader) stringAt(offset uint32) (string, error) {
	if offset == 0 {
		return "", nil
	}
	if int(offset) >= len(r.strings) {
		return "", fmt.Errorf("jimage: string offset %d is out of range", offset)
	}
	rest := r.strings[offset:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return "", fmt.Errorf("jimage: unterminated string at offset %d", offset)
	}
	return string(rest[:end]), nil
}

func (r *Reader) readEntry(entry Entry) ([]byte, error) {
	size := entry.CompressedSize
	if size == 0 {
		size = entry.UncompressedSize
	}
	if size > r.maxResourceSize {
		return nil, fmt.Errorf("jimage: resource size %d exceeds limit %d", size, r.maxResourceSize)
	}
	start := r.indexSize + entry.Offset
	if start < r.indexSize || start > uint64(r.size) {
		return nil, fmt.Errorf("jimage: resource offset %d is out of range", entry.Offset)
	}
	if size > uint64(r.size)-start {
		return nil, fmt.Errorf("jimage: resource %q exceeds image size", entry.Name())
	}
	payload := make([]byte, int(size))
	if _, err := r.r.ReadAt(payload, int64(start)); err != nil {
		return nil, fmt.Errorf("jimage: read resource %q: %w", entry.Name(), err)
	}
	if entry.CompressedSize == 0 {
		return payload, nil
	}
	return r.decompress(entry, payload)
}

func (r *Reader) decompress(entry Entry, payload []byte) ([]byte, error) {
	current := payload
	for depth := 0; ; depth++ {
		if depth > maxDecompressorLayers {
			return nil, fmt.Errorf("jimage: resource %q has too many decompressor layers", entry.Name())
		}
		if len(current) < resourceHeaderSize {
			return nil, fmt.Errorf("jimage: resource %q has a truncated decompressor header", entry.Name())
		}
		if r.endian.Uint32(current[0:4]) != resourceHeaderMagic {
			return nil, fmt.Errorf("jimage: resource %q is compressed but has no decompressor header", entry.Name())
		}
		compressedSize := r.endian.Uint64(current[4:12])
		uncompressedSize := r.endian.Uint64(current[12:20])
		compressorOffset := r.endian.Uint32(current[20:24])
		terminal := current[28]
		if compressedSize > uint64(len(current)-resourceHeaderSize) {
			return nil, fmt.Errorf("jimage: resource %q compressed payload exceeds its entry", entry.Name())
		}
		compressor, err := r.stringAt(compressorOffset)
		if err != nil {
			return nil, err
		}
		payloadStart := resourceHeaderSize
		payloadEnd := payloadStart + int(compressedSize)

		var next []byte
		switch compressor {
		case "zip":
			next, err = decompressZIP(current[payloadStart:payloadEnd], uncompressedSize)
		case "compact-cp":
			return nil, &UnsupportedCompressorError{Entry: entry.Name(), Compressor: compressor}
		default:
			return nil, fmt.Errorf("jimage: resource %q uses unknown compressor %q", entry.Name(), compressor)
		}
		if err != nil {
			return nil, fmt.Errorf("jimage: decompress %q with %s: %w", entry.Name(), compressor, err)
		}
		current = next
		if terminal != 0 {
			break
		}
	}
	if uint64(len(current)) != entry.UncompressedSize {
		return nil, fmt.Errorf("jimage: resource %q decompressed to %d bytes, want %d", entry.Name(), len(current), entry.UncompressedSize)
	}
	return current, nil
}

func decompressZIP(payload []byte, expected uint64) ([]byte, error) {
	if expected > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("jimage: inflated size %d is too large", expected)
	}
	var reader io.ReadCloser
	if zlibReader, err := zlib.NewReader(bytes.NewReader(payload)); err == nil {
		reader = zlibReader
	} else {
		reader = flate.NewReader(bytes.NewReader(payload))
	}
	defer reader.Close()
	output, err := io.ReadAll(io.LimitReader(reader, int64(expected)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(output)) != expected {
		return nil, fmt.Errorf("inflated to %d bytes, want %d", len(output), expected)
	}
	return output, nil
}

func archiveName(entry Entry) (string, error) {
	name := entry.Name()
	if entry.Extension == "class" && !(entry.Base == "module-info" && entry.Parent == "") {
		if entry.Parent != "" {
			name = entry.Parent + "/" + entry.Base + "." + entry.Extension
		} else {
			name = entry.Base + "." + entry.Extension
		}
	}
	if !safeArchiveName(name) {
		return "", fmt.Errorf("jimage: unsafe archive entry %q", name)
	}
	return name, nil
}

func safeArchiveName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func normalizeName(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, modulesModuleName+"/")
	return name
}

func endianOrLittle(raw []byte) uint32 {
	if len(raw) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(raw[:4])
}
