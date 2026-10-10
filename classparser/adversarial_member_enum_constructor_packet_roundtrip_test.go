package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"strings"
	"testing"
)

func TestAdversarialMemberEnumSourceConstructorPacketRoundTrip(t *testing.T) {
	f := strings.Replace(nativeEnumScopeFixture, "LEFT,RIGHT;", "LEFT(7),RIGHT(11);final int n;private Mode(int n){this.n=n;}", 1)
	f = strings.Replace(f, "EnumScopeOwner.Mode[] copy=", `if(EnumScopeOwner.Mode.LEFT.n!=7||EnumScopeOwner.Mode.RIGHT.n!=11)throw new AssertionError("source constructor operands");EnumScopeOwner.Mode[] copy=`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"EnumScopeOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumSourceConstructorPacketRenamedRoundTrip(t *testing.T) {
	f := strings.Replace(nativeEnumScopeFixture, "LEFT,RIGHT;", "LEFT(7),RIGHT(11);final int n;private Mode(int n){this.n=n;}", 1)
	f = strings.Replace(f, "EnumScopeOwner.Mode[] copy=", `if(EnumScopeOwner.Mode.LEFT.n!=7||EnumScopeOwner.Mode.RIGHT.n!=11)throw new AssertionError("source constructor operands");EnumScopeOwner.Mode[] copy=`, 1)
	f = strings.ReplaceAll(f, "EnumScopeOwner", "RenamedEnumOwner")
	f = strings.ReplaceAll(f, "Token", "Element")
	f = strings.ReplaceAll(f, "Mode", "Choice")
	testNativeIndependentFamilyFixture(t, f, []string{"RenamedEnumOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumWideEffectsConstructorPacketRoundTrip(t *testing.T) {
	const f = `class EnumPacketEffects{static String trace="";static final Object token=new Object();static long choose(long n){trace+="A";return n;}}
 class EnumPacketOwner{enum Mode{LEFT(EnumPacketEffects.choose(Long.MIN_VALUE),-0.0,EnumPacketEffects.token,true),RIGHT(EnumPacketEffects.choose(Long.MAX_VALUE),Double.longBitsToDouble(0x7ff8000000000042L),null,false);final long n;final double d;final Object value;final boolean flag;private Mode(long n,double d,Object v,boolean b){EnumPacketEffects.trace+="C";this.n=n;this.d=d;this.value=v;this.flag=b;}}static class Token{}}
 class EnumPacketDriver{public static void main(String[]args)throws Exception{EnumPacketOwner.Mode a=EnumPacketOwner.Mode.LEFT,b=EnumPacketOwner.Mode.RIGHT;if(a.n!=Long.MIN_VALUE||b.n!=Long.MAX_VALUE||Double.doubleToRawLongBits(a.d)!=0x8000000000000000L||Double.doubleToRawLongBits(b.d)!=0x7ff8000000000042L||a.value!=EnumPacketEffects.token||b.value!=null||!a.flag||b.flag||!EnumPacketEffects.trace.equals("ACAC"))throw new AssertionError("wide/category/identity/effects");if(a.getDeclaringClass().getDeclaringClass()!=EnumPacketOwner.class||new EnumPacketOwner.Token().getClass().getDeclaringClass()!=EnumPacketOwner.class||EnumPacketOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("owners");EnumPacketOwner.Mode[]copy=EnumPacketOwner.Mode.values();copy[0]=null;if(EnumPacketOwner.Mode.values()[0]!=a||EnumPacketOwner.Mode.valueOf("RIGHT")!=b||a.ordinal()!=0||b.ordinal()!=1)throw new AssertionError("factory");java.lang.reflect.Constructor<?>c=EnumPacketOwner.Mode.class.getDeclaredConstructor(String.class,int.class,long.class,double.class,Object.class,boolean.class);if(!java.lang.reflect.Modifier.isPrivate(c.getModifiers()))throw new AssertionError("original selected constructor");System.out.println("wide:raw-bits:identity:ACAC:owners");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"EnumPacketOwner"}, "EnumPacketDriver", "wide:raw-bits:identity:ACAC:owners\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumNestedVarargsConstructorPacketRoundTrip(t *testing.T) {
	const f = `class EnumArrayEffects{static String trace="";static final Object token=new Object();}class EnumArrayPayload{final String label;final int n;EnumArrayPayload(String label,int n){EnumArrayEffects.trace+="A";this.label=label;this.n=n;}}
 class EnumArrayOwner{enum Mode{LEFT(new Object[]{EnumArrayEffects.token,new EnumArrayPayload("LEFT",0),null}),RIGHT((Object[])null);final Object[]tail;private Mode(Object...tail){EnumArrayEffects.trace+="C";this.tail=tail;}}static class Token{}}
 class EnumArrayDriver{public static void main(String[]args)throws Exception{EnumArrayOwner.Mode a=EnumArrayOwner.Mode.LEFT,b=EnumArrayOwner.Mode.RIGHT;if(a.tail.length!=3||a.tail[0]!=EnumArrayEffects.token||!((EnumArrayPayload)a.tail[1]).label.equals("LEFT")||((EnumArrayPayload)a.tail[1]).n!=0||a.tail[2]!=null||b.tail!=null||!EnumArrayEffects.trace.equals("ACC"))throw new AssertionError("nested allocation/varargs/effects");if(a.getDeclaringClass().getDeclaringClass()!=EnumArrayOwner.class||new EnumArrayOwner.Token().getClass().getDeclaringClass()!=EnumArrayOwner.class||EnumArrayOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("owners");java.lang.reflect.Constructor<?>c=EnumArrayOwner.Mode.class.getDeclaredConstructor(String.class,int.class,Object[].class);if(!c.isVarArgs()||!java.lang.reflect.Modifier.isPrivate(c.getModifiers()))throw new AssertionError("varargs declaration");System.out.println("nested:array:identity:ACC:varargs:owners");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"EnumArrayOwner"}, "EnumArrayDriver", "nested:array:identity:ACC:varargs:owners\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumOverloadConstructorPacketRoundTrip(t *testing.T) {
	const f = `class EnumOverloadEffects{static String trace="";static Object value(int n){trace+="A";return n==0?null:"value";}}
 class EnumOverloadOwner{enum Mode{LEFT(EnumOverloadEffects.value(0),Long.MIN_VALUE),RIGHT(EnumOverloadEffects.value(1),Long.MAX_VALUE);final Object value;final long n;final int selected;private Mode(Object v,long n){EnumOverloadEffects.trace+="C";this.value=v;this.n=n;this.selected=1;}private Mode(String v,long n){throw new AssertionError("wrong String overload");}}static class Token{}}
 class EnumOverloadDriver{public static void main(String[]args)throws Exception{EnumOverloadOwner.Mode a=EnumOverloadOwner.Mode.LEFT,b=EnumOverloadOwner.Mode.RIGHT;if(a.value!=null||!b.value.equals("value")||a.n!=Long.MIN_VALUE||b.n!=Long.MAX_VALUE||a.selected!=1||b.selected!=1||!EnumOverloadEffects.trace.equals("ACAC"))throw new AssertionError("erased operand/overload/effects");if(a.getDeclaringClass().getDeclaringClass()!=EnumOverloadOwner.class||new EnumOverloadOwner.Token().getClass().getDeclaringClass()!=EnumOverloadOwner.class||EnumOverloadOwner.class.getDeclaredClasses().length!=2||EnumOverloadOwner.Mode.class.getDeclaredConstructors().length!=2)throw new AssertionError("owners/constructor family");System.out.println("overloads:Object:null:String-value:ACAC:owners");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"EnumOverloadOwner"}, "EnumOverloadDriver", "overloads:Object:null:String-value:ACAC:owners\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumConditionalConstructorPacketRoundTrip(t *testing.T) {
	const f = `class EnumFlowEffects{static String trace="";static boolean select(boolean b){trace+="A";return b;}}
 class EnumFlowOwner{enum Mode{LEFT(EnumFlowEffects.select(true)?7:11),RIGHT(EnumFlowEffects.select(false)?7:11);final int n;Mode(int n){EnumFlowEffects.trace+="C";this.n=n;}}static class Token{}}
 class EnumFlowDriver{public static void main(String[]args){if(EnumFlowOwner.Mode.LEFT.n!=7||EnumFlowOwner.Mode.RIGHT.n!=11||!EnumFlowEffects.trace.equals("ACAC"))throw new AssertionError("conditional argument order");if(EnumFlowOwner.Mode.class.getDeclaringClass()!=EnumFlowOwner.class||new EnumFlowOwner.Token().getClass().getDeclaringClass()!=EnumFlowOwner.class||EnumFlowOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("owners");System.out.println("branches:7:11:ACAC:owners");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"EnumFlowOwner"}, "EnumFlowDriver", "branches:7:11:ACAC:owners\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumFailingConstructorPacketRoundTrip(t *testing.T) {
	const f = `class EnumFailEffects{static String trace="";static final IllegalArgumentException error=new IllegalArgumentException("same");static int arg(int n){trace+="A";return n;}}
 class EnumFailOwner{enum Mode{LEFT(EnumFailEffects.arg(1)),RIGHT(EnumFailEffects.arg(2));Mode(int n){EnumFailEffects.trace+="C";if(n==2)throw EnumFailEffects.error;}}static class Token{}}
 class EnumFailDriver{public static void main(String[]args){try{EnumFailOwner.Mode.values();throw new AssertionError("lost initialization failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=EnumFailEffects.error||!EnumFailEffects.trace.equals("ACAC"))throw new AssertionError("failure identity/order");}try{EnumFailOwner.Mode.values();throw new AssertionError("repeated initialization");}catch(NoClassDefFoundError e){if(!EnumFailEffects.trace.equals("ACAC"))throw new AssertionError("extra effects");}if(EnumFailOwner.Mode.class.getDeclaringClass()!=EnumFailOwner.class||new EnumFailOwner.Token().getClass().getDeclaringClass()!=EnumFailOwner.class)throw new AssertionError("owners");System.out.println("failure:shared-identity:ACAC:once:owners");}}`
	testNativeIndependentFamilyFixture(t, f, []string{"EnumFailOwner"}, "EnumFailDriver", "failure:shared-identity:ACAC:once:owners\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberEnumInlineSourceConstructorPacketRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeEnumScopeFixture, "LEFT,RIGHT;", "LEFT(7),RIGHT(11);final int n;private Mode(int n){this.n=n;}", 1)
	fixture = strings.Replace(fixture, "EnumScopeOwner.Mode[] copy=", `if(EnumScopeOwner.Mode.LEFT.n!=7||EnumScopeOwner.Mode.RIGHT.n!=11)throw new AssertionError("source constructor operands");EnumScopeOwner.Mode[] copy=`, 1)
	testNativePrivateSetterCompiledFixture(t, "EnumScopeOwner", "EnumScopeDriver", "4:enum:member:sibling:identity\n", func(t *testing.T, debug string) map[string][]byte {
		files := nativeCompileDebugClasses(t, fixture, debug)
		obj, e := Parse(files["EnumScopeOwner$Mode.class"])
		if e != nil {
			t.Fatal(e)
		}
		var helper, initializer *MemberInfo
		var array, code *CodeAttribute
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					if n == "$values" {
						helper, array = m, c
					}
					if n == "<clinit>" {
						initializer, code = m, c
					}
				}
			}
		}
		if helper == nil || initializer == nil || array == nil || code == nil || len(array.Code) == 0 || array.Code[len(array.Code)-1] != byte(core.OP_ARETURN) {
			t.Fatal("independent enum protocol")
		}
		ops, known := nativeEnumMethodOps(obj, initializer, nil)
		if !known {
			t.Fatal("initializer opcodes")
		}
		pc := -1
		for _, op := range ops {
			if nativeEnumMemberOperand(obj, op, core.OP_INVOKESTATIC, obj.GetClassName(), "$values", "()[L"+obj.GetClassName()+";") {
				pc = int(op.CurrentOffset)
			}
		}
		if pc < 0 {
			t.Fatal("array helper call")
		}
		rewritten := append([]byte(nil), code.Code[:pc]...)
		rewritten = append(rewritten, array.Code[:len(array.Code)-1]...)
		rewritten = append(rewritten, code.Code[pc+3:]...)
		code.Code, code.Attributes = rewritten, nil
		code.AttrLen = uint32(12 + len(rewritten))
		if code.MaxStack < array.MaxStack {
			code.MaxStack = array.MaxStack
		}
		methods := []*MemberInfo{}
		for _, m := range obj.Methods {
			if m != helper {
				methods = append(methods, m)
			}
		}
		obj.Methods = methods
		files["EnumScopeOwner$Mode.class"] = obj.Bytes()
		return files
	}, nativeLexicalExactSignatures)
}
