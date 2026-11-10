package input

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/inputkind"
)

func TestCollectContainerEdgeFailures(t *testing.T) {
	empty := buildAPK(t, nil)
	if _, err := collectAPK(empty, "", Options{}, false); err == nil || !strings.Contains(err.Error(), "no classes*.dex entries") {
		t.Fatalf("empty APK error = %v", err)
	}
	if _, err := collectAPK([]byte("not a zip"), "", Options{}, false); err == nil {
		t.Fatal("collectAPK accepted a non-ZIP")
	}
	if _, err := collectAPKS(empty, inputkind.APKS, Options{}); err == nil || !strings.Contains(err.Error(), "no nested .apk entries") {
		t.Fatalf("empty bundle error = %v", err)
	}
	if _, err := collectAPKS([]byte("not a zip"), inputkind.APKS, Options{}); err == nil {
		t.Fatal("collectAPKS accepted a non-ZIP")
	}

	nestedEmpty := buildAPK(t, []zipEntry{{name: "base.apk", data: empty}})
	if _, err := collectAPKS(nestedEmpty, inputkind.APKS, Options{}); err == nil || !strings.Contains(err.Error(), "no classes*.dex entries in nested APKs") {
		t.Fatalf("empty nested APK error = %v", err)
	}
	if _, err := collectAAB(empty, Options{}); err == nil || !strings.Contains(err.Error(), "no <module>/dex/classes*.dex entries") {
		t.Fatalf("empty AAB error = %v", err)
	}
	if _, err := collectAAB([]byte("not a zip"), Options{}); err == nil {
		t.Fatal("collectAAB accepted a non-ZIP")
	}
}

func TestParseApplicationPropagatesClassificationFailure(t *testing.T) {
	if _, _, err := ParseApplicationWithOptions([]byte("not an artifact"), "", Options{}); err == nil || !strings.Contains(err.Error(), "unsupported DEX input") {
		t.Fatalf("ParseApplicationWithOptions error = %v", err)
	}
}
