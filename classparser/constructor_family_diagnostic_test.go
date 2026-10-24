package javaclassparser

import (
	"os"
	"testing"
)

func TestConstructorFamilyDiagnostic(t *testing.T) {
	path := os.Getenv("JDEC_FAMILY_DIAGNOSTIC_JAR")
	if path == "" {
		t.Skip("explicit static diagnostic input")
	}
	a, err := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, nil)
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
