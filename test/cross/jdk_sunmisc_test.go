package cross

// sun.misc completion for the recompile metric.
//
// sun.misc.Unsafe (and a few sun.misc siblings) live in the JDK's `jdk.unsupported` module. javac
// `--release 8` compiles against ct.sym, whose JDK-8 platform description does NOT export the internal
// `sun.*` packages. So ANY faithfully-decompiled class that imports sun.misc.Unsafe fails with
// `package sun.misc does not exist` -- guava's Striped64 / LittleEndianByteArray$UnsafeByteArray /
// UnsignedBytes$...$UnsafeComparator / AbstractFuture$UnsafeAtomicHelper (45 references in
// guava-28.2-android). This is an ENVIRONMENT false-positive that hits EVERY faithful decompiler
// (CFR/Vineflower emit the same import), NOT a JavaJive decompile defect.
//
// To measure REAL defects we "complete" the missing package: extract the running JDK's own sun.misc
// classes out of the jrt image and put that dir on the classpath for ALL tools equally. A
// classpath-supplied sun.misc IS honored under `--release 8` (the release-N platform only fixes the
// boot/system classes; the application classpath is still searched for non-platform packages), so the
// import resolves and the class compiles iff it was decompiled correctly. Extraction runs once per
// process; on any failure we silently fall back to the old behavior (sun.misc stays a false-positive).

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	sunMiscOnce sync.Once
	sunMiscDir  string // extracted sun.misc classpath root, or "" when extraction is unavailable
)

// jdkSunMiscDir extracts sun/misc/* from the running JDK (jrt:/modules/{jdk.unsupported,java.base})
// into a process-cached temp dir and returns it (or "" if java is missing / extraction fails). The
// returned dir is a classpath root containing sun/misc/Unsafe.class etc.
func jdkSunMiscDir(t *testing.T) string {
	t.Helper()
	sunMiscOnce.Do(func() {
		java, err := exec.LookPath("java")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-sunmisc-")
		if err != nil {
			return
		}
		// Single-file source program (JDK 11+ `java Foo.java`) copies sun/misc/* out of the jrt image.
		ext := filepath.Join(dir, "JdecSunMiscExtract.java")
		const src = `import java.nio.file.*;
public class JdecSunMiscExtract {
  public static void main(String[] a) throws Exception {
    FileSystem fs = FileSystems.getFileSystem(java.net.URI.create("jrt:/"));
    Path base = Paths.get(a[0]);
    for (String mod : new String[]{"jdk.unsupported","java.base"}) {
      Path root = fs.getPath("/modules/"+mod+"/sun/misc");
      if (!Files.exists(root)) continue;
      Files.walk(root).filter(Files::isRegularFile).forEach(p -> {
        try {
          Path rel = fs.getPath("/modules/"+mod).relativize(p);
          Path out = base.resolve(rel.toString());
          Files.createDirectories(out.getParent());
          Files.copy(p, out, StandardCopyOption.REPLACE_EXISTING);
        } catch (Exception e) { throw new RuntimeException(e); }
      });
    }
  }
}`
		if err := os.WriteFile(ext, []byte(src), 0o644); err != nil {
			return
		}
		cmd := exec.Command(java, ext, dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("sun.misc completion unavailable (sun.misc.Unsafe stays a --release 8 false-positive): %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "sun", "misc", "Unsafe.class")); err != nil {
			t.Logf("sun.misc completion produced no Unsafe.class; leaving sun.misc unresolved")
			return
		}
		sunMiscDir = dir
	})
	return sunMiscDir
}

// withSunMisc prepends the extracted sun.misc classpath root to classpath (no-op when unavailable or
// already empty). Prepending is harmless for jars that do not import sun.misc.
func withSunMisc(t *testing.T, classpath string) string {
	d := jdkSunMiscDir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}
