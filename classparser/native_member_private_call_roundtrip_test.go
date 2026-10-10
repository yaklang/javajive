package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMemberPrivateProducerSuperclassArrayRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "Object prepare(Object seed,long n)", "private Object prepare(Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "class Child extends ArraySuperParent{", "Object peek(Child child){return child.recorded;}class Child extends ArraySuperParent{private Object recorded;", 1)
	fixture = strings.Replace(fixture, "prepare(seed,n)});}", "prepare(seed,n)});recorded=token;}", 1)
	fixture = strings.Replace(fixture, "if(c.owner!=o||", "if(o.peek(c)!=token||c.owner!=o||", 1)
	testNativePrivateSetterFixture(t, fixture, "ArraySuperOwner", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberPrivateProducerKeepsNonvirtualBinding(t *testing.T) {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "Object prepare(Object seed,long n)", "private Object prepare(Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "class Child extends ArraySuperParent{", "Object peek(Child child){return child.recorded;}class Child extends ArraySuperParent{private Object recorded;", 1)
	fixture = strings.Replace(fixture, "prepare(seed,n)});}", "prepare(seed,n)});recorded=token;}", 1)
	fixture = strings.Replace(fixture, "if(c.owner!=o||", "if(o.peek(c)!=token||c.owner!=o||", 1)
	fixture = strings.Replace(fixture, "class ArraySuperDriver", `class ArraySuperDerived extends ArraySuperOwner{ArraySuperDerived(Object token){super(token);}public Object prepare(Object seed,long n){ArraySuperEffects.trace+="WRONG";return null;}}class ArraySuperDriver`, 1)
	fixture = strings.ReplaceAll(fixture, "new ArraySuperOwner(token)", "new ArraySuperDerived(token)")
	testNativePrivateSetterFixture(t, fixture, "ArraySuperOwner", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberPrivateProducerKeepsNullOverloadBinding(t *testing.T) {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "Object prepare(Object seed,long n)", "private Object prepare(Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "class Child extends ArraySuperParent{", "Object peek(Child child){return child.recorded;}class Child extends ArraySuperParent{private Object recorded;", 1)
	fixture = strings.Replace(fixture, "prepare(seed,n)});}", "prepare(seed,n)});recorded=token;}", 1)
	fixture = strings.Replace(fixture, "if(c.owner!=o||", "if(o.peek(c)!=token||c.owner!=o||", 1)
	fixture = strings.Replace(fixture, "private Object prepare(Object seed,long n)", `private Object prepare(String seed,long n){ArraySuperEffects.trace+="WRONG";return null;}private Object prepare(Object seed,long n)`, 1)
	fixture = strings.Replace(fixture, "prepare(seed,n)", "prepare((Object)null,n)", 1)
	fixture = fixture[:strings.Index(fixture, "class ArraySuperDriver")] + `class ArraySuperDriver{public static void main(String[]args)throws Exception{ArraySuperOwner owner=new ArraySuperOwner(new Object());ArraySuperEffects.trace="";try{owner.make(new Object(),Long.MAX_VALUE);throw new AssertionError("missing checked failure from Object overload");}catch(java.io.IOException e){if(e!=ArraySuperEffects.error||!ArraySuperEffects.trace.equals("A"))throw new AssertionError("wrong overload or failure order");}System.out.println("private:null:overload:identity:before:parent");}}`
	testNativePrivateSetterFixture(t, fixture, "ArraySuperOwner", "ArraySuperDriver", "private:null:overload:identity:before:parent\n")
}

func TestNativeMemberPrivateCallVoidStatementPreservesEffects(t *testing.T) {
	fixture := nativePrivateProducerFixture()
	fixture = strings.Replace(fixture, "class ArraySuperOwner{", `class ArraySuperOwner{Object saved;double wide;private void touch(Object value,double d)throws java.io.IOException{ArraySuperEffects.trace+="T";if(value==null)throw ArraySuperEffects.error;saved=value;wide=d;}`, 1)
	fixture = strings.Replace(fixture, "private Object recorded;", `private Object recorded;void after(Object value,double d)throws java.io.IOException{touch(value,d);}`, 1)
	fixture = strings.Replace(fixture, "rows++;", `c.after(seed,(double)n);if(o.saved!=seed||Double.doubleToRawLongBits(o.wide)!=Double.doubleToRawLongBits((double)n)||!ArraySuperEffects.trace.equals("APT"))throw new AssertionError("void private call effects/wide parameter identity");rows++;`, 1)
	testNativePrivateSetterFixture(t, fixture, "ArraySuperOwner", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberPrivateProducerSourceSpellingRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativePrivateProducerFixture(), "ArraySuperOwner", "IndependentPrivateScope")
	fixture = strings.ReplaceAll(fixture, "prepare", "assembleOriginal")
	fixture = strings.ReplaceAll(fixture, "recorded", "retainedReference")
	fixture = strings.Replace(fixture, "if(c.owner!=o||", "if(o.peek(c)!=token||c.owner!=o||", 1)
	testNativePrivateSetterFixture(t, fixture, "IndependentPrivateScope", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberPrivateProducerRefusesPublicSpecialInvocation(t *testing.T) {
	fixture := nativePrivateProducerFixture()
	fixture = strings.Replace(fixture, "class ArraySuperDriver", `class ArraySuperDerived extends ArraySuperOwner{ArraySuperDerived(Object token){super(token);}public Object prepare(Object seed,long n){ArraySuperEffects.trace+="WRONG";return null;}}class ArraySuperDriver`, 1)
	fixture = strings.ReplaceAll(fixture, "new ArraySuperOwner(token)", "new ArraySuperDerived(token)")
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["ArraySuperOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "prepare" {
			m.AccessFlags = (m.AccessFlags &^ 2) | 1
			changed++
		}
	}
	if changed != 1 {
		t.Fatal("original private target")
	}
	files["ArraySuperOwner.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for n, raw := range files {
		if err = os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "ArraySuperDriver"); got != "6:array:super:order:identity:checked\n" {
		t.Fatalf("verifier-valid nonvirtual original=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["ArraySuperOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberReader(root).planNativeMemberFamily() != nil {
		t.Fatal("public invokespecial admitted as a private source invocation")
	}
}
