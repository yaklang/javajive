package javaclassparser

import (
	"archive/zip"
	"io"
	"strings"
	"testing"
)

func TestCoremailInnerCaptureConstructorIsNotStub(t *testing.T) {
	const warPath = "/home/wlz/Target/coremail/CoremailSync.war"
	war, err := zip.OpenReader(warPath)
	if err != nil {
		t.Skip(err)
	}
	defer war.Close()
	var jarBytes []byte
	for _, f := range war.File {
		if f.Name != "WEB-INF/lib/wmsvr.jar" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		jarBytes, err = io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if len(jarBytes) == 0 {
		t.Fatal("wmsvr.jar missing")
	}
	jar, err := zip.NewReader(strings.NewReader(string(jarBytes)), int64(len(jarBytes)))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		for _, f := range jar.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}
		return nil
	}
	raw := read("a/df.class")
	if len(raw) == 0 {
		t.Fatal("a/df.class missing")
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{
		RetainUnprovedSource: true,
		Resolve: func(name string) ([]byte, bool) {
			got := read(name + ".class")
			return got, len(got) > 0
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := result.Source
	if strings.Contains(src, "undecompilable") {
		t.Fatalf("constructor still stubbed:\n%s", src)
	}
	if !strings.Contains(src, "super(") || !strings.Contains(src, "this.m =") {
		t.Fatalf("capture was not kept:\n%s", src)
	}
}
