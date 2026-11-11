package jar

import (
	"archive/zip"
	"bytes"
	"testing"
)

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, data := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestJarParserReusesNestedJarFSAndCloseIsIdempotent(t *testing.T) {
	inner := zipBytes(t, map[string][]byte{"p/value.txt": []byte("payload")})
	outer := zipBytes(t, map[string][]byte{"lib/inner.jar": inner})
	parser, err := NewJarParserFromBytes(outer)
	if err != nil {
		t.Fatal(err)
	}
	first, err := parser.GetNestedJarFS("lib/inner.jar")
	if err != nil {
		t.Fatal(err)
	}
	second, err := parser.GetNestedJarFS("lib/inner.jar")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("nested JarFS cache was not reused")
	}
	data, err := parser.DecompileClass("lib/inner.jar/p/value.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("nested content = %q", data)
	}
	if err := parser.Close(); err != nil {
		t.Fatal(err)
	}
	if err := parser.Close(); err != nil {
		t.Fatal(err)
	}
}
