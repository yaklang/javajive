package cross

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	flowOnce sync.Once
	flowDir  string
)

// jdkFlowDir extracts the java.util.concurrent.Flow class files (Flow + Flow$Publisher /
// Flow$Subscriber / Flow$Processor / Flow$Subscription) from the running JDK's jrt:/ image into a
// temporary classpath root. java.util.concurrent.Flow is a Java 9+ API absent from the --release 8
// ct.sym, so any jar whose dependency API references Flow.Publisher (e.g. mutiny's
// UniConvert.toPublisher / UniCreate.publisher(Flow.Publisher) / MultiCreate.publisher(Flow.Publisher))
// fails to compile with "cannot access Flow; class file for java.util.concurrent.Flow not found" --
// the same shape as sun.misc.Unsafe (jdk_sunmisc_test.go) and jdk.jfr (jdk_jfr_test.go). Extracting
// the real class files and placing them on the classpath makes the symbol visible to javac without
// changing any decompiled source. No-op (returns "") when the JDK lacks the Flow classes or
// extraction fails, leaving the false-positive in place.
func jdkFlowDir(t *testing.T) string {
	t.Helper()
	flowOnce.Do(func() {
		java, err := exec.LookPath("java")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-flow-")
		if err != nil {
			return
		}
		ext := filepath.Join(dir, "JdecFlowExtract.java")
		const src = `import java.nio.file.*;
public class JdecFlowExtract {
  public static void main(String[] a) throws Exception {
    FileSystem fs = FileSystems.getFileSystem(java.net.URI.create("jrt:/"));
    Path root = fs.getPath("/modules/java.base/java/util/concurrent");
    if (!Files.exists(root)) return;
    Files.walk(root).filter(Files::isRegularFile).filter(p -> p.toString().contains("Flow")).forEach(p -> {
      try {
        Path rel = fs.getPath("/modules/java.base").relativize(p);
        Path out = java.nio.file.Paths.get(a[0]).resolve(rel.toString());
        Files.createDirectories(out.getParent());
        Files.copy(p, out, StandardCopyOption.REPLACE_EXISTING);
      } catch (Exception e) { throw new RuntimeException(e); }
    });
  }
}`
		if err := os.WriteFile(ext, []byte(src), 0o644); err != nil {
			return
		}
		cmd := exec.Command(java, ext, dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("java.util.concurrent.Flow completion unavailable (Flow stays a --release 8 false-positive): %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "java", "util", "concurrent", "Flow.class")); err != nil {
			t.Logf("Flow completion produced no Flow.class; leaving java.util.concurrent.Flow unresolved")
			return
		}
		flowDir = dir
	})
	return flowDir
}

// withFlow prepends the extracted java.util.concurrent.Flow classpath root (no-op when unavailable
// or empty). Prepending is harmless for jars that do not reference Flow.
func withFlow(t *testing.T, classpath string) string {
	d := jdkFlowDir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}
