package cross

// jdk.jfr completion for the recompile metric (spring-core core/metrics/jfr/*).
//
// Java Flight Recorder (jdk.jfr.*) was added in JDK 9/11 and lives in the `jdk.jfr` module. javac
// `--release 8` compiles against ct.sym, whose JDK-8 platform description does NOT contain jdk.jfr. So
// ANY faithfully-decompiled class that imports jdk.jfr.Event / Category / Label fails with
// `package jdk.jfr does not exist` -- spring-core's FlightRecorderStartupEvent / FlightRecorderStartupStep.
// This is an ENVIRONMENT false-positive that hits EVERY faithful decompiler (CFR/Vineflower emit the
// same import) under --release 8, NOT a JavaJive decompile defect -- the exact same situation as
// sun.misc.Unsafe (see jdk_sunmisc_test.go).
//
// We "complete" the missing package the same way: extract the running JDK's own jdk/jfr classes out of
// the jrt image and put that dir on the classpath. A classpath-supplied jdk.jfr IS honored under
// `--release 8` (the release-N platform only fixes boot/system classes; the application classpath is
// still searched for non-platform packages), so the import resolves and the class compiles iff it was
// decompiled correctly. Extraction runs once per process; on any failure we silently fall back.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	jfrOnce sync.Once
	jfrDir  string // extracted jdk.jfr classpath root, or "" when extraction is unavailable
)

// jdkJfrDir extracts jdk/jfr/* from the running JDK (jrt:/modules/jdk.jfr) into a process-cached temp
// dir and returns it (or "" if java is missing / extraction fails). The returned dir is a classpath
// root containing jdk/jfr/Event.class etc.
func jdkJfrDir(t *testing.T) string {
	t.Helper()
	jfrOnce.Do(func() {
		java, err := exec.LookPath("java")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-jfr-")
		if err != nil {
			return
		}
		ext := filepath.Join(dir, "JdecJfrExtract.java")
		// jdk.jfr.Event extends jdk.internal.event.Event (in java.base), so both packages must be
		// extracted or javac reports "cannot access Event; class file for jdk.internal.event.Event not
		// found" for any Event subclass (spring's FlightRecorderStartupEvent).
		const src = `import java.nio.file.*;
public class JdecJfrExtract {
  public static void main(String[] a) throws Exception {
    FileSystem fs = FileSystems.getFileSystem(java.net.URI.create("jrt:/"));
    Path base = Paths.get(a[0]);
    String[][] roots = {{"jdk.jfr","jdk/jfr"},{"java.base","jdk/internal/event"}};
    for (String[] r : roots) {
      Path root = fs.getPath("/modules/"+r[0]+"/"+r[1]);
      if (!Files.exists(root)) continue;
      Files.walk(root).filter(Files::isRegularFile).forEach(p -> {
        try {
          Path rel = fs.getPath("/modules/"+r[0]).relativize(p);
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
			t.Logf("jdk.jfr completion unavailable (jdk.jfr stays a --release 8 false-positive): %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "jdk", "jfr", "Event.class")); err != nil {
			t.Logf("jdk.jfr completion produced no Event.class; leaving jdk.jfr unresolved")
			return
		}
		jfrDir = dir
	})
	return jfrDir
}

// withJfr prepends the extracted jdk.jfr classpath root to classpath (no-op when unavailable or empty).
// Prepending is harmless for jars that do not import jdk.jfr.
func withJfr(t *testing.T, classpath string) string {
	d := jdkJfrDir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}
