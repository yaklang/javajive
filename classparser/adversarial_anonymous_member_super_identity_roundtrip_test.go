package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A named class declared in an anonymous subclass inherits an inner class of
// that subclass's superclass. Its superclass enclosing object is the anonymous
// instance itself, not the factory receiver or an unrelated outer instance.
const anonymousMemberSuperIdentityFixture = `abstract class OriginalNamespace{
 final Object seed;OriginalNamespace(Object seed){this.seed=seed;}
 class Entry{final Object value;protected Entry(Object value){this.value=value;}Object outer(){return OriginalNamespace.this;}}
 abstract Entry read(Object token);
 static OriginalNamespace create(Object seed){return new OriginalNamespace(seed){
  class Derived extends Entry{Derived(Object token){super(token);}}
  Entry read(Object token){return new Derived(token);}
 };}
}
class AnonymousMemberSuperDriver{public static void main(String[]args){Object marker=new Object();int rows=0;boolean declaration=true;
 for(Object seed:new Object[]{null,marker,"seed"}){OriginalNamespace namespace=OriginalNamespace.create(seed);declaration&=namespace.getClass().isAnonymousClass()&&namespace.getClass().getEnclosingMethod()!=null&&namespace.getClass().getEnclosingMethod().getName().equals("create");
  for(Object token:new Object[]{null,marker,"token"}){OriginalNamespace.Entry entry=namespace.read(token);
   if(entry.value!=token||entry.outer()!=namespace||namespace.seed!=seed)throw new AssertionError("anonymous member SUPER enclosing identity");
   declaration&=entry.getClass().getSuperclass()==OriginalNamespace.Entry.class&&entry.getClass().getDeclaringClass()==namespace.getClass()&&entry.getClass().isMemberClass();rows++;
  }
 }
 System.out.println(rows+":anonymous:member:super:identity");if(!declaration)throw new AssertionError("anonymous/member declaration identity");}}
`

func TestAdversarialAnonymousSubclassMemberSuperUsesOriginalEnclosingInstance(t *testing.T) {
	testNativeIndependentFamilyFixture(t, anonymousMemberSuperIdentityFixture, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperKeepsGenericLexicalScope(t *testing.T) {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "class OriginalNamespace{", "class OriginalNamespace<T>{", 1)
	f = strings.Replace(f, "final Object seed;OriginalNamespace(Object seed)", "final T seed;OriginalNamespace(T seed)", 1)
	f = strings.Replace(f, "class Entry{final Object value;protected Entry(Object value)", "class Entry<U>{final U value;protected Entry(U value)", 1)
	f = strings.Replace(f, "abstract Entry read(Object token);", "abstract <U> Entry<U> read(U token);", 1)
	f = strings.Replace(f, "static OriginalNamespace create(Object seed){return new OriginalNamespace(seed)", "static <V> OriginalNamespace<V> create(V seed){return new OriginalNamespace<V>(seed)", 1)
	f = strings.Replace(f, "class Derived extends Entry{Derived(Object token){super(token);}}", "class Derived<U> extends Entry<U>{Derived(U token){super(token);}}", 1)
	f = strings.Replace(f, "Entry read(Object token){return new Derived(token);}", "<U> Entry<U> read(U token){return new Derived<U>(token);}", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperPreservesParentObservation(t *testing.T) {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "this.value=value;", "this.value=value;if(outer()!=OriginalNamespace.this)throw new AssertionError(\"early enclosing capture\");", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperKeepsInstanceLexicalDepth(t *testing.T) {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "static OriginalNamespace create(Object seed)", "OriginalNamespace create(Object seed)", 1)
	f = strings.Replace(f, "OriginalNamespace namespace=OriginalNamespace.create(seed);", "OriginalNamespace host=new OriginalNamespace(marker){Entry read(Object token){return new Entry(token);}};OriginalNamespace namespace=host.create(seed);", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperTraversesNamedDescendant(t *testing.T) {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "class Derived extends Entry{Derived(Object token){super(token);}}", "class Derived{class Leaf extends Entry{Leaf(Object token){super(token);}}Entry make(Object token){return new Leaf(token);}}", 1)
	f = strings.Replace(f, "return new Derived(token);", "return new Derived().make(token);", 1)
	f = strings.Replace(f, "entry.getClass().getDeclaringClass()==namespace.getClass()", "entry.getClass().getDeclaringClass().getDeclaringClass()==namespace.getClass()", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperTraversesMultipleNamedCaptures(t *testing.T) {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "class Derived extends Entry{Derived(Object token){super(token);}}", "class Derived{class Branch{class Leaf extends Entry{Leaf(Object token){super(token);}}Entry leaf(Object token){return new Leaf(token);}}Entry make(Object token){return new Branch().leaf(token);}}", 1)
	f = strings.Replace(f, "return new Derived(token);", "return new Derived().make(token);", 1)
	f = strings.Replace(f, "entry.getClass().getDeclaringClass()==namespace.getClass()", "entry.getClass().getDeclaringClass().getDeclaringClass().getDeclaringClass()==namespace.getClass()", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperRenamedDeclarations(t *testing.T) {
	f := strings.NewReplacer("OriginalNamespace", "EnclosingFactory", "Derived", "RenamedBranch", "Entry", "Item", "seed", "context", "token", "argument", "create", "build", "read", "produce").Replace(anonymousMemberSuperIdentityFixture)
	testNativeIndependentFamilyFixture(t, f, []string{"EnclosingFactory"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func anonymousNamedWithoutRootAnchorFixture() string {
	f := strings.Replace(anonymousMemberSuperIdentityFixture, "class Entry{final Object value;protected Entry(Object value){this.value=value;}Object outer(){return OriginalNamespace.this;}}", "", 1)
	f = strings.ReplaceAll(f, "Entry read(", "Object read(")
	f = strings.Replace(f, "class Derived extends Entry{Derived(Object token){super(token);}}", "class Derived{final Object value;Derived(Object token){this.value=token;}}", 1)
	f = strings.Replace(f, "OriginalNamespace.Entry entry=namespace.read(token);", "Object entry=namespace.read(token);java.lang.reflect.Field value=entry.getClass().getDeclaredField(\"value\");value.setAccessible(true);java.lang.reflect.Field outer=null;for(java.lang.reflect.Field field:entry.getClass().getDeclaredFields())if(field.isSynthetic()&&field.getType()==namespace.getClass()){if(outer!=null)throw new AssertionError(\"ambiguous capture\");outer=field;outer.setAccessible(true);}", 1)
	f = strings.Replace(f, "entry.value!=token||entry.outer()!=namespace", "value.get(entry)!=token||outer==null||outer.get(entry)!=namespace", 1)
	f = strings.Replace(f, "==OriginalNamespace.Entry.class", "==Object.class", 1)
	f = strings.Replace(f, "main(String[]args){", "main(String[]args)throws Exception{", 1)
	return f
}

func TestAdversarialAnonymousNamedMemberWithoutNamedRootAnchor(t *testing.T) {
	testNativeIndependentFamilyFixture(t, anonymousNamedWithoutRootAnchorFixture(), []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousNamedSiblingInheritanceWithoutNamedRootAnchor(t *testing.T) {
	f := strings.Replace(anonymousNamedWithoutRootAnchorFixture(), "class Derived{final Object value;Derived(Object token){this.value=token;}}", "class Base{final Object value;Base(Object value){this.value=value;}}class Derived extends Base{Derived(Object token){super(token);}}", 1)
	f = strings.Replace(f, "entry.getClass().getDeclaredField(\"value\")", "entry.getClass().getSuperclass().getDeclaredField(\"value\")", 1)
	f = strings.Replace(f, "entry.getClass().getSuperclass()==Object.class", "entry.getClass().getSuperclass().getDeclaringClass()==namespace.getClass()", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousSubclassMemberSuperNativeCompilerProtocol(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		root := t.TempDir()
		path := filepath.Join(root, "OriginalNamespace.java")
		if err := os.WriteFile(path, []byte(anonymousMemberSuperIdentityFixture), 0600); err != nil {
			t.Fatal(err)
		}
		if raw, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
			t.Fatal("authored original compile", err, string(raw))
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".class") {
				raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = raw
			}
		}
		return files
	}, NativeJavac8, javac, []string{"OriginalNamespace"}, "AnonymousMemberSuperDriver", "9:anonymous:member:super:identity\n", nil, nativeLexicalExactSignatures)
}
