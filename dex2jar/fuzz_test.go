package dex2jar_test

import (
	"testing"

	"github.com/yaklang/javajive/dex2jar"
)

func FuzzDexToJarNeverPanics(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("dex\n035\x00"),
		[]byte("PK\x03\x04"),
		[]byte("not a dex or archive"),
		{0xca, 0xfe, 0xba, 0xbe},
	} {
		f.Add(seed)
	}
	options := dex2jar.Options{
		MaxDEXSize:      1 << 20,
		MaxAPKSize:      2 << 20,
		MaxTotalDEXSize: 2 << 20,
		MaxDEXFiles:     8,
		MaxNestedAPKs:   8,
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = dex2jar.DexToJar(data, options)
		_, _, _ = dex2jar.DexToJarWithOptions(data, dex2jar.ConvertOptions{
			Input:  options,
			Policy: dex2jar.FailureStubUnsupportedMethods,
		})
	})
}
