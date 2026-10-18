package cross

import (
	"context"
	"fmt"
	"testing"
	"time"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
)

// A versioned directory does not make a multi-release JAR. Ask the same
// original-archive metadata authority used by the production logical reader,
// then synthesize only the marker needed to preserve runtime class selection.
// In particular, copying Class-Path or signatures could mask missing rebuilt
// classes or claim authenticity for bytecode that has changed.
func rebuiltJarManifest(originalJar string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	z, err := filesys.NewZipFSFromLocalWithOptions(originalJar,
		filesys.WithArchiveLimits(filesys.ArchiveLimits{MaxArchiveEntries: 65535}),
		filesys.WithArchiveContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("original JAR metadata: %w", err)
	}
	defer z.Close()
	multi, err := javaclassparser.NewJarFSWithOptions(z, false).RuntimeMultiRelease()
	if err != nil {
		return nil, fmt.Errorf("original JAR multi-release authority: %w", err)
	}
	if !multi {
		return nil, nil
	}
	return []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n"), nil
}

func zipRecompiledClassesToJar(t *testing.T, dir, jarPath, originalJar string) {
	t.Helper()
	manifest, err := rebuiltJarManifest(originalJar)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeClassTreeJar(dir, jarPath, manifest); err != nil {
		t.Fatalf("build rebuilt jar: %v", err)
	}
}
