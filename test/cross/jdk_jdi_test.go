package cross

// jdk.jdi / jdk.attach completion for the recompile metric.
//
// javassist HotSwapper / HotSwapAgent faithfully import com.sun.jdi.* and
// com.sun.tools.attach. javac --release 8 compiles against ct.sym, which does
// not export those JDK-internal packages, so ANY faithful decompiler fails with
// "package com.sun.jdi does not exist". Same class as sun.misc / jdk.jfr:
// extract the running JDK's own classes from the jrt image onto the classpath.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	jdiOnce sync.Once
	jdiDir  string
)

func jdkJdiDir(t *testing.T) string {
	t.Helper()
	jdiOnce.Do(func() {
		java, err := exec.LookPath("java")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-jdi-")
		if err != nil {
			return
		}
		ext := filepath.Join(dir, "JdecJdiExtract.java")
		const src = `import java.nio.file.*;
public class JdecJdiExtract {
  public static void main(String[] a) throws Exception {
    FileSystem fs = FileSystems.getFileSystem(java.net.URI.create("jrt:/"));
    Path base = Paths.get(a[0]);
    String[][] mods = new String[][]{
      {"jdk.jdi","com/sun/jdi"},
      {"jdk.attach","com/sun/tools/attach"},
    };
    for (String[] m : mods) {
      Path root = fs.getPath("/modules/"+m[0]+"/"+m[1]);
      if (!Files.exists(root)) continue;
      Files.walk(root).filter(Files::isRegularFile).forEach(p -> {
        try {
          Path rel = fs.getPath("/modules/"+m[0]).relativize(p);
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
			t.Logf("jdi/attach completion unavailable: %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "com", "sun", "jdi", "VirtualMachine.class")); err != nil {
			t.Logf("jdi completion produced no VirtualMachine.class")
			return
		}
		jdiDir = dir
	})
	return jdiDir
}

func withJdi(t *testing.T, classpath string) string {
	d := jdkJdiDir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}

func TestJdiCompletionExtractsVirtualMachine(t *testing.T) {
	d := jdkJdiDir(t)
	if d == "" {
		t.Skip("jdi extraction unavailable")
	}
	for _, rel := range []string{
		"com/sun/jdi/VirtualMachine.class",
		"com/sun/jdi/event/EventQueue.class",
		"com/sun/tools/attach/VirtualMachine.class",
	} {
		if _, err := os.Stat(filepath.Join(d, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}
