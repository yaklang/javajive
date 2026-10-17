package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A captured producer inside a loop or branch belongs to that lexical block.
// The unchanged driver checks surviving snapshots after the iterator advances,
// empty/null inputs, the callback before superclass completion and evaluation count.
func TestAdversarialMethodLocalBlockProducerSnapshots(t *testing.T) {
	for _, shape := range []string{"loop parameter", "branch parameter", "loop field"} {
		t.Run(shape, func(t *testing.T) {
			source := nativeBlockProducerFixture(shape)
			testNativeIndependentFamilyFixture(t, source, []string{"BlockScopeOwner"}, "BlockScopeDriver", "7:block:snapshot:callback:identity:consumption\n", nativeLexicalExactSignatures)
		})
	}
}

func nativeBlockProducerFixture(shape string) string {
	source := `abstract class BlockSnapshot{static Object observed;BlockSnapshot(){observed=get();}abstract Object get();}
class BlockScopeOwner{BlockSnapshot make(java.util.Iterator<?> iterator){while(iterator.hasNext()){final Object made=iterator.next();if(made!=null){class Entry extends BlockSnapshot{Object get(){return made;}}return new Entry();}}return null;}}
class BlockScopeDriver{public static void main(String[]args){Object first=new Object(),second=new Object();Object[][] inputs={ {},{null},{first},{null,first},{first,second},{null,null,second},{first,first}};int rows=0;for(Object[] input:inputs){BlockSnapshot.observed=null;java.util.Iterator<Object> iterator=java.util.Arrays.asList(input).iterator();BlockSnapshot actual=new BlockScopeOwner().make(iterator);Object expected=null;int consumed=0;for(Object token:input){consumed++;if(token!=null){expected=token;break;}}if((actual==null)!=(expected==null))throw new AssertionError("snapshot existence");if(actual==null?expected!=null:actual.get()!=expected||BlockSnapshot.observed!=expected)throw new AssertionError("block capture/callback identity");int remaining=0;while(iterator.hasNext()){iterator.next();remaining++;}if(remaining!=input.length-consumed)throw new AssertionError("producer moved or repeated");if(actual!=null&&actual.get()!=expected)throw new AssertionError("snapshot changed after iterator advancement");rows++;}System.out.println(rows+":block:snapshot:callback:identity:consumption");}}`
	switch shape {
	case "branch parameter":
		source = strings.Replace(source, "while(iterator.hasNext())", "if(iterator.hasNext())", 1)
		source = strings.Replace(source, "Object expected=null;int consumed=0;for(Object token:input){consumed++;if(token!=null){expected=token;break;}}", "Object expected=input.length==0?null:input[0];int consumed=input.length==0?0:1;", 1)
	case "loop field":
		source = strings.Replace(source, "class BlockScopeOwner{BlockSnapshot make(java.util.Iterator<?> iterator){", "class BlockScopeOwner{java.util.Iterator<?> iterator;BlockSnapshot make(java.util.Iterator<?> input){this.iterator=input;", 1)
	}
	return source
}

func TestAdversarialMethodLocalBlockProducerOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"loop parameter", "branch parameter", "loop field"} {
		t.Run(shape, func(t *testing.T) {
			source := nativeBlockProducerFixture(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, source, "BlockScopeOwner", debug)
			}, NativeJavac8, javac, []string{"BlockScopeOwner"}, "BlockScopeDriver", "7:block:snapshot:callback:identity:consumption\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialMethodLocalBlockProducerOracleRejectsSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	source := nativeBlockProducerFixture("loop parameter")
	original := nativeCompileDebugClasses(t, source, "none")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"lost snapshot identity", "return made;", "return new Object();"},
		{"lost superclass callback", "observed=get();", "observed=null;"},
		{"duplicated producer", "final Object made=iterator.next();", "final Object made=iterator.next();if(iterator.hasNext())iterator.next();"},
		{"wrong null branch", "if(made!=null)", "if(true)"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.before != "" {
				if !strings.Contains(source, mutation.before) {
					t.Fatal("missing mutation target")
				}
				files = nativeCompileDebugClasses(t, strings.Replace(source, mutation.before, mutation.after, 1), "none")
			}
			out := t.TempDir()
			for name, raw := range files {
				path := filepath.Join(out, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, mode := range []string{"-ea", "-da"} {
				output, err := exec.Command(java, mode, "-Xverify:all", "-cp", out, "BlockScopeDriver").CombinedOutput()
				if mutation.before == "" {
					if err != nil || string(output) != "7:block:snapshot:callback:identity:consumption\n" {
						t.Fatalf("original %v %s", err, output)
					}
				} else if err == nil || !strings.Contains(string(output), "AssertionError") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "LinkageError") {
					t.Fatalf("semantic mutant not rejected by independent oracle: %v %s", err, output)
				}
			}
		})
	}
}
