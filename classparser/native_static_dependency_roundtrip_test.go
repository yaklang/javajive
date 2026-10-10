package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeStaticDependencyFixture = `class DependencyEffects{static String trace="";static long arg(long n){trace+="A";return n;}}
class DependencyParent{final Object observed;DependencyParent(long n){DependencyEffects.trace+="P";observed=owner();if(n!=Long.MIN_VALUE)throw new AssertionError("argument");}Object owner(){return null;}}
class OtherScope{static class Value{final Object token;Value(Object token){this.token=token;}Object value(){return token;}}}
class DependencyOwner{class Child extends DependencyParent{Child(long n){super(n);}Object owner(){return DependencyOwner.this;}OtherScope.Value pass(OtherScope.Value value){return value;}}Child make(long n){return new Child(DependencyEffects.arg(n));}}
class DependencyDriver{public static void main(String[]args)throws Exception{DependencyOwner root=new DependencyOwner();Object token=new Object();OtherScope.Value value=new OtherScope.Value(token);DependencyEffects.trace="";DependencyOwner.Child child=root.make(Long.MIN_VALUE);if(child.observed!=root||child.owner()!=root||child.pass(value)!=value||child.pass(null)!=null||value.value()!=token||!DependencyEffects.trace.equals("AP"))throw new AssertionError("binding/identity/effects");if(child.getClass().getDeclaringClass()!=DependencyOwner.class||OtherScope.Value.class.getDeclaringClass()!=OtherScope.class)throw new AssertionError("source scope");System.out.println("static:dependency:identity:callback");}}
`

func TestNativeStaticCrossFamilyDependencyRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeStaticDependencyFixture, "DependencyOwner", "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticCrossFamilyDependencyRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(strings.ReplaceAll(nativeStaticDependencyFixture, "DependencyOwner", "ChangedScope"), "OtherScope", "ForeignNames")
	testNativePrivateSetterFixture(t, f, "ChangedScope", "DependencyDriver", "static:dependency:identity:callback\n")
}

// A valid original cross-family access can fail with IllegalAccessError. The
// type-name dependency cannot turn the two roots into one private nest.
func TestNativeStaticDependencyRefusesForeignPrivateConstructor(t *testing.T) {
	testNativeStaticDependencyPrivateConstructorFailure(t, false)
}

func TestNativeStaticDependencyRefusesLexicalDirectPrivateConstructor(t *testing.T) {
	testNativeStaticDependencyPrivateConstructorFailure(t, true)
}

func testNativeStaticDependencyPrivateConstructorFailure(t *testing.T, lexical bool) {
	t.Helper()
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `static class Value{`, `static class Value{static Value make(Object token){return new Value(token);}`)
	f = strings.ReplaceAll(f, `new OtherScope.Value(token)`, `OtherScope.Value.make(token)`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value){return value;}`, `OtherScope.Value pass(OtherScope.Value value){return value;}OtherScope.Value create(Object value){return new OtherScope.Value(value);}`)
	if lexical {
		f = strings.ReplaceAll(f, `class OtherScope{`, `class OtherScope{static Value create(Object token){return new Value(token);}`)
	}
	f = strings.ReplaceAll(f, `if(child.getClass().getDeclaringClass()`, `try{child.create(token);throw new AssertionError("missing original access failure");}catch(IllegalAccessError expected){if(!DependencyEffects.trace.equals("AP"))throw new AssertionError("access failure order");}if(child.getClass().getDeclaringClass()`)
	if lexical {
		f = strings.ReplaceAll(f, `child.create(token)`, `OtherScope.create(token)`)
	}
	files := nativeCompileClasses(t, f)
	obj, err := Parse(files["OtherScope$Value.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name == "<init>" {
			m.AccessFlags = (m.AccessFlags &^ uint16(7)) | 2
			changed++
		}
	}
	if changed != 1 {
		t.Fatal("original constructor")
	}
	files["OtherScope$Value.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for n, raw := range files {
		if err = os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "DependencyDriver"); got != "static:dependency:identity:callback\n" {
		t.Fatalf("verifier-valid original access failure=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	for _, owner := range []string{"DependencyOwner", "OtherScope"} {
		root, _ := Parse(files[owner+".class"])
		entry := z.nativeMemberEntry(root)
		if entry != nil && entry.family != nil {
			t.Fatalf("original private constructor access failure erased in %s", owner)
		}
	}
}

func TestNativeStaticDependencyPrivateConstructorOwnClassRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `Value(Object token){`, `private Value(Object token){`)
	f = strings.ReplaceAll(f, `static class Value{`, `static class Value{static Value make(Object token){return new Value(token);}`)
	f = strings.ReplaceAll(f, `new OtherScope.Value(token)`, `OtherScope.Value.make(token)`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}

func TestNativeStaticDependencyPrivateRootConstructorCallersClosed(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `class DependencyOwner{`, `class DependencyOwner{DependencyOwner(){}static DependencyOwner own(){return new DependencyOwner();}`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value)`, `Object pass(Object value)`)
	f = strings.ReplaceAll(f, `DependencyOwner root=new DependencyOwner();`, `try{new DependencyOwner();throw new AssertionError("root access failure");}catch(IllegalAccessError expected){}DependencyOwner root=DependencyOwner.own();`)
	files := nativeCompileClasses(t, f)
	root, err := Parse(files["DependencyOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, m := range root.Methods {
		name, _ := sourceBridgeUTF8(root, m.NameIndex)
		if name == "<init>" {
			m.AccessFlags = (m.AccessFlags &^ uint16(7)) | 2
			changed++
		}
	}
	if changed != 1 {
		t.Fatal("root constructor")
	}
	files["DependencyOwner.class"] = root.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "DependencyDriver"); got != "static:dependency:identity:callback\n" {
		t.Fatalf("valid original root access failure=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	entry := z.nativeMemberEntry(root)
	if entry != nil && entry.family != nil {
		t.Fatal("unindexed foreign private root call accepted")
	}
}
