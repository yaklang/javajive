package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const constructorLoweringNativeFixture = `class NativeLoweringWitness {static Object token;static long wide;static NativeLoweringBase escaped;static RuntimeException failure;}
abstract class NativeLoweringBase {NativeLoweringBase(Object input){NativeLoweringWitness.escaped=this;if(capture()!=NativeLoweringWitness.token||number()!=NativeLoweringWitness.wide)throw new AssertionError("native early capture");if(input==null)throw NativeLoweringWitness.failure;}abstract Object capture();abstract long number();}
public class NativeLoweringOwner {public NativeLoweringBase make(Object input,final Object token,final long wide){return new NativeLoweringBase(input){Object capture(){return token;}long number(){return wide;}};}}
class NativeLoweringDriver {public static void main(String[]args){int rows=0;NativeLoweringOwner owner=new NativeLoweringOwner();for(Object token:new Object[]{new Object(),null})for(long wide:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int fail=0;fail<2;fail++){NativeLoweringWitness.token=token;NativeLoweringWitness.wide=wide;NativeLoweringWitness.failure=new IllegalArgumentException("shared");NativeLoweringWitness.escaped=null;Throwable error=null;try{owner.make(fail==0?new Object():null,token,wide);}catch(Throwable e){error=e;}if(error!=(fail==0?null:NativeLoweringWitness.failure)||NativeLoweringWitness.escaped==null||NativeLoweringWitness.escaped.capture()!=token||NativeLoweringWitness.escaped.number()!=wide)throw new AssertionError("native lifecycle",error);rows++;}System.out.println(rows+":native:closed:unchanged-caller");}}`

func TestAdversarialConstructorLoweringKeepsNativeSourceOwnershipTransactions(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, constructorLoweringNativeFixture, "NativeLoweringOwner", debug, false)
			run := func(directory string) {
				t.Helper()
				for _, enabled := range []string{"-ea", "-da"} {
					out, err := exec.Command(java, enabled, "-Xverify:all", "-cp", directory, "NativeLoweringDriver").CombinedOutput()
					if err != nil || string(out) != "16:native:closed:unchanged-caller\n" {
						t.Fatalf("native callback/abrupt oracle: %v\n%s", err, out)
					}
				}
			}
			run(sourceArtifactWriteFiles(t, original))
			owned := map[string][]byte{"NativeLoweringOwner.class": original["NativeLoweringOwner.class"], "NativeLoweringOwner$1.class": original["NativeLoweringOwner$1.class"]}
			jar := filepath.Join(t.TempDir(), "original.jar")
			if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
				t.Fatal(err)
			}
			resolve := func(binary string) ([]byte, bool) { raw, ok := original[binary+".class"]; return raw, ok }
			plain, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, resolve)
			if err != nil {
				t.Fatal(err)
			}
			defer plain.Close()
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, resolve)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			var root []byte
			// Child first exercises native source-family publication without
			// letting a provisional flat reader enroll a lowering obligation.
			for _, name := range []string{"NativeLoweringOwner$1.class", "NativeLoweringOwner.class"} {
				source, err := a.ReadFile(name)
				control, controlErr := plain.ReadFile(name)
				if err != nil || controlErr != nil || !bytes.Equal(source, control) || legacySourceHasFailureMarker(string(source)) {
					t.Fatalf("native source selection changed %s: %v/%v\n%s", name, err, controlErr, source)
				}
				if name == "NativeLoweringOwner.class" {
					root = source
				}
			}
			if !strings.Contains(string(root), "new NativeLoweringBase") {
				t.Fatalf("authored native closure was not selected: %s", root)
			}
			if len(a.source.jar.sourceConstructorLowering.active) != 0 {
				t.Fatal("a native member reader enrolled a flat lowering")
			}
			dependencies := map[string][]byte{}
			for name, raw := range original {
				if owned[name] == nil {
					dependencies[name] = raw
				}
			}
			out := sourceArtifactWriteFiles(t, dependencies)
			path := filepath.Join(out, "NativeLoweringOwner.java")
			if err := os.WriteFile(path, root, 0600); err != nil {
				t.Fatal(err)
			}
			if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
				t.Fatal("owned original family absent from compilation", err, string(log), string(root))
			}
			for name := range owned {
				fresh, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatal("compiler omitted an owned class", name, err)
				}
				got, err := a.FinishClass(name, fresh)
				control, controlErr := plain.RestoreClassMetadata(name, fresh)
				if err != nil || controlErr != nil || !bytes.Equal(got, control) {
					t.Fatal("native completion changed existing source contract", name, err, controlErr)
				}
				if err := os.WriteFile(filepath.Join(out, name), got, 0600); err != nil {
					t.Fatal(err)
				}
			}
			run(out)
		})
	}
}
