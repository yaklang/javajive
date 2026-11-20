package javaclassparser

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	constructorBundleMagic        = "JJCC0001"
	constructorBundleMaxManifest  = 4 << 20
	constructorBundleMaxChunkRaw  = 4 << 20
	constructorBundleMaxClassSize = 2 << 20
)

type constructorCodeProfile struct {
	Release       int               `json:"release"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	Classes       map[string]string `json:"classes"`
}
type constructorCodeDocument struct {
	Schema   int                      `json:"schema"`
	Profiles []constructorCodeProfile `json:"profiles"`
}

type constructorBundleEntry struct {
	SHA256 string `json:"s"`
	Chunk  int    `json:"c"`
	Offset uint32 `json:"o"`
	Length uint32 `json:"l"`
}

type constructorBundleProfile struct {
	Release       int                               `json:"release"`
	ArchiveSHA256 string                            `json:"archive_sha256"`
	Classes       map[string]constructorBundleEntry `json:"classes"`
}

type constructorBundleIndex struct {
	Schema   int                        `json:"schema"`
	Profiles []constructorBundleProfile `json:"profiles"`
}

type constructorChunkMeta struct {
	compOffset uint32
	compLen    uint32
	rawLen     uint32
	once       sync.Once
	raw        []byte
	ok         bool
}

// Original classfiles, extracted from the exact platform archives pinned by
// the declaration catalog. They are read only as metadata/bytecode: never
// loaded by a JVM or used as emitted archive-owned classes. Selection by the
// offline generator is an index optimization; the request must still prove
// the original constructor's effects, field identity and all paths. Public
// platform annotation declarations also retain their original Retention/Target
// attributes; their presence is evidence, not permission to rewrite a use.
// Indexed declarations from each pinned primary archive retain their original
// code and ancestry. Interfaces retain abstract/default/static declarations for
// functional-target ownership proofs; class membership grants no effect permission.
// Public extensible APIs with complete original ancestry are retained even
// when their constructors observe THIS. Independent declaration/reference
// provenance pins every indexed class; code membership is never an effect proof.
//
// Keep the reviewed ZIP source for regeneration and provenance checks. Only its
// deterministic chunked-zstd representation is linked into applications.
//
//go:generate go run ../tools/compressjdk -constructor-zip jdk_constructor_code.zip
//go:embed jdk_constructor_code.bundle.zst
var jdkConstructorCodeBundle []byte

var jdkConstructorCode struct {
	once      sync.Once
	chunks    []constructorChunkMeta
	chunkData []byte
	entries   map[int]map[string]constructorBundleEntry
	hashes    map[int]map[string]string
}

// jdkConstructorNameCandidate is a conservative namespace gate over the
// embedded constructor corpus. A same-package identity probe for an application
// class (for example com/example/String) can never resolve here, and must not
// pay the one-time catalog + bundle index cost.
func jdkConstructorNameCandidate(name string) bool {
	switch {
	case strings.HasPrefix(name, "java/"), strings.HasPrefix(name, "javax/"),
		strings.HasPrefix(name, "jdk/"), strings.HasPrefix(name, "sun/"):
		return true
	case strings.HasPrefix(name, "org/omg/"), strings.HasPrefix(name, "org/w3c/"),
		strings.HasPrefix(name, "org/xml/"):
		return true
	}
	return false
}

func decodeConstructorZstd(src []byte, maxDecoded int) ([]byte, error) {
	decoder, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderLowmem(true),
		zstd.WithDecoderMaxMemory(uint64(maxDecoded)*2),
	)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	raw, err := decoder.DecodeAll(src, nil)
	if err != nil || len(raw) > maxDecoded {
		return nil, err
	}
	return raw, nil
}

func jdkConstructorChunkBytes(chunkIdx int) ([]byte, bool) {
	if chunkIdx < 0 || chunkIdx >= len(jdkConstructorCode.chunks) {
		return nil, false
	}
	chunk := &jdkConstructorCode.chunks[chunkIdx]
	chunk.once.Do(func() {
		start := uint64(chunk.compOffset)
		end := start + uint64(chunk.compLen)
		if end > uint64(len(jdkConstructorCode.chunkData)) || chunk.rawLen > constructorBundleMaxChunkRaw {
			return
		}
		raw, err := decodeConstructorZstd(jdkConstructorCode.chunkData[start:end], int(chunk.rawLen))
		if err != nil || uint32(len(raw)) != chunk.rawLen {
			return
		}
		chunk.raw = raw
		chunk.ok = true
	})
	return chunk.raw, chunk.ok
}

func jdkConstructorClassBytes(name string, release int) ([]byte, bool) {
	if !jdkConstructorNameCandidate(name) {
		return nil, false
	}
	jdkConstructorCode.once.Do(func() {
		buf := jdkConstructorCodeBundle
		if len(buf) < 16 || string(buf[:8]) != constructorBundleMagic {
			return
		}
		manifestCompLen := binary.LittleEndian.Uint32(buf[8:12])
		chunkCount := binary.LittleEndian.Uint32(buf[12:16])
		if manifestCompLen == 0 || chunkCount == 0 || chunkCount > 4096 {
			return
		}
		tableEnd := uint64(16) + uint64(chunkCount)*12
		manifestEnd := tableEnd + uint64(manifestCompLen)
		if manifestEnd > uint64(len(buf)) {
			return
		}
		chunks := make([]constructorChunkMeta, chunkCount)
		chunkData := buf[manifestEnd:]
		for i := uint32(0); i < chunkCount; i++ {
			off := 16 + int(i)*12
			compOffset := binary.LittleEndian.Uint32(buf[off : off+4])
			compLen := binary.LittleEndian.Uint32(buf[off+4 : off+8])
			rawLen := binary.LittleEndian.Uint32(buf[off+8 : off+12])
			if uint64(compOffset)+uint64(compLen) > uint64(len(chunkData)) || rawLen > constructorBundleMaxChunkRaw {
				return
			}
			chunks[i].compOffset = compOffset
			chunks[i].compLen = compLen
			chunks[i].rawLen = rawLen
		}
		manifestRaw, err := decodeConstructorZstd(buf[tableEnd:manifestEnd], constructorBundleMaxManifest)
		if err != nil || len(manifestRaw) == 0 {
			return
		}
		var idx constructorBundleIndex
		if json.Unmarshal(manifestRaw, &idx) != nil || idx.Schema != 1 || len(idx.Profiles) == 0 {
			return
		}
		entries := make(map[int]map[string]constructorBundleEntry, len(idx.Profiles))
		hashes := make(map[int]map[string]string, len(idx.Profiles))
		for _, profile := range idx.Profiles {
			if entries[profile.Release] != nil || len(profile.ArchiveSHA256) != 64 || len(profile.Classes) == 0 {
				return
			}
			table := make(map[string]constructorBundleEntry, len(profile.Classes))
			hashTable := make(map[string]string, len(profile.Classes))
			for className, entry := range profile.Classes {
				digest, err := hex.DecodeString(entry.SHA256)
				if err != nil || len(digest) != 32 {
					return
				}
				if entry.Chunk < 0 || entry.Chunk >= len(chunks) || entry.Length > constructorBundleMaxClassSize {
					return
				}
				if uint64(entry.Offset)+uint64(entry.Length) > uint64(chunks[entry.Chunk].rawLen) {
					return
				}
				table[className] = entry
				hashTable[className] = entry.SHA256
			}
			entries[profile.Release] = table
			hashes[profile.Release] = hashTable
		}
		jdkConstructorCode.chunks = chunks
		jdkConstructorCode.chunkData = chunkData
		jdkConstructorCode.entries = entries
		jdkConstructorCode.hashes = hashes
	})
	entry, ok := jdkConstructorCode.entries[release][name]
	if !ok {
		return nil, false
	}
	chunk, ok := jdkConstructorChunkBytes(entry.Chunk)
	if !ok {
		return nil, false
	}
	end := uint64(entry.Offset) + uint64(entry.Length)
	if end > uint64(len(chunk)) {
		return nil, false
	}
	raw := make([]byte, entry.Length)
	copy(raw, chunk[entry.Offset:end])
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != jdkConstructorCode.hashes[release][name] {
		return nil, false
	}
	return raw, true
}
