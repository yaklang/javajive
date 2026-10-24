package cross

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// A producer can emit old class versions against a newer SDK. The actual
// compiler target must also bind the postcompile constructor obligations.
func testHistoricalSourceAndCompilerShareTarget(t *testing.T) {
	for _, owner := range []string{"ApiCaptureOwner", "RenamedApiCaptureOwner"} {
		t.Run(owner, func(t *testing.T) {
			original, support, sources, classes := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			fixture := `class ApiEffects {static Object owner;static long word;}
public class ApiCaptureOwner {static void configure(javax.net.ssl.SSLParameters p){p.setApplicationProtocols(new String[]{"proof"});}Runnable make(final long word){return new Runnable(){public void run(){ApiEffects.owner=ApiCaptureOwner.this;ApiEffects.word=word;}};}}
class ApiDriver {public static void main(String[]args)throws Exception{javax.net.ssl.SSLParameters p=new javax.net.ssl.SSLParameters();ApiCaptureOwner.configure(p);if(!p.getApplicationProtocols()[0].equals("proof"))throw new AssertionError("SDK API");for(long word:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){ApiCaptureOwner owner=new ApiCaptureOwner();Runnable r=owner.make(word);java.lang.reflect.Constructor<?>c=r.getClass().getDeclaredConstructors()[0];Class<?>[]physical=c.getParameterTypes();java.lang.reflect.Type[]generic=c.getGenericParameterTypes();if(physical.length!=2||generic.length!=physical.length||physical[0]!=ApiCaptureOwner.class||physical[1]!=long.class||generic[0]!=physical[0]||generic[1]!=physical[1])throw new AssertionError("constructor metadata");r.run();if(ApiEffects.owner!=owner||ApiEffects.word!=word)throw new AssertionError("capture identity and wide value");}System.out.println("3:sdk:constructor:identity:wide");}}`
			fixture = strings.ReplaceAll(fixture, "ApiCaptureOwner", owner)
			file := filepath.Join(original, owner+".java")
			if err := os.WriteFile(file, []byte(fixture), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(lookJavac(t), "-proc:none", "-source", "8", "-target", "8", "-g:none", "-d", original, file).CombinedOutput(); err != nil {
				t.Fatal("authored old bytecode/new SDK", err, string(out))
			}
			// Model the independently valid native compiler contract: an all-hidden
			// anonymous constructor has no declared Signature or MethodParameters.
			child, err := classparser.Parse(readClass(t, original, owner+"$1.class"))
			if err != nil {
				t.Fatal(err)
			}
			cp := classparser.NewConstantPoolWithConstant(&child.ConstantPool)
			for _, m := range child.Methods {
				if cp.GetUtf8(int(m.NameIndex)).Value != "<init>" {
					continue
				}
				kept := m.Attributes[:0]
				for _, a := range m.Attributes {
					switch a.(type) {
					case *classparser.SignatureAttribute:
						continue
					}
					if raw, ok := a.(*classparser.UnparsedAttribute); ok && raw.Name == "MethodParameters" {
						continue
					}
					kept = append(kept, a)
				}
				m.Attributes = kept
			}
			if err := os.WriteFile(filepath.Join(original, owner+"$1.class"), child.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			run := func(path string) {
				t.Helper()
				out, err := exec.Command(lookJava(t), "-Xverify:all", "-cp", path, "ApiDriver").CombinedOutput()
				if err != nil || string(out) != "3:sdk:constructor:identity:wide\n" {
					t.Fatal("original/rebuilt algorithm and reflection oracle", err, string(out))
				}
			}
			run(original)
			t.Log("independent original JVM oracle passed")
			owned := map[string][]byte{}
			for _, n := range []string{owner, owner + "$1"} {
				owned[n+".class"] = readClass(t, original, n+".class")
			}
			jar := filepath.Join(t.TempDir(), "original.jar")
			rebuiltFixtureJar(t, jar, owned)
			for _, n := range []string{"ApiEffects", "ApiDriver"} {
				if err := os.WriteFile(filepath.Join(support, n+".class"), readClass(t, original, n+".class"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			resolve := func(n string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(support, n+".class"))
				return b, e == nil
			}
			plan := historicalRenderAtCompilerTarget(t, jar, sources, 8, "reconstruction", resolve)
			defer plan.close()
			if plan.failures != 0 || plan.target != 9 {
				t.Fatal("selected source target", plan.failures, plan.target)
			}
			a, files := plan.reconstruction, plan.files
			errors, raw, passes := historicalTreeCompile(t, files, support, classes, plan.target)
			if errors != 0 || len(passes) != 1 || passes[0].Release != 9 || passes[0].ExitCode != 0 {
				t.Fatal("required SDK compilation", errors, passes, raw)
			}
			report, err := historicalFinishArtifactClasses("explicit-reconstruction-artifact-v1", a.FinishClass, classes, t.TempDir())
			if err != nil || len(report.Failures) != 0 || len(report.RestoredClasses) != 2 {
				t.Fatal("source target and compiler contract diverged", report, err)
			}
			run(classes + string(os.PathListSeparator) + support)
		})
	}
}
