// Package coremail_test holds public-entry regression witnesses for source
// preparation.
//
// These witnesses compile repository-owned fixtures, then mutate only the
// bytecode protocol under test (attributes, access flags, constant-pool names)
// before decompiling. They are not part of test/cross: that harness is the JDK
// oracle for the vendored JAR corpus, while these fixtures need direct
// constant-pool edits between compile and decompile.
//
// Per HARNESS-WORKFLOW.md a missing JDK skips the witness instead of failing,
// so `go test ./...` stays green on machines without Java.
package coremail_test

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func tool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is required for this bytecode-protocol witness: %v", name, err)
	}
	return p
}

func command(t *testing.T, exe string, args ...string) string {
	t.Helper()
	out, err := exec.Command(exe, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", exe, args, err, out)
	}
	return string(out)
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func put(t *testing.T, p string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func archive(t *testing.T, dir string, entries map[string][]byte) string {
	t.Helper()
	p := filepath.Join(dir, "fixture.jar")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for n, b := range entries {
		w, e := z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(b); e != nil {
			t.Fatal(e)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
