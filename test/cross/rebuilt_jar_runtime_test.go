package cross

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func rebuiltFixtureJar(t *testing.T, file string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func rebuiltCompileFixture(t *testing.T, dir, name, source string, release string) []byte {
	t.Helper()
	file := filepath.Join(dir, name+".java")
	if err := os.WriteFile(file, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(lookJavac(t), "-proc:none", "--release", release, "-d", dir, file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile authored fixture: %v\n%s", err, out)
	}
	return readClass(t, dir, name+".class")
}

func rebuiltRuntimeTag(t *testing.T, driverDir, jar string) string {
	t.Helper()
	// This helper executes authored fixtures only. A null loader parent ensures
	// that neither the Driver class path nor an original target supplies a class
	// that is absent from the tested JAR.
	cmd := exec.Command(lookJava(t), "-Xverify:all", "-cp", driverDir, "RebuiltJarDriver", jar)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("authored JAR runtime: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRebuiltJarRuntimeSelectionPreservesOriginalAuthority(t *testing.T) {
	base := rebuiltCompileFixture(t, t.TempDir(), "RebuiltTag", `public class RebuiltTag {public static String value(){return "BASE8";}}`, "8")
	version := rebuiltCompileFixture(t, t.TempDir(), "RebuiltTag", `public class RebuiltTag {public static String value(){return "MR9";}}`, "9")
	driverDir := t.TempDir()
	driver := rebuiltCompileFixture(t, driverDir, "RebuiltJarDriver", `import java.io.File;import java.net.URL;import java.net.URLClassLoader;
public class RebuiltJarDriver {public static void main(String[]args)throws Exception{try(URLClassLoader loader=new URLClassLoader(new URL[]{new File(args[0]).toURI().toURL()},null)){System.out.println(loader.loadClass("RebuiltTag").getMethod("value").invoke(null));}}}`, "8")
	driverSHA := sha256.Sum256(driver)
	const marker = "Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n"
	for _, world := range []struct {
		name, manifest, want string
		multi                bool
	}{
		{"absent", "", "BASE8", false},
		{"true", marker, "MR9", true},
		{"case insensitive", "Manifest-Version: 1.0\nmUlTi-ReLeAsE: TrUe\n\n", "MR9", true},
		{"false", "Manifest-Version: 1.0\r\nMulti-Release: false\r\n\r\n", "BASE8", false},
		{"entry section cannot authorize", "Manifest-Version: 1.0\r\n\r\nName: p\r\nMulti-Release: true\r\n\r\n", "BASE8", false},
		{"main false entry true", "Manifest-Version: 1.0\r\nMulti-Release: false\r\n\r\nName: p\r\nMulti-Release: true\r\n\r\n", "BASE8", false},
		{"fold without contiguous witness", "Manifest-Version: 1.0\r\nMulti-Release: tr\r\n ue\r\n\r\n", "BASE8", false},
		{"fold and entry witness", "Manifest-Version: 1.0\r\nMulti-Release: tr\r\n ue\r\n\r\nName: p\r\nMulti-Release: true\r\n\r\n", "MR9", true},
		{"runtime metadata only", "Manifest-Version: 1.0\r\nMulti-Release: true\r\nClass-Path: original-target.jar\r\nMain-Class: Untrusted\r\n\r\nName: RebuiltTag.class\r\nSHA-256-Digest: obsolete\r\n\r\n", "MR9", true},
	} {
		t.Run(world.name, func(t *testing.T) {
			root := t.TempDir()
			original := filepath.Join(root, "original.jar")
			entries := map[string][]byte{"RebuiltTag.class": base, "META-INF/versions/9/RebuiltTag.class": version,
				"META-INF/versions/9/META-INF/MANIFEST.MF": []byte(marker), "META-INF/OLD.SF": []byte("obsolete"), "original-resource": []byte("original")}
			if world.manifest != "" {
				entries["META-INF/MANIFEST.MF"] = []byte(world.manifest)
			}
			rebuiltFixtureJar(t, original, entries)
			if got := rebuiltRuntimeTag(t, driverDir, original); got != world.want {
				t.Fatalf("original runtime %q, want %q", got, world.want)
			}
			classes := filepath.Join(root, "classes")
			for name, data := range map[string][]byte{"RebuiltTag.class": base, "META-INF/versions/9/RebuiltTag.class": version,
				"META-INF/MANIFEST.MF": []byte("Multi-Release: false\r\n\r\n"), "META-INF/OLD.SF": []byte("stale"), "unrelated-resource": []byte("not rebuilt")} {
				file := filepath.Join(classes, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			rebuilt := filepath.Join(root, "rebuilt.jar")
			zipRecompiledClassesToJar(t, classes, rebuilt, original)
			if got := rebuiltRuntimeTag(t, driverDir, rebuilt); got != world.want {
				t.Fatalf("rebuilt runtime %q, want original %q", got, world.want)
			}
			z, err := zip.OpenReader(rebuilt)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			var actual []string
			for _, file := range z.File {
				actual = append(actual, file.Name)
				if file.Name == "META-INF/MANIFEST.MF" {
					r, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(r)
					closeErr := r.Close()
					if err != nil || closeErr != nil || !bytes.Equal(data, []byte(marker)) {
						t.Fatalf("rebuilt manifest is not the minimal authorized marker: %q / %v / %v", data, err, closeErr)
					}
				}
			}
			sort.Strings(actual)
			want := []string{"META-INF/versions/9/RebuiltTag.class", "RebuiltTag.class"}
			if world.multi {
				want = append(want, "META-INF/MANIFEST.MF")
				sort.Strings(want)
			}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("rebuilt entries %v, want exactly rebuilt classes plus authorized metadata %v", actual, want)
			}
			if got := sha256.Sum256(readClass(t, driverDir, "RebuiltJarDriver.class")); got != driverSHA {
				t.Fatal("Driver changed between original and rebuilt")
			}
		})
	}
	t.Run("marker cannot replace a missing version implementation", func(t *testing.T) {
		root := t.TempDir()
		original := filepath.Join(root, "original.jar")
		rebuiltFixtureJar(t, original, map[string][]byte{"META-INF/MANIFEST.MF": []byte(marker), "RebuiltTag.class": base, "META-INF/versions/9/RebuiltTag.class": version})
		classes := filepath.Join(root, "classes")
		if err := os.MkdirAll(classes, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(classes, "RebuiltTag.class"), base, 0644); err != nil {
			t.Fatal(err)
		}
		rebuilt := filepath.Join(root, "rebuilt.jar")
		zipRecompiledClassesToJar(t, classes, rebuilt, original)
		if got := rebuiltRuntimeTag(t, driverDir, original); got != "MR9" {
			t.Fatal("original versioned behavior", got)
		}
		if got := rebuiltRuntimeTag(t, driverDir, rebuilt); got != "BASE8" {
			t.Fatal("missing rebuilt implementation was masked", got)
		}
	})
}

func TestRebuiltJarRejectsUnknownMetadataAndKeepsPlainPacking(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, "unknown.jar")
	rebuiltFixtureJar(t, unknown, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\nMalformed\r\n\r\n")})
	if marker, err := rebuiltJarManifest(unknown); err == nil || len(marker) != 0 {
		t.Fatalf("unknown main attributes silently treated as authority: %q / %v", marker, err)
	}
	classes := filepath.Join(root, "classes")
	if err := os.MkdirAll(classes, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(classes, "Authored.class"), []byte("authored fixture bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, "plain.jar")
	zipClassesToJar(t, classes, plain)
	z, err := zip.OpenReader(plain)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 1 || z.File[0].Name != "Authored.class" {
		t.Fatal("plain authored packing acquired runtime metadata")
	}
}
