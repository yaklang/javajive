// Command compressjdk deterministically compresses the reviewed JDK catalog
// and constructor classfile bundle.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	constructorBundleMagic      = "JJCC0001"
	constructorChunkTargetBytes = 1 << 20
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

func main() {
	input := flag.String("input", "", "reviewed JDK catalog JSON; output is <input>.zst")
	constructorZip := flag.String("constructor-zip", "", "reviewed JDK constructor ZIP; output replaces .zip with .bundle.zst")
	flag.Parse()
	if (*input == "" && *constructorZip == "") || (*input != "" && *constructorZip != "") || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: compressjdk -input jdk_metadata_catalog.json | -constructor-zip jdk_constructor_code.zip")
		os.Exit(2)
	}
	var err error
	if *input != "" {
		err = compress(*input)
	} else {
		err = compressConstructorZip(*constructorZip)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compress(input string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(8<<20),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return err
	}
	defer encoder.Close()
	compressed := encoder.EncodeAll(raw, nil)
	return os.WriteFile(input+".zst", compressed, 0644)
}

func compressConstructorZip(input string) error {
	zr, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if files[f.Name] != nil {
			return fmt.Errorf("duplicate zip entry %q", f.Name)
		}
		files[f.Name] = f
	}
	manifestFile := files["manifest.json"]
	if manifestFile == nil {
		return fmt.Errorf("missing manifest.json in %s", input)
	}
	mf, err := manifestFile.Open()
	if err != nil {
		return err
	}
	manifestBytes, err := io.ReadAll(io.LimitReader(mf, 2<<20))
	mf.Close()
	if err != nil {
		return err
	}
	var doc constructorCodeDocument
	if err := json.Unmarshal(manifestBytes, &doc); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	if doc.Schema != 1 || len(doc.Profiles) == 0 {
		return fmt.Errorf("invalid manifest schema=%d profiles=%d", doc.Schema, len(doc.Profiles))
	}

	type classRecord struct {
		release int
		name    string
		sha256  string
		raw     []byte
	}
	var records []classRecord
	profileIndex := make([]constructorBundleProfile, len(doc.Profiles))
	profilePos := make(map[int]int, len(doc.Profiles))
	for i, prof := range doc.Profiles {
		if _, dup := profilePos[prof.Release]; dup {
			return fmt.Errorf("duplicate release %d", prof.Release)
		}
		profilePos[prof.Release] = i
		profileIndex[i] = constructorBundleProfile{
			Release:       prof.Release,
			ArchiveSHA256: prof.ArchiveSHA256,
			Classes:       make(map[string]constructorBundleEntry, len(prof.Classes)),
		}
		for name, wantHash := range prof.Classes {
			entry := files[fmt.Sprintf("%d/%s.class", prof.Release, name)]
			if entry == nil {
				return fmt.Errorf("missing class %d/%s.class", prof.Release, name)
			}
			rc, err := entry.Open()
			if err != nil {
				return err
			}
			raw, err := io.ReadAll(io.LimitReader(rc, 4<<20))
			rc.Close()
			if err != nil {
				return err
			}
			sum := sha256.Sum256(raw)
			if got := hex.EncodeToString(sum[:]); got != wantHash {
				return fmt.Errorf("hash mismatch for %d/%s: got %s want %s", prof.Release, name, got, wantHash)
			}
			records = append(records, classRecord{
				release: prof.Release,
				name:    name,
				sha256:  wantHash,
				raw:     raw,
			})
		}
	}

	// Sort by class name first, then release, so versions of the same JDK class
	// across releases sit adjacent in the same chunk for strong cross-release
	// dictionary compression.
	sort.Slice(records, func(i, j int) bool {
		if records[i].name != records[j].name {
			return records[i].name < records[j].name
		}
		return records[i].release < records[j].release
	})

	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(2<<20),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return err
	}
	defer encoder.Close()

	type chunkMeta struct {
		compOffset uint32
		compLen    uint32
		rawLen     uint32
	}
	var chunksMeta []chunkMeta
	var chunksData bytes.Buffer
	var curChunk bytes.Buffer

	flushChunk := func() {
		if curChunk.Len() == 0 {
			return
		}
		rawBytes := curChunk.Bytes()
		comp := encoder.EncodeAll(rawBytes, nil)
		chunksMeta = append(chunksMeta, chunkMeta{
			compOffset: uint32(chunksData.Len()),
			compLen:    uint32(len(comp)),
			rawLen:     uint32(len(rawBytes)),
		})
		chunksData.Write(comp)
		curChunk.Reset()
	}

	for _, rec := range records {
		if curChunk.Len() > 0 && curChunk.Len()+len(rec.raw) > constructorChunkTargetBytes {
			flushChunk()
		}
		chunkIdx := len(chunksMeta)
		offset := uint32(curChunk.Len())
		curChunk.Write(rec.raw)
		pidx := profilePos[rec.release]
		profileIndex[pidx].Classes[rec.name] = constructorBundleEntry{
			SHA256: rec.sha256,
			Chunk:  chunkIdx,
			Offset: offset,
			Length: uint32(len(rec.raw)),
		}
	}
	flushChunk()

	indexJSON, err := json.Marshal(constructorBundleIndex{
		Schema:   1,
		Profiles: profileIndex,
	})
	if err != nil {
		return err
	}
	manifestComp := encoder.EncodeAll(indexJSON, nil)

	var out bytes.Buffer
	out.WriteString(constructorBundleMagic)
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(manifestComp)))
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(chunksMeta)))
	for _, cm := range chunksMeta {
		_ = binary.Write(&out, binary.LittleEndian, cm.compOffset)
		_ = binary.Write(&out, binary.LittleEndian, cm.compLen)
		_ = binary.Write(&out, binary.LittleEndian, cm.rawLen)
	}
	out.Write(manifestComp)
	out.Write(chunksData.Bytes())

	outPath := strings.TrimSuffix(input, ".zip") + ".bundle.zst"
	return os.WriteFile(outPath, out.Bytes(), 0644)
}
