package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMethodLocalLexicalCaptureFixture = `interface LocalCaptureView {Object root();Object inner();Object argument();long eval(int width);}
class LocalCaptureOwner {final Object token;final long word;static String trace="";static boolean fail;static final RuntimeException error=new RuntimeException("identity");LocalCaptureOwner(Object token,long word){this.token=token;this.word=word;}static abstract class Parent {final Object observed;Parent(){trace+="P";observed=observe();if(fail)throw error;}abstract Object observe();}class Container extends Parent {final Object local;Container(Object local){this.local=local;}Object observe(){return LocalCaptureOwner.this.token;}LocalCaptureView make(Object argument,long seed){class Entry implements LocalCaptureView {public Object root(){return LocalCaptureOwner.this.token;}public Object inner(){return Container.this.local;}public Object argument(){return argument;}public long eval(int width){return (LocalCaptureOwner.this.word^seed)&((1L<<(width&63))-1L);}}return new Entry();}}}
class LocalCaptureDriver {public static void main(String[]args)throws Exception {Object token=new Object();int rows=0;for(Object outer:new Object[]{null,token})for(Object inner:new Object[]{null,token})for(Object argument:new Object[]{null,token})for(long word:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){LocalCaptureOwner owner=new LocalCaptureOwner(outer,word);LocalCaptureOwner.trace="";LocalCaptureOwner.fail=false;LocalCaptureOwner.Container c=owner.new Container(inner);if(c.observed!=outer||!LocalCaptureOwner.trace.equals("P"))throw new AssertionError("capture callback");LocalCaptureView v=c.make(argument,seed);if(v.root()!=outer||v.inner()!=inner||v.argument()!=argument||c.getClass().getDeclaringClass()!=LocalCaptureOwner.class||v.getClass().getEnclosingMethod().getDeclaringClass()!=c.getClass()||!v.getClass().getEnclosingMethod().getName().equals("make")||v.getClass().getDeclaringClass()!=null)throw new AssertionError("lexical original identities");for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,31,32,63,64,65,Integer.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(word).xor(java.math.BigInteger.valueOf(seed)).and(java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE)).longValue();if(v.eval(width)!=expected)throw new AssertionError("word oracle");rows++;}LocalCaptureOwner.trace="";LocalCaptureOwner.fail=true;try{owner.new Container(inner);throw new AssertionError("missing abrupt callback");}catch(RuntimeException e){if(e!=LocalCaptureOwner.error||!LocalCaptureOwner.trace.equals("P"))throw new AssertionError("failure identity");}}System.out.println(rows+":local-class:lexical-capture:identity:word:callback:failure");}}
`

func TestNativeMethodLocalNamedLexicalCaptureRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		for _, deep := range []bool{false, true} {
			name := "ordinary"
			fixture := nativeMethodLocalLexicalCaptureFixture
			owner := "LocalCaptureOwner"
			driver := "LocalCaptureDriver"
			if deep {
				name += "/deep"
				fixture = strings.ReplaceAll(fixture, "LocalCaptureView make(Object argument,long seed){", "class Layer {LocalCaptureView make(Object argument,long seed){")
				fixture = strings.ReplaceAll(fixture, "return new Entry();}}}", "return new Entry();}}}}")
				fixture = strings.ReplaceAll(fixture, "c.make(argument,seed)", "c.new Layer().make(argument,seed)")
				fixture = strings.ReplaceAll(fixture, "getEnclosingMethod().getDeclaringClass()!=c.getClass()", "getEnclosingMethod().getDeclaringClass()!=LocalCaptureOwner.Container.Layer.class")
			}
			if renamed {
				name += "/renamed"
				fixture = strings.NewReplacer("LocalCaptureOwner", "RenamedEnvelope", "LocalCaptureDriver", "RenamedDriver", "Container", "Chamber", "Entry", "Packet", "Layer", "Floor", "argument", "request", "seed", "salt").Replace(fixture)
				owner = "RenamedEnvelope"
				driver = "RenamedDriver"
			}
			t.Run(name, func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "1600:local-class:lexical-capture:identity:word:callback:failure\n", nativeLexicalExactSignatures)
			})
		}
	}
}

func TestNativeMethodLocalLexicalGenericAndPrivateScopeRoundTrip(t *testing.T) {
	for _, profile := range []string{"generic", "private type", "modern11"} {
		t.Run(profile, func(t *testing.T) {
			fixture := nativeMethodLocalLexicalCaptureFixture
			release := "8"
			switch profile {
			case "generic":
				fixture = strings.ReplaceAll(fixture, "class LocalCaptureOwner {", "class LocalCaptureOwner<T> {")
			case "private type":
				fixture = strings.ReplaceAll(fixture, "interface LocalCaptureView {", "interface LocalCaptureView {Class<?> kind();")
				fixture = strings.ReplaceAll(fixture, "class LocalCaptureOwner {", "class LocalCaptureOwner {private static class Key {} static Class<?> keyType(){return Key.class;}")
				fixture = strings.ReplaceAll(fixture, "public Object root(){", "public Class<?> kind(){return Key.class;}public Object root(){")
				fixture = strings.ReplaceAll(fixture, "if(v.root()!=outer", "if(v.kind()!=LocalCaptureOwner.keyType()||v.root()!=outer")
			case "modern11":
				release = "11"
			}
			testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte { return nativeCompileReleaseClasses(t, fixture, debug, release) }, []string{"LocalCaptureOwner"}, "LocalCaptureDriver", "1600:local-class:lexical-capture:identity:word:callback:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}

// A lexical capture proof does not grant an unproved input-version namespace.
// Preserve the existing post-55 refusal separately from the positive source
// profiles; the original authored program is still independently executable.
func TestNativeMethodLocalLexicalCaptureKeepsUnprovedModernVersionRefusal(t *testing.T) {
	files := nativeCompileReleaseClasses(t, nativeMethodLocalLexicalCaptureFixture, "none", "16")
	_, java := t04Tools(t)
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "LocalCaptureDriver"); got != "1600:local-class:lexical-capture:identity:word:callback:failure\n" {
		t.Fatalf("original oracle=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, e := Parse(files["LocalCaptureOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	if z.prepareNativeMemberFamily(root, snapshotJDECEnv()) != nil {
		t.Fatal("local capture proof licensed unproved input namespace")
	}
}
