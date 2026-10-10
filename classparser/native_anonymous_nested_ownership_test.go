package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeNestedAnonymousOwnershipRequiresOriginalEnclosingMethod(t *testing.T) {
	fixture := `class NativeArchiveOwner {static Runnable make(){return new Runnable(){public void run(){new Runnable(){public void run(){}}.run();}};}}`
	files := nativeCompileClasses(t, fixture)
	for _, kind := range []string{"original", "missing enclosing declaration", "foreign enclosing owner"} {
		t.Run(kind, func(t *testing.T) {
			copyFiles := make(map[string][]byte, len(files))
			for n, raw := range files {
				copyFiles[n] = append([]byte(nil), raw...)
			}
			obj, err := Parse(copyFiles["NativeArchiveOwner$1$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for i, a := range obj.Attributes {
				if enclosing, ok := a.(*UnparsedAttribute); ok && enclosing != nil && enclosing.Name == "EnclosingMethod" {
					found = true
					switch kind {
					case "missing enclosing declaration":
						obj.Attributes = append(obj.Attributes[:i], obj.Attributes[i+1:]...)
					case "foreign enclosing owner":
						cp := NewConstantPoolWithConstant(&obj.ConstantPool)
						idx := uint16(cp.AddNewClassInfo("UnknownOwner"))
						enclosing.Info = append([]byte(nil), enclosing.Info...)
						enclosing.Info[0] = byte(idx >> 8)
						enclosing.Info[1] = byte(idx)
					}
					break
				}
			}
			if !found {
				t.Fatal("original nesting metadata missing")
			}
			copyFiles["NativeArchiveOwner$1$1.class"] = obj.Bytes()
			archive := nativeArchive(t, copyFiles)
			defer archive.Close()
			src, err := archive.ReadFile("NativeArchiveOwner.class")
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(string(src), "jdec-owned-anonymous-ordinal:")
			if got != (kind == "original") {
				t.Fatalf("complete original nested ownership admitted=%v source=%s", got, src)
			}
			child, err := archive.ReadFile("NativeArchiveOwner$1.class")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(child), "original anonymous body owned by") != (kind == "original") {
				t.Fatal("suppressed child with missing original nesting evidence")
			}
		})
	}
}
