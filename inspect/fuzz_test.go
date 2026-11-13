package inspect_test

import (
	"archive/zip"
	"bytes"
	"testing"

	artifactinspect "github.com/yaklang/javajive/inspect"
	"github.com/yaklang/javajive/internal/jimagefixture"
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

func FuzzAnalyze(f *testing.F) {
	emptyZIP := fuzzZIP(f, nil)
	apk := fuzzZIP(f, map[string][]byte{"classes.dex": []byte("dex\n035\x00")})
	jimageData, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe}},
	})
	if err != nil {
		f.Fatal(err)
	}
	seeds := [][]byte{
		[]byte("dex\n035\x00"),
		[]byte{0xca, 0xfe, 0xba, 0xbe},
		append([]byte{'J', 'M', 1, 0}, emptyZIP...),
		jimageData,
		apk,
		fuzzZIP(f, map[string][]byte{"base.apk": apk}),
		fuzzZIP(f, map[string][]byte{"base/dex/classes.dex": []byte("dex\n035\x00")}),
		fuzzZIP(f, map[string][]byte{"manifest.json": {1}, "base.apk": apk}),
		fuzzZIP(f, map[string][]byte{"info.json": {1}, "base.apk": apk}),
		fuzzZIP(f, map[string][]byte{"WEB-INF/classes/Foo.class": {0xca, 0xfe, 0xba, 0xbe}}),
		[]byte("not an artifact"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = artifactinspect.AnalyzeWithOptions(data, "fuzz", artifactinspect.Options{Disassembly: false})
	})
}
