package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The unchanged original caller observes the constructor's early virtual
// callback, delayed enclosing reads, generic identity and exact anonymous ABI.
const hierarchyQualifiedAnonymousFixture = `abstract class NumberBase<T extends Number> {
 final int slots; final String label; final T missing; final boolean reverse; final int flags; final int observed;
 NumberBase(int slots,String label,T missing,boolean reverse,int flags){this.slots=slots;this.label=label;this.missing=missing;this.reverse=reverse;this.flags=flags;this.observed=probe();}
 abstract int probe(); abstract NumberLeaf leaf(Object context);
 abstract class NumberLeaf {final Object context;NumberLeaf(Object context){this.context=context;}T missing(){return NumberBase.this.missing;}abstract int read();Object context(){return context;}}
}
abstract class IntNumber extends NumberBase<Integer> {IntNumber(int slots,String label,Integer missing,boolean reverse,int flags){super(slots,label,missing,reverse,flags);}}
class ConstructOwner {private int stamp;ConstructOwner(int stamp){this.stamp=stamp;} void set(int value){stamp=value;}
 NumberBase<Integer> make(int slots,String label,Integer missing,boolean reverse,int flags){return new IntNumber(slots,label,missing,reverse,flags){
  int probe(){return stamp;}
  NumberLeaf leaf(Object context){return new NumberLeaf(context){int read(){return stamp+slots;}};}
 };}
}
class ConstructDriver {public static void main(String[]args){Object shared=new Object();int rows=0;
 for(int stamp:new int[]{-1,0,2147483647})for(Integer missing:new Integer[]{null,-1,7})for(Object context:new Object[]{null,shared,new Object()}){
 ConstructOwner owner=new ConstructOwner(stamp);NumberBase<Integer> number=owner.make(3,"L",missing,true,9);NumberBase<Integer>.NumberLeaf first=number.leaf(context),second=number.leaf(context);
 if(number.observed!=stamp||first.context()!=context||first.missing()!=missing||first==second)throw new AssertionError("constructor original ownership/early callback");
 if(!number.getClass().isAnonymousClass()||!first.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=number.getClass())throw new AssertionError("constructor original scope");
 owner.set(stamp^17);if(first.read()!=(stamp^17)+3||second.read()!=(stamp^17)+3)throw new AssertionError("constructor delayed enclosing receiver");rows++;
 }System.out.println(rows+":constructor");}}
 `

func hierarchyQualifiedAnonymousVariant(shape, rename string) (string, string) {
	source := strings.NewReplacer(
		"for(Object context:new Object[]{null,shared,new Object()}){", "for(Object context:new Object[]{null,shared,new Object()})for(int size:new int[]{Integer.MIN_VALUE,-1,Integer.MAX_VALUE}){",
		"owner.make(3,", "owner.make(size,", "(stamp^17)+3", "(stamp^17)+size",
	).Replace(hierarchyQualifiedAnonymousFixture)
	switch shape {
	case "wide":
		source = strings.NewReplacer("final int slots;", "final long slots;", "int slots", "long slots", "for(int size:new int[]{Integer.MIN_VALUE,-1,Integer.MAX_VALUE})", "for(long size:new long[]{Long.MIN_VALUE,-1,Long.MAX_VALUE})", "abstract int read();", "abstract long read();", "int read(){return stamp+slots;}", "long read(){return stamp+slots;}").Replace(source)
	case "volatile":
		source = strings.NewReplacer("final int slots;", "volatile int slots;", "owner.set(stamp^17);", "owner.set(stamp^17);number.slots=size^17;", "(stamp^17)+size", "(stamp^17)+(size^17)").Replace(source)
	case "local-shadow":
		source = strings.Replace(source, "int read(){return stamp+slots;}", "int read(){int observed=stamp+slots;int slots=31;if(slots==observed)throw new AssertionError(\"local identity\");return observed;}", 1)
	}
	owner := "ConstructOwner"
	if rename == "renamed" {
		source = strings.NewReplacer("NumberBase", "Ancestor", "IntNumber", "ConcreteAncestor", "NumberLeaf", "ReaderLeaf", "ConstructOwner", "EnclosingHost", "stamp", "epoch", "slots", "width").Replace(source)
		owner = "EnclosingHost"
	}
	return source, owner
}

func TestAdversarialAnonymousInheritedLexicalFieldKeepsBinding(t *testing.T) {
	for _, shape := range []string{"int", "wide", "volatile", "local-shadow"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					source, owner := hierarchyQualifiedAnonymousVariant(shape, rename)
					testNativePrivateSetterCompiledFixture(t, owner, "ConstructDriver", "81:constructor\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

func TestAdversarialAnonymousInheritedLexicalFieldNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	for _, shape := range []string{"int", "wide", "volatile", "local-shadow"} {
		t.Run(shape, func(t *testing.T) {
			source, owner := hierarchyQualifiedAnonymousVariant(shape, "renamed")
			compile := func(debug string) map[string][]byte {
				root := t.TempDir()
				path := filepath.Join(root, owner+".java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if data, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
					t.Fatal("authored compile", err, string(data))
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
			}
			testNativeIndependentCompilerFamilyFixture(t, compile, NativeJavac8, javac, []string{owner}, "ConstructDriver", "81:constructor\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func hierarchyQualifiedProtectedSources() map[string]string {
	split := strings.Index(hierarchyQualifiedAnonymousFixture, "abstract class IntNumber")
	base := strings.NewReplacer(
		"abstract class NumberBase", "public abstract class NumberBase",
		"final int slots;", "protected final int slots;",
		"final int observed;", "public final int observed;",
		" NumberBase(", " protected NumberBase(",
		"abstract int probe();", "protected abstract int probe();",
		"abstract NumberLeaf leaf(", "public abstract NumberLeaf leaf(",
		"abstract class NumberLeaf", "public abstract class NumberLeaf",
		"NumberLeaf(Object context)", "protected NumberLeaf(Object context)",
		"T missing(){", "public T missing(){",
		"abstract int read();", "public abstract int read();",
		"Object context(){", "public Object context(){",
	).Replace(hierarchyQualifiedAnonymousFixture[:split])
	owner := strings.NewReplacer("int probe(){", "protected int probe(){", "NumberLeaf leaf(Object context)", "public NumberLeaf leaf(Object context)", "int read(){", "public int read(){").Replace(hierarchyQualifiedAnonymousFixture[split:])
	return map[string]string{"ancestry/NumberBase.java": "package ancestry;\n" + base, "client/ConstructOwner.java": "package client;\nimport ancestry.NumberBase;\n" + owner}
}

func TestAdversarialAnonymousInheritedProtectedLexicalFieldAcrossPackages(t *testing.T) {
	sources := hierarchyQualifiedProtectedSources()
	testNativePrivateSetterCompiledFixture(t, "client/ConstructOwner", "client.ConstructDriver", "27:constructor\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	}, nativeLexicalExactSignatures)
}
