package javaclassparser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This oracle only opens authored resource ZIPs with the JDK's JarFile. It
// never loads or executes code from an archive. A single JVM observes every
// world, independently of the production parser and its physical-line rules.
const jarManifestDetectorOracle = `import java.io.File; import java.util.jar.JarFile; import java.util.zip.ZipFile;
public class ManifestDetectorOracle {
 public static void main(String[]args)throws Exception {
  for(int i=0;i<args.length;i++)try(JarFile jar=new JarFile(new File(args[i]),false,ZipFile.OPEN_READ,Runtime.version())){
   System.out.println(i+":"+jar.isMultiRelease());
  }
 }
}`

func TestAdversarialJarReleaseSelectionJDKPhysicalStreamOracle(t *testing.T) {
	javac, java := requireJDK(t)
	root := t.TempDir()
	oracle := filepath.Join(root, "ManifestDetectorOracle.java")
	if err := os.WriteFile(oracle, []byte(jarManifestDetectorOracle), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, javac, "-proc:none", "-d", root, oracle).CombinedOutput(); err != nil {
		t.Fatalf("compile authored metadata oracle: %v\n%s", err, out)
	}
	type world struct {
		name        string
		raw         []byte
		invalidLine bool
	}
	var worlds []world
	const main = "Manifest-Version: 1.0\n"
	prefix := func(size int) string {
		s := main
		remaining := size - len(s)
		if remaining%2 == 1 {
			s += "  \n"
			remaining -= 3
		}
		return s + strings.Repeat(" \n", remaining/2)
	}
	for _, newline := range []string{"\n", "\r", "\r\n"} {
		for _, lineBytes := range []int{254, 255, 256, 509, 510, 511, 512} {
			for _, start := range []int{len(main), 7680, 7681, 7682} {
				text := prefix(start) + "X: " + strings.Repeat("a", lineBytes-3) + newline + "Multi-Release: true" + newline + newline
				worlds = append(worlds, world{fmt.Sprintf("newline=%x/line=%d/start=%d", newline, lineBytes, start), []byte(text), lineBytes >= 512})
			}
		}
		for _, tail := range []struct{ name, text string }{
			{"unterminated flag", main + "Multi-Release: true"},
			{"unterminated extra header", main + "Multi-Release: true" + newline + "X: tail"},
			{"uncommitted replacement", main + "Multi-Release: true" + newline + "Multi-Release: fal" + newline + " se"},
			{"uncommitted continuation", main + "Multi-Release: true" + newline + " "},
		} {
			worlds = append(worlds, world{fmt.Sprintf("newline=%x/%s", newline, tail.name), []byte(tail.text), false})
		}
	}
	args := []string{"-cp", root, "ManifestDetectorOracle"}
	for i, w := range worlds {
		jar := filepath.Join(root, fmt.Sprintf("world-%d.jar", i))
		if err := os.WriteFile(jar, jarReleaseTestZIP(t, []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", w.raw}}), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, jar)
	}
	out, err := exec.CommandContext(ctx, java, args...).Output()
	if err != nil {
		t.Fatalf("authored metadata oracle: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(worlds) {
		t.Fatalf("oracle returned %d/%d worlds", len(lines), len(worlds))
	}
	for i, w := range worlds {
		t.Run(w.name, func(t *testing.T) {
			id, flag, ok := strings.Cut(lines[i], ":")
			want, err := strconv.ParseBool(flag)
			if !ok || id != strconv.Itoa(i) || err != nil {
				t.Fatalf("invalid independent observation %q", lines[i])
			}
			got, err := jarMainMultiRelease(w.raw)
			if w.invalidLine {
				if want || err == nil {
					t.Fatalf("oversized physical line incorrectly known: original=%v selected=%v err=%v", want, got, err)
				}
				return
			}
			if err != nil || got != want {
				t.Fatalf("original JDK=%v selected=%v err=%v", want, got, err)
			}
		})
	}
}
