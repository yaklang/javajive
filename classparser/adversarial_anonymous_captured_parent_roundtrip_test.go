package javaclassparser

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const anonymousCapturedParentFixture = `abstract class CapturedParentBox<T>{abstract T item();}
class CapturedParentEffects{static String trace="";static Object published;static boolean fail;static final RuntimeException error=new RuntimeException("identity");static String choose(CapturedParentBox<?> box){trace+="W";return "wide";}static String choose(Object box){throw new AssertionError("wrong binding");}}
abstract class CapturedParentReader{final Object early;CapturedParentReader(){CapturedParentEffects.trace+="P";CapturedParentEffects.published=this;early=read();if(CapturedParentEffects.fail)throw CapturedParentEffects.error;}abstract Object read();abstract Object box();}
class CapturedParentOwner{CapturedParentReader make(final CharSequence token){final CapturedParentBox<CharSequence> selected=new CapturedParentBox<CharSequence>(){CharSequence item(){return token;}};if(!CapturedParentEffects.choose(selected).equals("wide"))throw new AssertionError("wide binding");return new CapturedParentReader(){Object read(){return selected.item();}Object box(){return selected;}};}}
class CapturedParentDriver{public static void main(String[]args)throws Exception{int rows=0;for(CharSequence token:new CharSequence[]{null,new String("same"),new StringBuilder("different")})for(boolean fail:new boolean[]{false,true}){CapturedParentEffects.trace="";CapturedParentEffects.published=null;CapturedParentEffects.fail=fail;CapturedParentReader value=null;try{value=new CapturedParentOwner().make(token);if(fail)throw new AssertionError("lost failure");}catch(RuntimeException error){if(!fail||error!=CapturedParentEffects.error)throw new AssertionError("exception identity");value=(CapturedParentReader)CapturedParentEffects.published;}if(value!=CapturedParentEffects.published||value.early!=token||value.read()!=token||((CapturedParentBox<?>)value.box()).item()!=token||!CapturedParentEffects.trace.equals("WP"))throw new AssertionError("original capture/early callback/order");java.lang.reflect.Field field=value.getClass().getDeclaredField("val$selected");field.setAccessible(true);if(field.getType()!=CapturedParentBox.class||field.get(value)!=value.box()||value.getClass().getEnclosingClass()!=CapturedParentOwner.class||value.box().getClass().getEnclosingClass()!=CapturedParentOwner.class)throw new AssertionError("original type/binder/identity");rows++;}System.out.println(rows+":anonymous:parent:binding:early:identity:failure");}}`

func anonymousCapturedParentShape(shape string) (string, string) {
	fixture, owner := anonymousCapturedParentFixture, "CapturedParentOwner"
	switch shape {
	case "interface":
		fixture = strings.Replace(fixture, `abstract class CapturedParentBox<T>{abstract T item();}`, `interface CapturedParentBox<T>{T item();}`, 1)
		fixture = strings.Replace(fixture, `CharSequence item(){`, `public CharSequence item(){`, 1)
	case "bounded":
		fixture = strings.Replace(fixture, `class CapturedParentBox<T>`, `class CapturedParentBox<T extends CharSequence>`, 1)
	case "intersection":
		fixture = strings.ReplaceAll(fixture, `CharSequence`, `String`)
		fixture = strings.ReplaceAll(fixture, `new StringBuilder("different")`, `new String("different")`)
		fixture = strings.Replace(fixture, `class CapturedParentBox<T>`, `class CapturedParentBox<T extends CharSequence & java.io.Serializable>`, 1)
	case "array":
		fixture = strings.ReplaceAll(fixture, `CharSequence`, `CharSequence[]`)
		fixture = strings.Replace(fixture, `new CharSequence[][]{null,new String("same"),new StringBuilder("different")}`, `new CharSequence[][]{null,new CharSequence[]{new String("same")},new CharSequence[]{null,new StringBuilder("different")}}`, 1)
	case "two-parameters":
		fixture = strings.Replace(fixture, `class CapturedParentBox<T>{abstract T item();}`, `class CapturedParentBox<T,U>{abstract T item();abstract U unused();}`, 1)
		fixture = strings.ReplaceAll(fixture, `<CharSequence>`, `<CharSequence,Object>`)
		fixture = strings.ReplaceAll(fixture, `<?>`, `<?,?>`)
		fixture = strings.Replace(fixture, `CharSequence item(){return token;}`, `CharSequence item(){return token;}Object unused(){return null;}`, 1)
	case "renamed":
		owner = "AnonymousDeclaredNamespace"
		fixture = strings.ReplaceAll(fixture, `CapturedParentOwner`, owner)
		fixture = strings.ReplaceAll(fixture, `selected`, `originalDeclaredParent`)
	}
	return fixture, owner
}

// The original caller retains both anonymous instances and observes the second
// capture before super returns. Header/bridge signatures and overload selection
// constrain the source view independently of the first object's concrete class.
func TestAdversarialAnonymousCapturedParentDeclaration(t *testing.T) {
	for _, shape := range []string{"generic", "interface", "bounded", "intersection", "array", "two-parameters", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture, owner := anonymousCapturedParentShape(shape)
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "CapturedParentDriver", "6:anonymous:parent:binding:early:identity:failure\n", nativeLexicalExactSignatures)
		})
	}
}
func TestAdversarialAnonymousCapturedParentOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", err, string(out))
	}
	for _, shape := range []string{"generic", "interface", "bounded", "intersection", "array", "two-parameters", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture, owner := anonymousCapturedParentShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, "CapturedParentDriver", "6:anonymous:parent:binding:early:identity:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}
