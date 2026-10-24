package javaclassparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestConstructorFamilyDiagnostic(t *testing.T) {
	path := os.Getenv("JDEC_FAMILY_DIAGNOSTIC_JAR")
	if path == "" {
		t.Skip("explicit static diagnostic input")
	}
	var dependencies []*zip.ReadCloser
	for _, path := range filepath.SplitList(os.Getenv("JDEC_FAMILY_DIAGNOSTIC_CP")) {
		z, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		dependencies = append(dependencies, z)
	}
	resolve := func(name string) ([]byte, bool) {
		for _, z := range dependencies {
			for _, f := range z.File {
				if f.Name == name+".class" {
					r, e := f.Open()
					if e != nil {
						return nil, false
					}
					defer r.Close()
					b, e := io.ReadAll(r)
					return b, e == nil
				}
			}
		}
		return nil, false
	}
	a, err := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, resolve)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	z := a.source.jar
	name := os.Getenv("JDEC_FAMILY_DIAGNOSTIC_OWNER")
	raw, err := z.readJarEntry(name + ".class")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := z.nativeMemberReader(obj)
	p := d.planNativeMemberFamily()
	t.Log("planned family", p != nil)
	if p != nil {
		for n, c := range p.children {
			t.Log(n, "constructors", len(c.constructors), "bridges", len(c.accessBridges))
		}
	}
	source, err := a.ReadFile(name + ".class")
	t.Log("source", err, len(source))
}
