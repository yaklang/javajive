// Package testfixtures locates the shared external container samples under
// testdata/external. Tests in any package can use it without hard-coding a
// relative path depth.
package testfixtures

import (
	"os"
	"path/filepath"
)

// Dir returns the absolute path of testdata/external.
func Dir() string {
	if workingDir, err := os.Getwd(); err == nil {
		dir := workingDir
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return filepath.Join(dir, "testdata", "external")
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Fall back to the conventional location relative to the test process.
	return filepath.Join("testdata", "external")
}

// Path returns one external sample by name.
func Path(name string) string {
	return filepath.Join(Dir(), name)
}
