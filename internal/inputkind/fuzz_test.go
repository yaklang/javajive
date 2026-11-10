package inputkind

import (
	"archive/zip"
	"bytes"
	"testing"

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

func FuzzDetectBytes(f *testing.F) {
	emptyZIP := fuzzZIP(f, nil)
	nestedAPK := fuzzZIP(f, map[string][]byte{"classes.dex": []byte("dex\n035\x00")})
	jimageData, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe}},
	})
	if err != nil {
		f.Fatal(err)
	}
	seeds := []struct {
		data []byte
		name string
	}{
		{[]byte("dex\n035\x00"), "classes.dex"},
		{[]byte{0xca, 0xfe, 0xba, 0xbe}, "Foo.class"},
		{append([]byte{'J', 'M', 1, 0}, emptyZIP...), "java.base.jmod"},
		{jimageData, "modules"},
		{fuzzZIP(f, map[string][]byte{"WEB-INF/classes/Foo.class": {0xca, 0xfe, 0xba, 0xbe}}), "app.war"},
		{fuzzZIP(f, map[string][]byte{"base.apk": nestedAPK}), "app.apks"},
		{fuzzZIP(f, map[string][]byte{"manifest.json": {1}, "base.apk": nestedAPK}), "app.xapk"},
		{fuzzZIP(f, map[string][]byte{"info.json": {1}, "base.apk": nestedAPK}), "app.apkm"},
		{fuzzZIP(f, map[string][]byte{"base/dex/classes.dex": []byte("dex\n035\x00")}), "app.aab"},
		{fuzzZIP(f, map[string][]byte{"classes.jar": {0x50, 0x4b, 3, 4}, "AndroidManifest.xml": {1}}), "lib.aar"},
		{[]byte("not an artifact"), "notes.txt"},
	}
	for _, seed := range seeds {
		f.Add(seed.data, seed.name)
	}
	f.Fuzz(func(t *testing.T, data []byte, name string) {
		kind := DetectBytes(data, name)
		_ = IsAndroid(kind)
		_ = IsJavaArchive(kind)
		_ = DetectMagic(data)
		_ = IsJMODHeader(data)
		_ = IsJImageHeader(data)
	})
}
