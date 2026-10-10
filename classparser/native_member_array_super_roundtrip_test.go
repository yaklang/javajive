package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberArraySuperFixture = `class ArraySuperEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class ArraySuperParent{final Object owner;final Object[] args;ArraySuperParent(String label,Object...args){ArraySuperEffects.trace+="P";owner=owner();this.args=args;if(!label.equals("physical")||args.length!=2||!args[0].equals("first"))throw new AssertionError("varargs values");}Object owner(){return null;}}
class ArraySuperOwner{final Object token;ArraySuperOwner(Object token){this.token=token;}Object prepare(Object seed,long n)throws java.io.IOException{ArraySuperEffects.trace+="A";if(seed==null)throw ArraySuperEffects.error;return new Object[]{token,seed,Long.valueOf(n)};}class Child extends ArraySuperParent{Child(Object seed,long n)throws java.io.IOException{super("physical",new Object[]{"first",prepare(seed,n)});}Object owner(){return ArraySuperOwner.this;}}Child make(Object seed,long n)throws java.io.IOException{return new Child(seed,n);}}
class ArraySuperDriver{public static void main(String[]args)throws Exception{int rows=0;Object seed=new Object();for(Object token:new Object[]{null,new Object()}){ArraySuperOwner o=new ArraySuperOwner(token);for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){ArraySuperEffects.trace="";ArraySuperOwner.Child c=o.make(seed,n);Object[] kept=(Object[])c.args[1];if(c.owner!=o||c.owner()!=o||kept[0]!=token||kept[1]!=seed||((Long)kept[2]).longValue()!=n||!ArraySuperEffects.trace.equals("AP"))throw new AssertionError("capture before callback/array allocation and component order/long identity");rows++;}ArraySuperEffects.trace="";try{o.make(null,0);throw new AssertionError("missing failure");}catch(java.io.IOException e){if(e!=ArraySuperEffects.error||!ArraySuperEffects.trace.equals("A"))throw new AssertionError("argument failure moved after parent");}}System.out.println(rows+":array:super:order:identity:checked");}}
`

func TestNativeMemberReferenceArraySuperclassRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberArraySuperFixture, "ArraySuperOwner", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberArraySuperclassKeepsEarlierArgumentOrder(t *testing.T) {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "class ArraySuperOwner{", "class ArraySuperOwner{String label(){ArraySuperEffects.trace+=\"L\";return \"physical\";}", 1)
	fixture = strings.Replace(fixture, "super(\"physical\",", "super(label(),", 1)
	fixture = strings.ReplaceAll(fixture, "equals(\"AP\")", "equals(\"LAP\")")
	fixture = strings.ReplaceAll(fixture, "equals(\"A\")", "equals(\"LA\")")
	fixture = strings.ReplaceAll(fixture, "ArraySuperOwner", "IndependentArrayScope")
	testNativePrivateSetterFixture(t, fixture, "IndependentArrayScope", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

func TestNativeMemberNestedReferenceArraySuperclassRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "new Object[]{\"first\",prepare(seed,n)}", "new Object[]{\"first\",new Object[][]{(Object[])prepare(seed,n)}}", 1)
	fixture = strings.Replace(fixture, "Object[] kept=(Object[])c.args[1];", "Object[] kept=((Object[][])c.args[1])[0];", 1)
	testNativePrivateSetterFixture(t, fixture, "ArraySuperOwner", "ArraySuperDriver", "6:array:super:order:identity:checked\n")
}

// A verifier-valid covariance mismatch throws ArrayStoreException after the
// producer's effect and before parent initialization. Refuse projecting this
// packet as an initializer that would instead cast/erase the stored element.
func TestNativeMemberArraySuperclassRefusesOriginalCovarianceFailure(t *testing.T) {
	fixture := nativeMemberArraySuperFixture[:strings.Index(nativeMemberArraySuperFixture, "class ArraySuperDriver")] + `class ArraySuperDriver{public static void main(String[]args)throws Exception{ArraySuperOwner owner=new ArraySuperOwner(new Object());ArraySuperEffects.trace="";try{owner.make(new Object(),7);throw new AssertionError("missing ArrayStoreException");}catch(ArrayStoreException expected){if(!ArraySuperEffects.trace.equals("A"))throw new AssertionError("failure did not follow producer before parent");}System.out.println("covariance:arraystore:before:parent");}}`
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["ArraySuperOwner$Child.class"])
	if err != nil {
		t.Fatal(err)
	}
	idx := sourceBridgePoolString(t, obj, "java/lang/String")
	obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: idx})
	classIndex := len(obj.ConstantPool)
	changed := 0
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "<init>" {
			continue
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err = d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				for _, op := range constructorMotionOps(d) {
					if op.Instr.OpCode == core.OP_ANEWARRAY {
						pc := int(op.CurrentOffset)
						code.Code[pc+1] = byte(classIndex >> 8)
						code.Code[pc+2] = byte(classIndex)
						changed++
					}
				}
			}
		}
	}
	if changed != 1 {
		t.Fatal("exact original array allocation")
	}
	files["ArraySuperOwner$Child.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for n, raw := range files {
		if err = os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "ArraySuperDriver"); got != "covariance:arraystore:before:parent\n" {
		t.Fatalf("verifier-valid original=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["ArraySuperOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberReader(root).planNativeMemberFamily() != nil {
		t.Fatal("covariant mismatch accepted as a source initializer")
	}
}
