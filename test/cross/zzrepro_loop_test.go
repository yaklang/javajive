package cross

import (
	"os"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// zztmp: decompile a single .class file path (REPRO_CLASS) and print the source.
func TestZZReproLoop(t *testing.T) {
	p := os.Getenv("REPRO_CLASS")
	if p == "" {
		t.Skip("set REPRO_CLASS=/path/to/X.class")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	src, err := classparser.Decompile(data)
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	t.Logf("\n%s", src)
}
