package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A checked producer is caught, and the handler exits only with the caller's
// declared wrapper. This needs no new member or inherited-name approximation.
// TreeNode deliberately lies outside the bounded platform method catalog.
const declaredCatchExitFixture = `abstract class DeclaredCatchOwner implements javax.swing.tree.TreeNode {
 static Object read(DeclaredFactory factory)throws DeclaredFailure {try{return factory.make();}catch(Exception caught){throw new DeclaredFailure(caught);}}
}
interface DeclaredFactory {Object make()throws Exception;}
class DeclaredFailure extends Exception {DeclaredFailure(Exception cause){super(cause);}}
class DeclaredEffects {static int calls;static int mode;static final Object token=new Object();static final Exception checked=new java.io.IOException("checked");static final RuntimeException runtime=new IllegalStateException("runtime");static final Error error=new AssertionError("error");}
class DeclaredCatchDriver {public static void main(String[]args)throws Exception{int rows=0;DeclaredFactory factory=new DeclaredFactory(){public Object make()throws Exception{DeclaredEffects.calls++;if(DeclaredEffects.mode==1)throw DeclaredEffects.checked;if(DeclaredEffects.mode==2)throw DeclaredEffects.runtime;if(DeclaredEffects.mode==3)return null;if(DeclaredEffects.mode==4)throw DeclaredEffects.error;return DeclaredEffects.token;}};for(int mode=0;mode<5;mode++){DeclaredEffects.mode=mode;DeclaredEffects.calls=0;try{Object value=DeclaredCatchOwner.read(factory);if(mode!=0&&mode!=3||value!=(mode==0?DeclaredEffects.token:null))throw new AssertionError("return identity");}catch(DeclaredFailure failure){if(mode!=1&&mode!=2||failure.getCause()!=(mode==1?DeclaredEffects.checked:DeclaredEffects.runtime))throw new AssertionError("wrapper identity",failure);}catch(Error failure){if(mode!=4||failure!=DeclaredEffects.error)throw new AssertionError("error identity",failure);}if(DeclaredEffects.calls!=1)throw new AssertionError("replayed factory");rows++;}System.out.println(rows+":declared-catch:identity:once");}}
`

func TestAdversarialDeclaredCheckedCatchExitNeedsNoInheritedHelperRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, declaredCatchExitFixture, "DeclaredCatchOwner", "DeclaredCatchDriver", "5:declared-catch:identity:once\n", "8", []int{8, 16})
}

func TestNativeDeclaredCheckedCatchExitWithoutDeclarationStillRequiresHelperNamespace(t *testing.T) {
	files := nativeCompileClasses(t, declaredCatchExitFixture)
	obj, err := Parse(files["DeclaredCatchOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for _, method := range obj.Methods {
		name, _ := obj.getUtf8(method.NameIndex)
		if name != "read" {
			continue
		}
		var attrs []AttributeInfo
		for _, attr := range method.Attributes {
			if _, exception := attr.(*ExceptionsAttribute); exception {
				removed++
				continue
			}
			attrs = append(attrs, attr)
		}
		method.Attributes = attrs
	}
	if removed != 1 {
		t.Fatal("no exact original throws declaration")
	}
	files["DeclaredCatchOwner.class"] = obj.Bytes()
	archive := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(archive, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	z, err := NewJarFSFromLocalWithSourceVersion(archive, 16, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	source, err := z.ReadFile("DeclaredCatchOwner.class")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(source), "yak-decompiler: undecompilable method body") != 1 || !strings.Contains(string(source), "checked escape helper requires complete inherited member names") {
		t.Fatalf("undeclared wrapper bypassed the helper namespace proof:\n%s", source)
	}
}
