package input

import (
	"crypto/sha1"
	"encoding/binary"
	"hash/adler32"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/inputkind"
)

func TestCollectDEXSourcesAPKS(t *testing.T) {
	base := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lbase/One;")},
		{name: "classes2.dex", data: buildDEX(t, "Lbase/Two;")},
	})
	split := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lsplit/Three;")},
	})
	bundle := buildAPK(t, []zipEntry{
		{name: "base.apk", data: base},
		{name: "split_config.apk", data: split},
	})
	sources, kind, err := CollectDEXSources(bundle, "app.apks", Options{})
	if err != nil {
		t.Fatalf("CollectDEXSources: %v", err)
	}
	if kind != inputkind.APKS {
		t.Fatalf("kind = %q, want apks", kind)
	}
	want := []string{
		"base.apk!/classes.dex",
		"base.apk!/classes2.dex",
		"split_config.apk!/classes.dex",
	}
	if len(sources) != len(want) {
		t.Fatalf("sources = %d, want %d", len(sources), len(want))
	}
	for index, entry := range want {
		if sources[index].Entry != entry {
			t.Fatalf("sources[%d].Entry = %q, want %q", index, sources[index].Entry, entry)
		}
	}
	if sources[0].MultidexIndex != 1 || sources[1].MultidexIndex != 2 || sources[2].MultidexIndex != 1 {
		t.Fatalf("multidex indexes = %d, %d, %d", sources[0].MultidexIndex, sources[1].MultidexIndex, sources[2].MultidexIndex)
	}
}

func TestCollectDEXSourcesAAB(t *testing.T) {
	bundle := buildAPK(t, []zipEntry{
		{name: "base/dex/classes.dex", data: buildDEX(t, "Lbase/One;")},
		{name: "base/dex/classes2.dex", data: buildDEX(t, "Lbase/Two;")},
		{name: "feature/dex/classes.dex", data: buildDEX(t, "Lfeature/Three;")},
	})
	sources, kind, err := CollectDEXSources(bundle, "app.aab", Options{})
	if err != nil {
		t.Fatalf("CollectDEXSources: %v", err)
	}
	if kind != inputkind.AAB {
		t.Fatalf("kind = %q, want aab", kind)
	}
	want := []string{
		"base/dex/classes.dex",
		"base/dex/classes2.dex",
		"feature/dex/classes.dex",
	}
	if len(sources) != len(want) {
		t.Fatalf("sources = %d, want %d", len(sources), len(want))
	}
	for index, entry := range want {
		if sources[index].Entry != entry {
			t.Fatalf("sources[%d].Entry = %q, want %q", index, sources[index].Entry, entry)
		}
	}
}

func TestParseApplicationAPKSPrefersUniversalAPK(t *testing.T) {
	universal := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Luniversal/Only;")},
	})
	split := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lsplit/Only;")},
	})
	bundle := buildAPK(t, []zipEntry{
		{name: "universal.apk", data: universal},
		{name: "base-master.apk", data: split},
	})
	app, kind, err := ParseApplicationWithOptions(bundle, "app.apks", Options{})
	if err != nil {
		t.Fatalf("ParseApplicationWithOptions: %v", err)
	}
	if kind != inputkind.APKS || len(app.Files) != 1 {
		t.Fatalf("kind/files = %q/%d", kind, len(app.Files))
	}
	if app.Files[0].Entry != "universal.apk!/classes.dex" {
		t.Fatalf("entry = %q", app.Files[0].Entry)
	}
	if len(app.Files[0].Classes) != 1 || app.Files[0].Classes[0].Descriptor != "Luniversal/Only;" {
		t.Fatalf("classes = %#v", app.Files[0].Classes)
	}
}

func TestParseApplicationAPKSStandalonesUseSingleVariant(t *testing.T) {
	first := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lvariant/First;")},
	})
	second := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lvariant/Second;")},
	})
	bundle := buildAPK(t, []zipEntry{
		{name: "standalones/standalone-b.apk", data: second},
		{name: "standalones/standalone-a.apk", data: first},
	})
	app, kind, err := ParseApplicationWithOptions(bundle, "app.apks", Options{})
	if err != nil {
		t.Fatalf("ParseApplicationWithOptions: %v", err)
	}
	if kind != inputkind.APKS || len(app.Files) != 1 {
		t.Fatalf("kind/files = %q/%d", kind, len(app.Files))
	}
	if app.Files[0].Entry != "standalones/standalone-a.apk!/classes.dex" {
		t.Fatalf("entry = %q", app.Files[0].Entry)
	}
	if len(app.Files[0].Classes) != 1 || app.Files[0].Classes[0].Descriptor != "Lvariant/First;" {
		t.Fatalf("classes = %#v", app.Files[0].Classes)
	}
}

func TestParseApplicationXAPKSkipsResourceOnlySplit(t *testing.T) {
	base := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lbase/One;")},
	})
	config := buildAPK(t, []zipEntry{
		{name: "res/raw/config.txt", data: []byte("resources only")},
	})
	bundle := buildAPK(t, []zipEntry{
		{name: "manifest.json", data: []byte(`{"xapk_version":2}`)},
		{name: "base.apk", data: base},
		{name: "config.arm64_v8a.apk", data: config},
	})
	app, kind, err := ParseApplicationWithOptions(bundle, "app.xapk", Options{})
	if err != nil {
		t.Fatalf("ParseApplicationWithOptions: %v", err)
	}
	if kind != inputkind.XAPK || len(app.Files) != 1 {
		t.Fatalf("kind/files = %q/%d", kind, len(app.Files))
	}
	if app.Files[0].Entry != "base.apk!/classes.dex" {
		t.Fatalf("entry = %q", app.Files[0].Entry)
	}
}

func TestParseApplicationAPKSDedupesIdenticalClasses(t *testing.T) {
	first := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lduplicate/Class;")},
	})
	second := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lduplicate/Class;")},
	})
	bundle := buildAPK(t, []zipEntry{
		{name: "base.apk", data: first},
		{name: "split.apk", data: second},
	})
	app, _, err := ParseApplicationWithOptions(bundle, "app.apks", Options{})
	if err != nil {
		t.Fatalf("ParseApplicationWithOptions: %v", err)
	}
	if len(app.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(app.Files))
	}
	if len(app.Files[0].Classes) != 1 || len(app.Files[1].Classes) != 0 {
		t.Fatalf("deduped classes = %d/%d", len(app.Files[0].Classes), len(app.Files[1].Classes))
	}
}

func TestParseApplicationAPKSRejectsConflictingClasses(t *testing.T) {
	first := buildDEX(t, "Lconflict/Class;")
	secondFixture := buildDEXFixture(t, "Lconflict/Class;")
	// Change the class access flags so the second definition differs.
	const classDefAccessOffset = 4
	binary.LittleEndian.PutUint32(secondFixture.data[secondFixture.classDefsOff+classDefAccessOffset:], 0x0011)
	fixDEXIntegrity(secondFixture.data)
	firstAPK := buildAPK(t, []zipEntry{{name: "classes.dex", data: first}})
	secondAPK := buildAPK(t, []zipEntry{{name: "classes.dex", data: secondFixture.data}})
	bundle := buildAPK(t, []zipEntry{
		{name: "base.apk", data: firstAPK},
		{name: "split.apk", data: secondAPK},
	})
	_, _, err := ParseApplicationWithOptions(bundle, "app.apks", Options{})
	if err == nil || !strings.Contains(err.Error(), "duplicate class") {
		t.Fatalf("error = %v, want duplicate class", err)
	}
}

func TestParseApplicationAPKSLimits(t *testing.T) {
	makeBundle := func() []byte {
		first := buildAPK(t, []zipEntry{{name: "classes.dex", data: buildDEX(t, "Llimit/One;")}})
		second := buildAPK(t, []zipEntry{{name: "classes.dex", data: buildDEX(t, "Llimit/Two;")}})
		return buildAPK(t, []zipEntry{
			{name: "base.apk", data: first},
			{name: "split.apk", data: second},
		})
	}
	t.Run("nested APK count", func(t *testing.T) {
		_, _, err := ParseApplicationWithOptions(makeBundle(), "app.apks", Options{MaxNestedAPKs: 1})
		if err == nil || !strings.Contains(err.Error(), "nested APK count") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("DEX file count", func(t *testing.T) {
		_, _, err := ParseApplicationWithOptions(makeBundle(), "app.apks", Options{MaxDEXFiles: 1})
		if err == nil || !strings.Contains(err.Error(), "DEX file count") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("total nested APK size", func(t *testing.T) {
		_, _, err := ParseApplicationWithOptions(makeBundle(), "app.apks", Options{MaxTotalAPKSize: 1})
		if err == nil || !strings.Contains(err.Error(), "total nested APK size") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCollectDEXSourcesRejectsJavaArchive(t *testing.T) {
	archive := buildAPK(t, []zipEntry{
		{name: "META-INF/MANIFEST.MF", data: []byte("Manifest-Version: 1.0\r\n\r\n")},
	})
	_, kind, err := CollectDEXSources(archive, "library.jar", Options{})
	if kind != inputkind.JAR {
		t.Fatalf("kind = %q, want jar", kind)
	}
	if err == nil || !strings.Contains(err.Error(), "unsupported DEX input") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseApplicationRawDEX(t *testing.T) {
	app, kind, err := ParseApplicationWithOptions(buildDEX(t, "Lraw/One;"), "classes.bin", Options{})
	if err != nil {
		t.Fatalf("ParseApplicationWithOptions: %v", err)
	}
	if kind != inputkind.DEX || len(app.Files) != 1 || app.Files[0].Entry != "classes.dex" {
		t.Fatalf("kind/files = %q/%#v", kind, app.Files)
	}
}

func fixDEXIntegrity(data []byte) {
	if len(data) < 32 {
		return
	}
	signature := sha1.Sum(data[32:])
	copy(data[12:32], signature[:])
	checksum := adler32.Checksum(data[12:])
	binary.LittleEndian.PutUint32(data[8:12], checksum)
}
