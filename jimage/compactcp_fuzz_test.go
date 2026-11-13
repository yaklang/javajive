package jimage

import "testing"

func FuzzDecompressCompactCP(f *testing.F) {
	compressed, _, reader := compactCPFixture()
	f.Add(compressed)
	f.Add(append(compactCPHeader()[:8], 0, 2, compactCPExternalizedString, 0xA1))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = reader.decompressCompactCP(data, 1<<20, "fuzz")
	})
}
