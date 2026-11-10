package input

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func fuzzZIP(tb testing.TB, files map[string][]byte) []byte {
	tb.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			tb.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		tb.Fatal(err)
	}
	return buffer.Bytes()
}

func FuzzParseApplication(f *testing.F) {
	dex, err := os.ReadFile(filepath.Join("..", "testdata", "accept", "arithmetic.dex"))
	if err != nil {
		dex = []byte("dex\n035\x00")
	}
	apk := fuzzZIP(f, map[string][]byte{"classes.dex": dex})
	apks := fuzzZIP(f, map[string][]byte{"base.apk": apk})
	aab := fuzzZIP(f, map[string][]byte{"base/dex/classes.dex": dex})
	seeds := [][]byte{
		dex,
		apk,
		apks,
		aab,
		[]byte("dex\n035\x00"),
		[]byte("not an artifact"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	options := Options{
		MaxDEXSize:      1 << 20,
		MaxAPKSize:      2 << 20,
		MaxTotalDEXSize: 2 << 20,
		MaxDEXFiles:     8,
		MaxNestedAPKs:   8,
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = ParseApplicationWithOptions(data, "fuzz", options)
	})
}
