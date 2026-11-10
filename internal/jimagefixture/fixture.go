// Package jimagefixture builds small synthetic JImage files for tests.
package jimagefixture

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	imageMagic          = 0xCAFEDADA
	resourceHeaderMagic = 0xCAFEFAFA
	headerSize          = 28
	resourceHeaderSize  = 29

	attributeModule       = 1
	attributeParent       = 2
	attributeBase         = 3
	attributeExtension    = 4
	attributeOffset       = 5
	attributeCompressed   = 6
	attributeUncompressed = 7
)

// Entry describes one resource in a synthetic JImage. Path must start with
// "/<module>/...". When Compressed is true, Compressor defaults to "zip";
// "compact-cp" produces a structurally valid unsupported resource.
type Entry struct {
	Path       string
	Data       []byte
	Compressed bool
	Compressor string
}

type parsedEntry struct {
	module     string
	parent     string
	base       string
	extension  string
	data       []byte
	compressed bool
	compressor string
}

// Build returns a little-endian JImage containing entries.
func Build(entries []Entry) ([]byte, error) {
	parsed := make([]parsedEntry, 0, len(entries))
	for _, entry := range entries {
		module, parent, base, extension, err := parsePath(entry.Path)
		if err != nil {
			return nil, err
		}
		compressor := entry.Compressor
		if entry.Compressed && compressor == "" {
			compressor = "zip"
		}
		if !entry.Compressed {
			compressor = ""
		}
		parsed = append(parsed, parsedEntry{
			module: module, parent: parent, base: base, extension: extension,
			data: entry.Data, compressed: entry.Compressed, compressor: compressor,
		})
	}

	stringsData := []byte{0}
	stringOffsets := map[string]uint32{"": 0}
	addString := func(value string) uint32 {
		if value == "" {
			return 0
		}
		if offset, ok := stringOffsets[value]; ok {
			return offset
		}
		offset := uint32(len(stringsData))
		stringsData = append(stringsData, value...)
		stringsData = append(stringsData, 0)
		stringOffsets[value] = offset
		return offset
	}
	for _, entry := range parsed {
		addString(entry.module)
		addString(entry.parent)
		addString(entry.base)
		addString(entry.extension)
		if entry.compressed {
			addString(entry.compressor)
		}
	}

	locations := []byte{0}
	offsetsTable := make([]uint32, len(parsed))
	resourceData := make([]byte, 0)
	for index, entry := range parsed {
		payload := entry.data
		compressedSize := uint64(0)
		if entry.compressed {
			compressed, err := compressResource(entry.data, entry.compressor, addString)
			if err != nil {
				return nil, err
			}
			payload = compressed
			compressedSize = uint64(len(compressed))
		}

		attributes := make([]byte, 0, 32)
		attributes = appendAttribute(attributes, attributeModule, uint64(addString(entry.module)))
		if entry.parent != "" {
			attributes = appendAttribute(attributes, attributeParent, uint64(addString(entry.parent)))
		}
		attributes = appendAttribute(attributes, attributeBase, uint64(addString(entry.base)))
		if entry.extension != "" {
			attributes = appendAttribute(attributes, attributeExtension, uint64(addString(entry.extension)))
		}
		if len(resourceData) != 0 {
			attributes = appendAttribute(attributes, attributeOffset, uint64(len(resourceData)))
		}
		if compressedSize != 0 {
			attributes = appendAttribute(attributes, attributeCompressed, compressedSize)
		}
		attributes = appendAttribute(attributes, attributeUncompressed, uint64(len(entry.data)))
		attributes = append(attributes, 0)

		offsetsTable[index] = uint32(len(locations))
		locations = append(locations, attributes...)
		resourceData = append(resourceData, payload...)
	}

	tableLength := len(parsed)
	header := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(header[0:4], imageMagic)
	binary.LittleEndian.PutUint32(header[4:8], 0x00010001)
	binary.LittleEndian.PutUint32(header[8:12], 0)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(parsed)))
	binary.LittleEndian.PutUint32(header[16:20], uint32(tableLength))
	binary.LittleEndian.PutUint32(header[20:24], uint32(len(locations)))
	binary.LittleEndian.PutUint32(header[24:28], uint32(len(stringsData)))

	redirects := make([]byte, tableLength*4)
	offsetBytes := make([]byte, tableLength*4)
	for index, offset := range offsetsTable {
		binary.LittleEndian.PutUint32(offsetBytes[index*4:], offset)
	}

	result := make([]byte, 0, headerSize+len(redirects)+len(offsetBytes)+len(locations)+len(stringsData)+len(resourceData))
	result = append(result, header...)
	result = append(result, redirects...)
	result = append(result, offsetBytes...)
	result = append(result, locations...)
	result = append(result, stringsData...)
	result = append(result, resourceData...)
	return result, nil
}

func compressResource(data []byte, compressor string, addString func(string) uint32) ([]byte, error) {
	var payload []byte
	if compressor == "zip" {
		var buffer bytes.Buffer
		writer := zlib.NewWriter(&buffer)
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		payload = buffer.Bytes()
	} else {
		// The reader rejects unsupported compressors before reading the
		// payload, so a placeholder keeps the resource structurally valid.
		payload = []byte{0}
	}

	header := make([]byte, resourceHeaderSize)
	binary.LittleEndian.PutUint32(header[0:4], resourceHeaderMagic)
	binary.LittleEndian.PutUint64(header[4:12], uint64(len(payload)))
	binary.LittleEndian.PutUint64(header[12:20], uint64(len(data)))
	binary.LittleEndian.PutUint32(header[20:24], addString(compressor))
	binary.LittleEndian.PutUint32(header[24:28], 0)
	header[28] = 1
	return append(header, payload...), nil
}

func appendAttribute(destination []byte, kind byte, value uint64) []byte {
	length := 1
	for shifted := value >> 8; shifted != 0; shifted >>= 8 {
		length++
	}
	destination = append(destination, kind<<3|byte(length-1))
	for shift := (length - 1) * 8; shift >= 0; shift -= 8 {
		destination = append(destination, byte(value>>uint(shift)))
	}
	return destination
}

func parsePath(value string) (module, parent, base, extension string, err error) {
	if !strings.HasPrefix(value, "/") {
		return "", "", "", "", fmt.Errorf("jimage fixture: path %q must start with /", value)
	}
	parts := strings.Split(strings.TrimPrefix(value, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return "", "", "", "", fmt.Errorf("jimage fixture: path %q needs a module and a file name", value)
	}
	last := parts[len(parts)-1]
	if last == "" {
		return "", "", "", "", fmt.Errorf("jimage fixture: path %q has an empty file name", value)
	}
	base = last
	if dot := strings.LastIndex(last, "."); dot >= 0 {
		base = last[:dot]
		extension = last[dot+1:]
	}
	module = parts[0]
	parent = strings.Join(parts[1:len(parts)-1], "/")
	return module, parent, base, extension, nil
}
