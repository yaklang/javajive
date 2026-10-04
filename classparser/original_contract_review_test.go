package javaclassparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// These checks read original metadata; they never execute third-party classes.
// JVM behavior counterexamples live in the trusted adversarial roundtrip tests.
func originalJarClassForReview(t *testing.T, relative, entry string) []byte {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	jar, err := zip.OpenReader(filepath.Join(home, ".m2/repository", relative))
	if err != nil {
		t.Skipf("optional historical jar: %v", err)
	}
	defer jar.Close()
	for _, f := range jar.File {
		if f.Name != entry {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("missing original class %s", entry)
	return nil
}

func assertOriginalCatchContract(t *testing.T, raw []byte, switches ...string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	want := map[string]bool{}
	for _, method := range object.Methods {
		for _, attr := range method.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				for _, entry := range code.ExceptionTable {
					if entry.CatchType == 0 {
						t.Fatal("catch-all cleanup needs a separate scoped oracle")
					}
					name := cp.GetClassName(int(entry.CatchType))
					want[name[strings.LastIndex(name, "/")+1:]] = true
				}
			}
		}
	}
	catch := regexp.MustCompile(`catch\s*\(\s*([\w.$]+(?:\s*\|\s*[\w.$]+)*)\s+\w+\s*\)`)
	for _, setting := range []string{"", "1"} {
		for _, key := range switches {
			t.Setenv(key, setting)
		}
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, match := range catch.FindAllStringSubmatch(source, -1) {
			for _, name := range strings.Split(match[1], "|") {
				name = strings.TrimSpace(name)
				got[name[strings.LastIndex(name, ".")+1:]] = true
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("switch=%q: emitted catch types %v differ from original %v\n%s", setting, got, want, source)
		}
	}
}
