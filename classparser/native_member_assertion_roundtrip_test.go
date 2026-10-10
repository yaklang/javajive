package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The assertion status is fixed by the original outermost class literal when
// the member initializes. Disabled assertions never evaluate their condition
// or message; an enabled failing condition evaluates the message once.
const nativeMemberAssertionFixture = `class MemberAssertionEffects{static String trace="";}
class MemberAssertionParent{final Object observed;MemberAssertionParent(long n){MemberAssertionEffects.trace+="P";observed=owner();if(n!=Long.MIN_VALUE)throw new AssertionError("original constructor argument");}Object owner(){return null;}}
class MemberAssertionOwner{boolean condition(boolean value){MemberAssertionEffects.trace+="C";return value;}Object message(Object token){MemberAssertionEffects.trace+="M";return token;}class Child extends MemberAssertionParent{Child(long n){super(n);}Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}}Child make(long n){return new Child(n);}}
class MemberAssertionDriver{public static void main(String[]args)throws Exception{ClassLoader loader=ClassLoader.getSystemClassLoader();boolean enabled=Boolean.parseBoolean(System.getProperty("fixture.assertions","false"));loader.setClassAssertionStatus("MemberAssertionOwner",enabled);MemberAssertionOwner owner=new MemberAssertionOwner();MemberAssertionEffects.trace="";MemberAssertionOwner.Child child=owner.make(Long.MIN_VALUE);if(child.observed!=owner||!MemberAssertionEffects.trace.equals("P"))throw new AssertionError("pre-super enclosing capture");MemberAssertionEffects.trace="";child.check(true,"success");if(!MemberAssertionEffects.trace.equals(enabled?"C":""))throw new AssertionError("success condition evaluation");MemberAssertionEffects.trace="";try{child.check(false,"failure");if(enabled)throw new AssertionError("missing source assertion");}catch(AssertionError e){if(!enabled||!e.getMessage().equals("failure"))throw new AssertionError("assertion failure kind/message",e);}if(!MemberAssertionEffects.trace.equals(enabled?"CM":""))throw new AssertionError("message effect order");java.lang.reflect.Field field=child.getClass().getDeclaredField("$assertionsDisabled");if(!field.isSynthetic()||field.getModifiers()!=0x1018)throw new AssertionError("regenerated assertion field metadata");System.out.println("assertions:"+enabled+":owner:condition:message:order");}}
`

func TestNativeMemberAssertionProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberAssertionFixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:false:owner:condition:message:order\n")
}

func TestNativeMemberEnabledAssertionProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`), "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

func TestNativeMemberRenamedAssertionProtocolRoundTrip(t *testing.T) {
	fixture := strings.NewReplacer("MemberAssertionOwner", "RenamedAssertionScope", "condition(", "predicate(", "message(", "payload(").Replace(nativeMemberAssertionFixture)
	testNativePrivateSetterFixture(t, fixture, "RenamedAssertionScope", "MemberAssertionDriver", "assertions:false:owner:condition:message:order\n")
}
func TestNativeMemberAssertionWithoutMessageRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `assert condition(value):message(token);`, `assert condition(value);`)
	fixture = strings.ReplaceAll(fixture, `!e.getMessage().equals("failure")`, `e.getMessage()!=null`)
	fixture = strings.ReplaceAll(fixture, `enabled?"CM":""`, `enabled?"C":""`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}
func TestNativeMemberAssertionMessagePreservesThrowableIdentity(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `child.check(false,"failure");`, `child.check(false,token);`)
	fixture = strings.ReplaceAll(fixture, `try{child.check(false,token);`, `Throwable token=new RuntimeException("payload");try{child.check(false,token);`)
	fixture = strings.ReplaceAll(fixture, `!e.getMessage().equals("failure")`, `e.getCause()!=token`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

// A verifier-valid class literal mutation fixes the child's assertion state to
// its own loader override. Java source member asserts instead query the root.
func TestNativeMemberAssertionRefusesDifferentStatusClass(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `loader.setClassAssertionStatus("MemberAssertionOwner",enabled);`, `loader.setClassAssertionStatus("MemberAssertionOwner",false);loader.setClassAssertionStatus("MemberAssertionOwner$Child",true);`)
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["MemberAssertionOwner$Child.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "<clinit>" {
			continue
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				if obj.ThisClass > 255 || len(code.Code) != 17 || code.Code[0] != 18 {
					t.Fatal("original class literal encoding")
				}
				code.Code[1] = byte(obj.ThisClass)
				changed++
			}
		}
	}
	if changed != 1 {
		t.Fatal("original status witness")
	}
	files["MemberAssertionOwner$Child.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for name, raw := range files {
		if err = os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "MemberAssertionDriver"); got != "assertions:true:owner:condition:message:order\n" {
		t.Fatalf("verifier-valid independent status=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["MemberAssertionOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberReader(root).planNativeMemberFamily() != nil {
		t.Fatal("foreign assertion status admitted as outermost source assert")
	}
}

func TestNativeMemberMultipleAssertionsPreserveShortCircuitOrder(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `assert condition(value):message(token);`, `assert condition(true);assert condition(value):message(token);`)
	fixture = strings.ReplaceAll(fixture, `enabled?"C":""`, `enabled?"CC":""`)
	fixture = strings.ReplaceAll(fixture, `enabled?"CM":""`, `enabled?"CCM":""`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}
func TestNativeMemberAssertionWidePrimitiveMessageRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `Object message(Object token){MemberAssertionEffects.trace+="M";return token;}`, `double message(Object token){MemberAssertionEffects.trace+="M";return -0.0d;}`)
	fixture = strings.ReplaceAll(fixture, `!e.getMessage().equals("failure")`, `!e.getMessage().equals("-0.0")`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

func TestNativeStaticMemberAssertionProtocolRoundTrip(t *testing.T) {
	fixture := strings.NewReplacer(`boolean condition(boolean value)`, `static boolean condition(boolean value)`, `Object message(Object token)`, `static Object message(Object token)`, `class Child extends`, `static class Child extends`, `return MemberAssertionOwner.this;`, `return MemberAssertionOwner.class;`, `child.observed!=owner`, `child.observed!=MemberAssertionOwner.class`).Replace(nativeMemberAssertionFixture)
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			input := strings.ReplaceAll(fixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, enabled)
			testNativePrivateSetterFixture(t, input, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:"+enabled+":owner:condition:message:order\n")
		})
	}
}

func TestNativeMemberConstantFalseAssertionRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `assert condition(value):message(token);`, `if(value)return;assert false:message(token);`)
	fixture = strings.ReplaceAll(fixture, `enabled?"C":""`, `""`)
	fixture = strings.ReplaceAll(fixture, `enabled?"CM":""`, `enabled?"M":""`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

func TestNativeMemberCompoundAssertionPreservesConditionOrder(t *testing.T) {
	fixture := strings.ReplaceAll(nativeMemberAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `assert condition(value):message(token);`, `assert condition(value)&&condition(value):message(token);`)
	fixture = strings.ReplaceAll(fixture, `enabled?"C":""`, `enabled?"CC":""`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}
