package javaclassparser

import (
	"os"
	"path/filepath"
	"testing"
)

const nativeMemberConstructorHandleFixture = `class HandleRoot{private HandleRoot(){}static HandleRoot make(){return new HandleRoot();}static java.util.function.Supplier<HandleRoot> factory(){return HandleRoot::new;}static Object echo(Object value){return value;}static java.util.function.Function<Object,Object> function(){return HandleRoot::echo;}static class Member{final Object token;Member(Object token){this.token=token;}}}
class HandleDriver{public static void main(String[]args){Object token=new Object();HandleRoot root=HandleRoot.make();HandleRoot other=HandleRoot.factory().get();if(root==null||other==null||root==other||HandleRoot.function().apply(token)!=token||HandleRoot.function().apply(null)!=null||new HandleRoot.Member(token).token!=token)throw new AssertionError("handle binding/identity");System.out.println("handles:own-constructor:ordinary:identity");}}
`

func TestNativeMemberConstructorHandlesOwnAndOrdinaryRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberConstructorHandleFixture, []string{"HandleRoot"}, "HandleDriver", "handles:own-constructor:ordinary:identity\n")
}

// A constructor method handle is resolved by the lookup class, not by the
// reconstructed Java lexical nest. The original valid class can deliberately
// fail linkage; admitting its family would erase that observable failure.
func TestNativeMemberConstructorHandleForeignPrivateAccessRefused(t *testing.T) {
	fixture := `class HandleRoot{HandleRoot(){}static HandleRoot own(){return new HandleRoot();}static class Member{}}class HandleDriver{public static void main(String[]args){try{java.util.function.Supplier<HandleRoot> f=HandleRoot::new;f.get();throw new AssertionError("missing linkage failure");}catch(LinkageError expected){Throwable cause=expected;while(cause.getCause()!=null)cause=cause.getCause();if(!(cause instanceof IllegalAccessError))throw new AssertionError("wrong access error",expected);}if(HandleRoot.own()==null)throw new AssertionError("own access");System.out.println("foreign:constructor:handle:access");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			root, err := Parse(files["HandleRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if n == "<init>" {
					m.AccessFlags = (m.AccessFlags &^ uint16(7)) | 2
					count++
				}
			}
			if count != 1 {
				t.Fatal("constructor identity")
			}
			files["HandleRoot.class"] = root.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "HandleDriver"); got != "foreign:constructor:handle:access\n" {
				t.Fatalf("valid original oracle=%q", got)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			entry := z.nativeMemberEntry(root)
			if entry != nil && entry.family != nil {
				t.Fatal("foreign private constructor handle erased")
			}
		})
	}
}
