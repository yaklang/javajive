package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const iterationCaptureFixture = `class IterationCaptureEffects{static String trace="";static Object published;static int fail;static final RuntimeException error=new RuntimeException("identity");static Object choose(Object value){trace+="E";if(fail==1)throw error;return value;}}
abstract class IterationCaptureBase{final Object early;IterationCaptureBase(){IterationCaptureEffects.trace+="P";IterationCaptureEffects.published=this;early=read();if(IterationCaptureEffects.fail==2)throw IterationCaptureEffects.error;}abstract Object read();}
class IterationCaptureOwner{IterationCaptureBase[] make(Object[] values){IterationCaptureBase[] result=new IterationCaptureBase[values.length];for(int i=0;i<values.length;i++){final Object selected;if(values[i]!=null){selected=IterationCaptureEffects.choose(values[i]);}else{selected=null;}result[i]=new IterationCaptureBase(){Object read(){return selected;}};}return result;}}
class IterationCaptureDriver{public static void main(String[]args){int rows=0;Object identity=new Object();for(Object[] input:new Object[][]{ {},{null},{identity},{null,identity,null,new Object(),identity}})for(int fail=0;fail<3;fail++){IterationCaptureEffects.trace="";IterationCaptureEffects.published=null;IterationCaptureEffects.fail=fail;String expected="";int index=0;boolean failed=false;for(;index<input.length;index++){if(input[index]!=null){expected+="E";if(fail==1){failed=true;break;}}expected+="P";if(fail==2){failed=true;break;}}try{IterationCaptureBase[] result=new IterationCaptureOwner().make(input);if(failed||result.length!=input.length)throw new AssertionError("completion");for(int i=0;i<input.length;i++){if(result[i].read()!=input[i]||result[i].early!=input[i])throw new AssertionError("per iteration/early capture");for(int j=0;j<i;j++)if(result[i]==result[j])throw new AssertionError("allocation identity");}}catch(RuntimeException error){if(!failed||error!=IterationCaptureEffects.error)throw new AssertionError("failure identity");if(fail==2&&((IterationCaptureBase)IterationCaptureEffects.published).read()!=input[index])throw new AssertionError("failed constructor capture");}if(!IterationCaptureEffects.trace.equals(expected))throw new AssertionError("evaluation order:"+IterationCaptureEffects.trace+":"+expected);rows++;}System.out.println(rows+":iteration:join:identity:early:failure");}}`

// Each callback is invoked by the original superclass before the constructor
// completes. Retaining all earlier allocations detects a reused per-iteration
// capture cell, not just a wrong value in the last anonymous instance.
func iterationCaptureShape(shape string) (string, string) {
	fixture, owner := iterationCaptureFixture, "IterationCaptureOwner"
	start := `for(int i=0;i<values.length;i++){`
	switch shape {
	case "while":
		fixture = strings.Replace(fixture, start, `int i=0;while(i<values.length){`, 1)
		fixture = strings.Replace(fixture, `return selected;}};}return result;`, `return selected;}};i++;}return result;`, 1)
	case "do":
		fixture = strings.Replace(fixture, start, `int i=0;if(values.length!=0)do{`, 1)
		fixture = strings.Replace(fixture, `return selected;}};}return result;`, `return selected;}};i++;}while(i<values.length);return result;`, 1)
	case "nested":
		fixture = strings.Replace(fixture, start, `for(int outer=0;outer<1;outer++){`+start, 1)
		fixture = strings.Replace(fixture, `return selected;}};}return result;`, `return selected;}};}}return result;`, 1)
	case "renamed":
		owner = "FreshLocalNamespace"
		fixture = strings.ReplaceAll(fixture, "IterationCaptureOwner", owner)
		fixture = strings.ReplaceAll(fixture, "selected", "capturedPayload")
	}
	return fixture, owner
}
func TestAdversarialIterationCaptureJoinFreshLocal(t *testing.T) {
	for _, shape := range []string{"for", "while", "do", "nested", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture, owner := iterationCaptureShape(shape)
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "IterationCaptureDriver", "12:iteration:join:identity:early:failure\n", nativeLexicalExactSignatures)
		})
	}
}
func TestAdversarialIterationCaptureJoinOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", err, string(out))
	}
	for _, shape := range []string{"for", "while", "do", "nested", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture, owner := iterationCaptureShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				path := filepath.Join(dir, owner+".java")
				if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatal("original compile", err, string(out))
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}, NativeJavac8, javac, []string{owner}, "IterationCaptureDriver", "12:iteration:join:identity:early:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}
