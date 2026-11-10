package zip

import (
	stdzip "archive/zip"
	"bytes"
	"errors"
	"io"
	"testing"
)

func rawStoreZIP(t testing.TB, body, extra []byte, crc uint32) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := stdzip.NewWriter(&buffer)
	header := &stdzip.FileHeader{
		Name:               "test.txt",
		Method:             stdzip.Store,
		CRC32:              crc,
		CompressedSize64:   uint64(len(body)),
		UncompressedSize64: uint64(len(body)),
		Extra:              extra,
	}
	entry, err := writer.CreateRaw(header)
	if err != nil {
		t.Fatalf("CreateRaw: %v", err)
	}
	if _, err := entry.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP: %v", err)
	}
	return buffer.Bytes()
}

func TestNewReaderRejectsShortWinZipAESExtra(t *testing.T) {
	extra := []byte{0x01, 0x99, 0x01, 0x00, 0x02}
	raw := rawStoreZIP(t, nil, extra, 0)
	if _, err := NewReader(bytes.NewReader(raw), int64(len(raw))); !errors.Is(err, ErrFormat) {
		t.Fatalf("NewReader error = %v, want ErrFormat", err)
	}
}

func TestOpenVerifiesZeroCRC(t *testing.T) {
	raw := rawStoreZIP(t, []byte("x"), nil, 0)
	reader, err := NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	stream, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = io.ReadAll(stream)
	_ = stream.Close()
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("ReadAll error = %v, want ErrChecksum", err)
	}
}

func FuzzNewReaderMalformed(f *testing.F) {
	f.Add(rawStoreZIP(f, nil, []byte{0x01, 0x99, 0x01, 0x00, 0x02}, 0))
	f.Add(rawStoreZIP(f, []byte("x"), nil, 0))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		reader, err := NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		for index, file := range reader.File {
			if index >= 4 {
				break
			}
			stream, err := file.Open()
			if err != nil {
				continue
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(stream, 1<<20))
			_ = stream.Close()
		}
	})
}
