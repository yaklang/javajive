package cross

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

func testHistoricalConstructorLoweringRoundTrip(t *testing.T) {
	originalDir, sourceDir := t.TempDir(), t.TempDir()
	fixture := `class AuditCtorWitness {static Object token;static long wide;static AuditCtorParent escaped;static RuntimeException error;}
abstract class AuditCtorParent {AuditCtorParent(Object input){AuditCtorWitness.escaped=this;if(token()!=AuditCtorWitness.token||wide()!=AuditCtorWitness.wide)throw new AssertionError("early capture");if(input==null)throw AuditCtorWitness.error;}abstract Object token();abstract long wide();}
public class AuditCtorChild extends AuditCtorParent {final Object token;final long wide;public AuditCtorChild(Object input,Object token,long wide){super(input);this.token=token;this.wide=wide;}Object token(){return token;}long wide(){return wide;}}
class AuditCtorCaller {public static void main(String[]args){int rows=0;for(Object token:new Object[]{new Object(),null})for(long wide:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int fail=0;fail<2;fail++){AuditCtorWitness.token=token;AuditCtorWitness.wide=wide;AuditCtorWitness.error=new IllegalArgumentException("cause");AuditCtorWitness.escaped=null;Throwable error=null;try{new AuditCtorChild(fail==0?new Object():null,token,wide);}catch(Throwable e){error=e;}if(error!=(fail==0?null:AuditCtorWitness.error)||AuditCtorWitness.escaped==null||AuditCtorWitness.escaped.token()!=token||AuditCtorWitness.escaped.wide()!=wide)throw new AssertionError("binding/publication/abrupt",error);rows++;}System.out.println(rows+":owned:lowered:packed");}}`
	path := filepath.Join(originalDir, "AuditCtorChild.java")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(lookJavac(t), "-proc:none", "--release", "8", "-g:none", "-d", originalDir, path).CombinedOutput(); err != nil {
		t.Fatal("authored original", err, string(log))
	}
	obj, err := classparser.Parse(readClass(t, originalDir, "AuditCtorChild.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range obj.Fields {
		field.AccessFlags |= 0x1000
	}
	ctors := 0
	for _, method := range obj.Methods {
		for _, attr := range method.Attributes {
			code, ok := attr.(*classparser.CodeAttribute)
			if !ok || len(code.Code) != 16 || !bytes.Equal(code.Code[:3], []byte{0x2a, 0x2b, 0xb7}) {
				continue
			}
			ctors++
			fresh := bytes.Clone(code.Code)
			copy(code.Code, fresh[5:15])
			copy(code.Code[10:], fresh[:5])
		}
	}
	if ctors != 1 {
		t.Fatal("independent authored constructor packet", ctors)
	}
	if err := os.WriteFile(filepath.Join(originalDir, "AuditCtorChild.class"), obj.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(target string) ([]byte, error) {
		return exec.Command(lookJava(t), "-Xverify:all", "-cp", target, "AuditCtorCaller").CombinedOutput()
	}
	const want = "16:owned:lowered:packed\n"
	if out, err := run(originalDir); err != nil || string(out) != want {
		t.Fatal("independent original oracle", err, string(out))
	}
	jar := filepath.Join(t.TempDir(), "original.jar")
	rebuiltFixtureJar(t, jar, map[string][]byte{"AuditCtorChild.class": obj.Bytes()})
	a, err := classparser.NewJarReconstructionArtifactFromLocal(jar, 8, classparser.ModernJavac, func(binary string) ([]byte, bool) {
		raw, err := os.ReadFile(filepath.Join(originalDir, binary+".class"))
		return raw, err == nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	files, units, failures := decompileAllFromSourceReader(t, jar, sourceDir, 0, a)
	if failures != 0 || units != 1 || len(files) != 1 {
		t.Fatal("actual reconstruction source", units, failures)
	}
	for _, name := range []string{"AuditCtorParent", "AuditCtorWitness", "AuditCtorCaller"} {
		if err := os.WriteFile(filepath.Join(sourceDir, name+".class"), readClass(t, originalDir, name+".class"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if log, err := exec.Command(lookJavac(t), append([]string{"-proc:none", "--release", "8", "-g:none", "-cp", sourceDir, "-sourcepath", sourceDir, "-d", sourceDir}, files...)...).CombinedOutput(); err != nil {
		t.Fatal("compile with owned original absent", err, string(log))
	}
	if out, err := run(sourceDir); err == nil || !strings.Contains(string(out), "early capture") {
		t.Fatal("unlowered runtime negative control", err, string(out))
	}
	// Only the owned child participates in the postcompile transaction. The
	// independently compiled caller/dependency bytes cannot authorize edits.
	classes := t.TempDir()
	if err := os.WriteFile(filepath.Join(classes, "AuditCtorChild.class"), readClass(t, sourceDir, "AuditCtorChild.class"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := historicalFinishArtifactClasses("explicit-reconstruction-artifact-v1", a.FinishClass, classes, t.TempDir())
	if err != nil || len(report.Failures) != 0 || len(report.RestoredClasses) != 1 {
		t.Fatal("actual postcompile and quarantine path", report, err)
	}
	for _, name := range []string{"AuditCtorParent", "AuditCtorWitness", "AuditCtorCaller"} {
		if err := os.WriteFile(filepath.Join(classes, name+".class"), readClass(t, originalDir, name+".class"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	packed := filepath.Join(t.TempDir(), "rebuilt.jar")
	zipRecompiledClassesToJar(t, classes, packed, jar)
	if out, err := run(packed); err != nil || string(out) != want {
		t.Fatal("fresh lowered and packed runtime", err, string(out))
	}
}
