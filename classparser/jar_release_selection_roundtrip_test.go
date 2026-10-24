package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/internal/filesys"
)

func TestAdversarialJarReleaseSelectionUnchangedCallerRoundTrip(t *testing.T) {
	_, java := requireJDK(t)
	base, variant := jarReleaseChoiceBytes(t)
	driver := jarReleaseTestHex(t, jarReleaseDriverHex)
	driverDir := t.TempDir()
	driverFile := filepath.Join(driverDir, "mrdriver", "MRDriver.class")
	if err := os.MkdirAll(filepath.Dir(driverFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driverFile, driver, 0600); err != nil {
		t.Fatal(err)
	}
	driverSHA := sha256.Sum256(driver)
	defer func() {
		current, err := os.ReadFile(driverFile)
		if err != nil || sha256.Sum256(current) != driverSHA {
			t.Error("original binary caller changed during a failed or completed world")
		}
	}()
	run := func(t *testing.T, phase, classpath string, target int, word, marker string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, java, "-Xverify:all", "-Djdk.util.jar.version="+strconv.Itoa(target), "-cp", classpath+string(os.PathListSeparator)+driverDir, "mrdriver.MRDriver", word, marker)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.HasPrefix(string(out), "sentinel="+word+" marker="+marker+" resource=") {
			t.Fatalf("%s unchanged binary caller on selected source: %v\n%s", phase, err, out)
		}
	}
	for _, world := range []struct {
		name, manifest string
		multi          bool
	}{
		{"missing", "", false},
		{"true", "Manifest-Version: 1.0\nMulti-Release: true\n\n", true},
		{"false", "Manifest-Version: 1.0\nMulti-Release: false\n\n", false},
		{"entry alone", "Manifest-Version: 1.0\n\nName: x\nMulti-Release: true\n\n", false},
		{"fold without witness", "Manifest-Version: 1.0\nMulti-Release: tr\n ue\n\n", false},
		{"fold and entry witness", "Manifest-Version: 1.0\nMulti-Release: tr\n ue\n\nName: x\nMulti-Release: true\n\n", true},
	} {
		for _, target := range []int{8, 9, 21} {
			t.Run(fmt.Sprintf("%s/target%d", world.name, target), func(t *testing.T) {
				entries := []jarReleaseTestEntry{{"mrprobe/Choice.class", base}, {"META-INF/versions/9/mrprobe/Choice.class", variant}}
				if world.manifest != "" {
					entries = append(entries, jarReleaseTestEntry{"META-INF/MANIFEST.MF", []byte(world.manifest)})
				}
				raw := jarReleaseTestZIP(t, entries)
				jar := filepath.Join(t.TempDir(), "original.jar")
				if err := os.WriteFile(jar, raw, 0600); err != nil {
					t.Fatal(err)
				}
				word, marker, release, want := "BASE8", "8", "8", base
				if world.multi && target >= 9 {
					word, marker, release, want = "MR9", "9", "9", variant
				}
				// First prove the unchanged original caller on the original JAR.
				run(t, "original", jar, target, word, marker)
				zip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), filesys.WithTargetRelease(target))
				if err != nil {
					t.Fatal(err)
				}
				z := NewJarFSWithOptions(zip, false)
				source, err := z.ReadFile("mrprobe/Choice.class")
				if err != nil || strings.Contains(string(source), DecompileStubMarker) {
					t.Fatalf("selected source %v\n%s", err, source)
				}
				compiled := nativeCompileSourceReleaseClasses(t, map[string]string{"mrprobe/Choice.java": string(source)}, "none", release)
				rebuilt, ok := compiled["mrprobe/Choice.class"]
				if !ok || len(compiled) != 1 {
					t.Fatalf("source produced %d classes; expected only the selected Choice", len(compiled))
				}
				if nativeBinaryShape(t, rebuilt) != nativeBinaryShape(t, want) {
					t.Fatal("selected original class ABI changed")
				}
				nativeLexicalExactSignatures(t, "mrprobe/Choice", want, rebuilt)
				out := t.TempDir()
				file := filepath.Join(out, "mrprobe", "Choice.class")
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, rebuilt, 0600); err != nil {
					t.Fatal(err)
				}
				// The rebuilt classpath contains generated classes and the original
				// Driver only. It contains neither the target JAR nor original Choice.
				run(t, "rebuilt", out, target, word, marker)
				current, err := os.ReadFile(driverFile)
				if err != nil || sha256.Sum256(current) != driverSHA {
					t.Fatal("original binary caller changed")
				}
			})
		}
	}
}
