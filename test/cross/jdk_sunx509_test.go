package cross

// sun.security.x509 completion for the recompile metric.
//
// OpenJdkSelfSignedCertGenerator (netty-handler) faithfully imports sun.security.x509.*
// (X509CertInfo / X500Name / CertificateVersion / ...). Those classes live in java.base
// but are not exported; javac --release 8 compiles against ct.sym and reports
// `package sun.security.x509 does not exist`. Same environment false-positive as
// sun.misc.Unsafe (jdk_sunmisc_test.go). Extract the running JDK's own
// sun/security/x509 classes from the jrt image onto the application classpath.

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	sunX509Once sync.Once
	sunX509Dir  string
)

func jdkSunX509Dir(t *testing.T) string {
	t.Helper()
	sunX509Once.Do(func() {
		java, err := exec.LookPath("java")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-sunx509-")
		if err != nil {
			return
		}
		ext := filepath.Join(dir, "JdecSunX509Extract.java")
		const src = `import java.nio.file.*;
public class JdecSunX509Extract {
  public static void main(String[] a) throws Exception {
    FileSystem fs = FileSystems.getFileSystem(java.net.URI.create("jrt:/"));
    Path base = Paths.get(a[0]);
    for (String pkg : new String[]{"sun/security/x509", "sun/security/util"}) {
      Path root = fs.getPath("/modules/java.base/"+pkg);
      if (!Files.exists(root)) continue;
      Files.walk(root).filter(Files::isRegularFile).forEach(p -> {
        try {
          Path rel = fs.getPath("/modules/java.base").relativize(p);
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
			t.Logf("sun.security.x509 completion unavailable: %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "sun", "security", "x509", "X509CertInfo.class")); err != nil {
			t.Logf("sun.security.x509 completion produced no X509CertInfo.class; leaving unresolved")
			return
		}
		sunX509Dir = dir
	})
	return sunX509Dir
}

func withSunX509(t *testing.T, classpath string) string {
	d := jdkSunX509Dir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}

func TestSunX509CompletionExtractsCertInfo(t *testing.T) {
	d := jdkSunX509Dir(t)
	if d == "" {
		t.Skip("sun.security.x509 extraction unavailable on this JDK")
	}
	if _, err := os.Stat(filepath.Join(d, "sun", "security", "x509", "X509CertInfo.class")); err != nil {
		t.Fatalf("expected X509CertInfo.class under %s: %v", d, err)
	}
}
