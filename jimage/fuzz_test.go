package jimage_test

import (
	"testing"

	"github.com/yaklang/javajive/internal/jimagefixture"
	"github.com/yaklang/javajive/jimage"
)

func FuzzOpenBytes(f *testing.F) {
	fixture, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe, 1, 2, 3, 4}},
		{Path: "/test/conf/app.properties", Data: []byte("key=value\n")},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(fixture)
	f.Add([]byte{0xda, 0xda, 0xfe, 0xca})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		reader, err := jimage.OpenBytes(data)
		if err != nil {
			return
		}
		defer reader.Close()
		for index, entry := range reader.Entries() {
			if index >= 32 {
				break
			}
			if entry.Directory {
				continue
			}
			_, _ = reader.Read(entry.Path)
		}
	})
}
