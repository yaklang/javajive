package inputkind

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/internal/jimagefixture"
)

func testZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := file.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func TestDetectBytes(t *testing.T) {
	emptyZIP := testZIP(t, nil)
	nestedAPK := testZIP(t, map[string][]byte{"classes.dex": []byte("dex\n035\x00")})
	jimageData, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe}},
	})
	if err != nil {
		t.Fatalf("build JImage: %v", err)
	}
	cases := []struct {
		name string
		data []byte
		file string
		want Kind
	}{
		{"dex", []byte("dex\n035\x00"), "renamed.bin", DEX},
		{"class", []byte{0xca, 0xfe, 0xba, 0xbe}, "renamed.bin", Class},
		{"jmod", append([]byte{'J', 'M', 1, 0}, emptyZIP...), "java.base.jmod", JMOD},
		{"jimage", jimageData, "modules", JIMAGE},
		{"jimage-extension", nil, "modules.jimage", JIMAGE},
		{"apk", testZIP(t, map[string][]byte{"classes.dex": []byte("dex\n035\x00")}), "renamed.zip", APK},
		{"apks", testZIP(t, map[string][]byte{"base.apk": nestedAPK}), "renamed.bin", APKS},
		{"apks-toc", testZIP(t, map[string][]byte{"toc.pb": {1}, "base-master.apk": nestedAPK}), "renamed.bin", APKS},
		{"xapk", testZIP(t, map[string][]byte{"manifest.json": {1}, "base.apk": nestedAPK}), "renamed.bin", XAPK},
		{"apkm", testZIP(t, map[string][]byte{"info.json": {1}, "base.apk": nestedAPK}), "renamed.bin", APKM},
		{"aab", testZIP(t, map[string][]byte{"base/dex/classes.dex": []byte("dex\n035\x00")}), "renamed.bin", AAB},
		{"war", testZIP(t, map[string][]byte{"WEB-INF/classes/demo/Foo.class": {0xca, 0xfe, 0xba, 0xbe}}), "renamed.bin", WAR},
		{"ear", testZIP(t, map[string][]byte{"EAR/lib/dep.jar": {0x50, 0x4b, 3, 4}}), "renamed.bin", EAR},
		{"aar", testZIP(t, map[string][]byte{"AndroidManifest.xml": {1}, "classes.jar": {0x50, 0x4b, 3, 4}}), "renamed.bin", AAR},
		{"jar", testZIP(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\n\r\n")}), "renamed.bin", JAR},
		{"par", emptyZIP, "plugin.par", PAR},
		{"zip", emptyZIP, "data.zip", ZIP},
		{"unknown-text", []byte("not an artifact"), "notes.txt", Unknown},
		{"generic-zip-unknown-extension", emptyZIP, "payload.bin", ZIP},
		{"jmod-extension-fallback", emptyZIP, "module.jmod", JMOD},
		{"apkx-extension-fallback", testZIP(t, map[string][]byte{"base.apk": nestedAPK}), "bundle.apkx", XAPK},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectBytes(test.data, test.file); got != test.want {
				t.Fatalf("DetectBytes = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetectFile(t *testing.T) {
	jmod := append([]byte{'J', 'M', 1, 0}, testZIP(t, map[string][]byte{"classes/demo/Foo.class": {0xca, 0xfe, 0xba, 0xbe}})...)
	path := filepath.Join(t.TempDir(), "java.base.jmod")
	if err := os.WriteFile(path, jmod, 0o644); err != nil {
		t.Fatal(err)
	}
	kind, err := DetectFile(path)
	if err != nil {
		t.Fatalf("DetectFile: %v", err)
	}
	if kind != JMOD {
		t.Fatalf("DetectFile = %q, want jmod", kind)
	}
}

func TestRootDEXIndex(t *testing.T) {
	cases := []struct {
		name      string
		wantIndex int
		wantOK    bool
	}{
		{"classes.dex", 1, true},
		{"classes2.dex", 2, true},
		{"classes10.dex", 10, true},
		{"classes0.dex", 0, false},
		{"classes1.dex", 0, false},
		{"classes01.dex", 0, false},
		{"assets/classes.dex", 0, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			index, ok := RootDEXIndex(test.name)
			if index != test.wantIndex || ok != test.wantOK {
				t.Fatalf("RootDEXIndex = (%d, %v), want (%d, %v)", index, ok, test.wantIndex, test.wantOK)
			}
		})
	}
}

func TestAABDEX(t *testing.T) {
	cases := []struct {
		name       string
		wantModule string
		wantIndex  int
		wantOK     bool
	}{
		{"base/dex/classes.dex", "base", 1, true},
		{"feature/dex/classes2.dex", "feature", 2, true},
		{"base/dex/classes01.dex", "", 0, false},
		{"base/classes.dex", "", 0, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			module, index, ok := AABDEX(test.name)
			if module != test.wantModule || index != test.wantIndex || ok != test.wantOK {
				t.Fatalf("AABDEX = (%q, %d, %v), want (%q, %d, %v)", module, index, ok, test.wantModule, test.wantIndex, test.wantOK)
			}
		})
	}
}

func TestKindGroups(t *testing.T) {
	for _, kind := range []Kind{APK, APKS, XAPK, APKM, AAB} {
		if !IsAndroid(kind) {
			t.Fatalf("IsAndroid(%q) = false", kind)
		}
	}
	for _, kind := range []Kind{JMOD, JIMAGE, JAR, WAR, EAR, AAR, PAR, ZIP} {
		if !IsJavaArchive(kind) {
			t.Fatalf("IsJavaArchive(%q) = false", kind)
		}
	}
	if IsAndroid(JAR) || IsJavaArchive(APK) {
		t.Fatal("container groups overlap")
	}
}
