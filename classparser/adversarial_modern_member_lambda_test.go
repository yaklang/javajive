package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

const modernMemberLambdaFixture = `class PeerLambdaMemberOwner {
 static class Callback {static java.util.function.Supplier<String> make(int captured){return ()->Callback.class.getCanonicalName()+":"+captured;}}
 static java.util.function.Supplier<String> make(int captured){return Callback.make(captured);}
}
class PeerLambdaMemberDriver {
 public static void main(String[]args)throws Exception{int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE}){String actual=PeerLambdaMemberOwner.make(value).get();if(!actual.equals(PeerLambdaMemberOwner.class.getName()+".Callback:"+value))throw new AssertionError("member capture and canonical identity");rows++;}
  Class<?> peer=Class.forName(PeerLambdaMemberOwner.class.getName()+"$Callback");
  if(peer.getNestHost()!=PeerLambdaMemberOwner.class||PeerLambdaMemberOwner.class.getNestMembers().length!=2||peer.getDeclaringClass()!=PeerLambdaMemberOwner.class)throw new AssertionError("original complete nest metadata");
  java.lang.reflect.Method method=peer.getDeclaredMethod("lambda$make$0",int.class);
  if(!method.isSynthetic()||method.getModifiers()!=4106||peer.getDeclaredMethods().length!=2)throw new AssertionError("private synthetic member declaration");
  if(rows!=5)throw new AssertionError("coverage");System.out.println("5:member:lambda:canonical:nest:metadata");
 }
}`

func TestAdversarialModernMemberLambdaRetainsCaptureCanonicalNameAndNest(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, modernMemberLambdaFixture, "PeerLambdaMemberOwner", "PeerLambdaMemberDriver", "5:member:lambda:canonical:nest:metadata\n", "11", []int{11})
}

func TestNativeModernMemberLambdaDeclarationUsesSameOriginalClassView(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"PeerLambdaMemberOwner.java": modernMemberLambdaFixture}, "none", "11")
	_, java := t04Tools(t)
	dir := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if output := t04RunJava(t, java, dir, "PeerLambdaMemberDriver"); output != "5:member:lambda:canonical:nest:metadata\n" {
		t.Fatal("original model", output)
	}
	root, err := Parse(bytes.Clone(files["PeerLambdaMemberOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(root)
	d.options.TargetSourceVersion = 11
	d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
	family := d.planNativeMemberFamily()
	if family == nil {
		t.Fatal("ordinary authored modern member family did not plan")
	}
	child := family.children["PeerLambdaMemberOwner$Callback"]
	if child == nil {
		t.Fatal("original member was not owned")
	}
	t.Logf("distinct parses for original nest and lexical declaration: %v", family.modernNestObjects[child.object.GetClassName()] != child.object)
	sub := NewClassObjectDumper(child.object)
	sub.options = d.options
	sub.foldSiblingResolver = d.foldSiblingResolver
	sub.nativeMemberRoot = family
	sub.nativeMemberCurrent = child
	for _, method := range child.object.Methods {
		if method.AccessFlags != 0x100a {
			continue
		}
		name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
		desc, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		if kind := sub.originalLambdaDeclaration(child.object.GetClassName(), name, desc); kind != core.LambdaDeclarationSyntheticBody {
			t.Fatalf("whole original nest lost member declaration identity: %d", kind)
		}
		return
	}
	t.Fatal("original private synthetic implementation missing")
}
