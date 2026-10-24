package javaclassparser

import (
	"archive/zip"
	"io"
	"strings"
	"testing"
)

func TestJacksonSwitchCaseDeclarationIsBraced(t *testing.T) {
	const jarPath = "/home/wlz/Target/ekp/hotdeploy/ekp/WEB-INF/lib/jackson-core-2.9.10.jar"
	const member = "com/fasterxml/jackson/core/sym/ByteQuadsCanonicalizer.class"
	zf, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Skip(err)
	}
	defer zf.Close()
	var raw []byte
	for _, f := range zf.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if len(raw) == 0 {
		t.Fatal("class missing")
	}
	src, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(src, "case 8:")
	if i < 0 {
		t.Fatal("missing case 8")
	}
	window := src[i:]
	if len(window) > 80 {
		window = window[:80]
	}
	if !strings.Contains(window, "{") {
		t.Fatalf("case declaration is not braced:\n%s", window)
	}
}
