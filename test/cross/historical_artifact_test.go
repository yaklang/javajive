package cross

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

type historicalArtifactMetadata struct {
	Policy          string
	RestoredClasses []string
	Failures        []historicalMetadataFailure
}
type historicalMetadataFailure struct{ Class, Reason string }

// Only successful restored classes reach the runtime JAR. Retain rejected
// compiler output separately for diagnosis; never copy an original class back.
func historicalRestoreArtifactMetadata(artifact *classparser.JarSourceArtifact, classes, rejected string) (*historicalArtifactMetadata, error) {
	report := &historicalArtifactMetadata{Policy: "explicit-source-artifact", RestoredClasses: []string{}, Failures: []historicalMetadataFailure{}}
	err := filepath.WalkDir(classes, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular compiled class %s: %v", path, err)
		}
		relative, err := filepath.Rel(classes, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		compiled, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		restored, metadataError := artifact.RestoreClassMetadata(name, compiled)
		if metadataError != nil {
			report.Failures = append(report.Failures, historicalMetadataFailure{name, metadataError.Error()})
			target := filepath.Join(rejected, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.Rename(path, target)
		}
		if err := os.WriteFile(path, restored, 0644); err != nil {
			return err
		}
		report.RestoredClasses = append(report.RestoredClasses, name)
		return nil
	})
	return report, err
}

func testHistoricalArtifactMetadataRoundTrip(t *testing.T) {
	originalDir, driverDir, classes := t.TempDir(), t.TempDir(), t.TempDir()
	owner := `public class AuditArtifactOwner {public long combine(final long left,long right){return left*31L+right;}}`
	file := filepath.Join(originalDir, "AuditArtifactOwner.java")
	if err := os.WriteFile(file, []byte(owner), 0644); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(lookJavac(t), "-proc:none", "--release", "8", "-parameters", "-g:none", "-d", originalDir, file).CombinedOutput(); err != nil {
		t.Fatal("authored original", err, string(log))
	}
	jar := filepath.Join(t.TempDir(), "original.jar")
	rebuiltFixtureJar(t, jar, map[string][]byte{"AuditArtifactOwner.class": readClass(t, originalDir, "AuditArtifactOwner.class")})
	driver := `public class AuditArtifactDriver {public static void main(String[]args)throws Exception {java.lang.reflect.Parameter[] p=AuditArtifactOwner.class.getDeclaredMethod("combine",long.class,long.class).getParameters();if(p.length!=2||!p[0].isNamePresent()||!p[0].getName().equals("left")||!p[1].getName().equals("right")||p[0].getModifiers()!=16||p[1].getModifiers()!=0)throw new AssertionError("original parameter metadata");int rows=0;AuditArtifactOwner o=new AuditArtifactOwner();for(long a:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(long b:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(b)).longValue();if(o.combine(a,b)!=expected)throw new AssertionError("wide overflow or parameter binding");rows++;}System.out.println(rows+":artifact:parameters:overflow");}}`
	driverFile := filepath.Join(driverDir, "AuditArtifactDriver.java")
	if err := os.WriteFile(driverFile, []byte(driver), 0644); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(lookJavac(t), "-proc:none", "--release", "8", "-g:none", "-cp", jar, "-sourcepath", driverDir, "-d", driverDir, driverFile).CombinedOutput(); err != nil {
		t.Fatal("unchanged binary caller", err, string(log))
	}
	driverHash := sha256.Sum256(readClass(t, driverDir, "AuditArtifactDriver.class"))
	run := func(target string) ([]byte, error) {
		return exec.Command(lookJava(t), "-Xverify:all", "-cp", driverDir+string(os.PathListSeparator)+target, "AuditArtifactDriver").CombinedOutput()
	}
	const expected = "25:artifact:parameters:overflow\n"
	if log, err := run(jar); err != nil || string(log) != expected {
		t.Fatal("independent original oracle", err, string(log))
	}
	artifact, err := classparser.NewJarSourceArtifactFromLocal(jar, 8, classparser.ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	files, units, failures := decompileAllFromSourceReader(t, jar, classes, 0, artifact)
	if failures != 0 || units != 1 || len(files) != 1 {
		t.Fatal("actual artifact source units", units, failures)
	}
	args := []string{"-proc:none", "--release", "8", "-g:none", "-cp", classes, "-sourcepath", classes, "-d", classes}
	if log, err := exec.Command(lookJavac(t), append(args, files...)...).CombinedOutput(); err != nil {
		t.Fatal("original subject absent from compiler classpath", err, string(log))
	}
	if log, err := run(classes); err == nil || !strings.Contains(string(log), "original parameter metadata") {
		t.Fatal("unrestored metadata negative control was inactive", err, string(log))
	}
	rejected := filepath.Join(t.TempDir(), "rejected")
	report, err := historicalRestoreArtifactMetadata(artifact, classes, rejected)
	if err != nil || len(report.RestoredClasses) != 1 || len(report.Failures) != 0 {
		t.Fatal("explicit artifact metadata stage", report, err)
	}
	packed := filepath.Join(t.TempDir(), "rebuilt.jar")
	zipRecompiledClassesToJar(t, classes, packed, jar)
	if log, err := run(packed); err != nil || string(log) != expected {
		t.Fatal("rebuilt metadata and algorithm oracle", err, string(log))
	}
	// An unrelated generated class is retained for diagnosis and excluded from
	// the runtime artifact. The original archive cannot authorize its metadata.
	unowned := rebuiltCompileFixture(t, t.TempDir(), "UnownedAuditClass", "public class UnownedAuditClass {}", "8")
	if err := os.WriteFile(filepath.Join(classes, "UnownedAuditClass.class"), unowned, 0644); err != nil {
		t.Fatal(err)
	}
	report, err = historicalRestoreArtifactMetadata(artifact, classes, rejected)
	if err != nil || len(report.Failures) != 1 || report.Failures[0].Class != "UnownedAuditClass.class" {
		t.Fatal("unowned compiler output was promoted", report, err)
	}
	if data, err := os.ReadFile(filepath.Join(rejected, "UnownedAuditClass.class")); err != nil || sha256.Sum256(data) != sha256.Sum256(unowned) {
		t.Fatal("rejected output not retained exactly", err)
	}
	zipRecompiledClassesToJar(t, classes, packed, jar)
	z, err := zip.OpenReader(packed)
	if err != nil {
		t.Fatal(err)
	}
	if names := historicalRebuiltClassInventory(&z.Reader); len(names) != 1 || names[0] != "AuditArtifactOwner.class" {
		t.Fatal("rejected class leaked into runtime inventory", names)
	}
	z.Close()
	if sha256.Sum256(readClass(t, driverDir, "AuditArtifactDriver.class")) != driverHash {
		t.Fatal("binary caller changed")
	}
	if err := os.Remove(filepath.Join(classes, "AuditArtifactOwner.class")); err != nil {
		t.Fatal(err)
	}
	zipRecompiledClassesToJar(t, classes, packed, jar)
	if log, err := run(packed); err == nil || !strings.Contains(string(log), "AuditArtifactOwner") {
		t.Fatal("a missing rebuilt class was masked by the original", err, string(log))
	}
}
