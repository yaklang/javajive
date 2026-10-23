package javaclassparser

import (
	"archive/zip"
	"io"
	"regexp"
	"strings"
	"testing"
)

func TestClinitRendersAsStaticBlock(t *testing.T) {
	const jarPath = "/home/wlz/Target/ekp/hotdeploy/ekp/WEB-INF/lib/ridl-3.0.1.jar"
	const member = "com/sun/star/lang/IllegalArgumentException.class"
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
		t.Fatal("class missing from jar")
	}
	src, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src, "private static") {
		t.Fatalf("clinit kept illegal modifiers:\n%s", src)
	}
	if !regexp.MustCompile(`static\s+\{`).MatchString(src) {
		t.Fatalf("missing static initializer:\n%s", src)
	}
}
