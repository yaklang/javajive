package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nativeAnonymousAssertionFixture = `class MemberAssertionEffects{static String trace="";}
class MemberAssertionParent{final Object observed;MemberAssertionParent(long n){MemberAssertionEffects.trace+="P";observed=owner();if(n!=Long.MIN_VALUE)throw new AssertionError("original constructor argument");}Object owner(){return null;}void check(boolean value,Object token){}}
class MemberAssertionOwner{boolean condition(boolean value){MemberAssertionEffects.trace+="C";return value;}Object message(Object token){MemberAssertionEffects.trace+="M";return token;}MemberAssertionParent make(long n){return new MemberAssertionParent(n){Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}};}}
class MemberAssertionDriver{public static void main(String[]args)throws Exception{ClassLoader loader=ClassLoader.getSystemClassLoader();boolean enabled=Boolean.parseBoolean(System.getProperty("fixture.assertions","false"));loader.setClassAssertionStatus("MemberAssertionOwner",enabled);MemberAssertionOwner owner=new MemberAssertionOwner();MemberAssertionEffects.trace="";MemberAssertionParent child=owner.make(Long.MIN_VALUE);if(child.observed!=owner||!MemberAssertionEffects.trace.equals("P"))throw new AssertionError("pre-super enclosing capture");MemberAssertionEffects.trace="";child.check(true,"success");if(!MemberAssertionEffects.trace.equals(enabled?"C":""))throw new AssertionError("success condition evaluation");MemberAssertionEffects.trace="";try{child.check(false,"failure");if(enabled)throw new AssertionError("missing source assertion");}catch(AssertionError e){if(!enabled||!e.getMessage().equals("failure"))throw new AssertionError("assertion failure kind/message",e);}if(!MemberAssertionEffects.trace.equals(enabled?"CM":""))throw new AssertionError("message effect order");if(!child.getClass().getName().equals("MemberAssertionOwner$1")||child.getClass().getEnclosingClass()!=MemberAssertionOwner.class||child.getClass().getDeclaringClass()!=null||!child.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("original anonymous ownership");java.lang.reflect.Field field=child.getClass().getDeclaredField("$assertionsDisabled");if(!field.isSynthetic()||field.getModifiers()!=0x1018)throw new AssertionError("regenerated assertion field metadata");System.out.println("assertions:"+enabled+":owner:condition:message:order");}}
`

func TestNativeAnonymousAssertionProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousAssertionFixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:false:owner:condition:message:order\n")
}
func TestNativeAnonymousEnabledAssertionProtocolRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

func TestNativeAnonymousAssertionInsideNamedMemberRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `MemberAssertionParent make(long n){return new MemberAssertionParent(n){Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}};}`, `class Factory{MemberAssertionParent make(long n){return new MemberAssertionParent(n){Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}};}}MemberAssertionParent make(long n){return new Factory().make(n);}`)
	fixture = strings.ReplaceAll(fixture, `"MemberAssertionOwner$1"`, `"MemberAssertionOwner$Factory$1"`)
	fixture = strings.ReplaceAll(fixture, `child.getClass().getEnclosingClass()!=MemberAssertionOwner.class`, `child.getClass().getEnclosingClass()!=MemberAssertionOwner.Factory.class`)
	testNativePrivateSetterFixture(t, fixture, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}
func TestNativeAnonymousAssertionRenamedScopeRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousAssertionFixture, "MemberAssertionOwner", "OtherAssertionScope")
	testNativePrivateSetterFixture(t, fixture, "OtherAssertionScope", "MemberAssertionDriver", "assertions:false:owner:condition:message:order\n")
}
func TestNativeAnonymousAssertionRefusesDifferentStatusClass(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousAssertionFixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	fixture = strings.ReplaceAll(fixture, `loader.setClassAssertionStatus("MemberAssertionOwner",enabled);`, `loader.setClassAssertionStatus("MemberAssertionOwner",false);loader.setClassAssertionStatus("MemberAssertionOwner$1",true);`)
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["MemberAssertionOwner$1.class"])
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
					t.Fatal("original class literal")
				}
				code.Code[1] = byte(obj.ThisClass)
				changed++
			}
		}
	}
	if changed != 1 {
		t.Fatal("original assertion witness")
	}
	files["MemberAssertionOwner$1.class"] = obj.Bytes()
	original := t.TempDir()
	_, java := t04Tools(t)
	for name, raw := range files {
		if err = os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "MemberAssertionDriver"); got != "assertions:true:owner:condition:message:order\n" {
		t.Fatalf("independent original status=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["MemberAssertionOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberReader(root).planNativeAnonymousFamily() != nil {
		t.Fatal("anonymous assertion status converted to wrong outermost class")
	}
}

func nativeAnonymousNestedAssertionFixture(enabled bool) string {
	fixture := strings.ReplaceAll(nativeAnonymousAssertionFixture, `void check(boolean value,Object token){}}`, `void check(boolean value,Object token){}MemberAssertionParent deeper(){return this;}}`)
	fixture = strings.ReplaceAll(fixture, `MemberAssertionParent make(long n){return new MemberAssertionParent(n){Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}};}`, `MemberAssertionParent make(long n){return new MemberAssertionParent(n){Object owner(){return MemberAssertionOwner.this;}MemberAssertionParent deeper(){return new MemberAssertionParent(Long.MIN_VALUE){Object owner(){return MemberAssertionOwner.this;}void check(boolean value,Object token){assert condition(value):message(token);}};}};}`)
	fixture = strings.ReplaceAll(fixture, `owner.make(Long.MIN_VALUE);`, `owner.make(Long.MIN_VALUE).deeper();`)
	fixture = strings.ReplaceAll(fixture, `trace.equals("P")`, `trace.equals("PP")`)
	fixture = strings.ReplaceAll(fixture, `"MemberAssertionOwner$1"`, `"MemberAssertionOwner$1$1"`)
	fixture = strings.ReplaceAll(fixture, `child.getClass().getEnclosingClass()!=MemberAssertionOwner.class`, `child.getClass().getEnclosingClass()!=Class.forName("MemberAssertionOwner$1")`)
	fixture = strings.ReplaceAll(fixture, `getName().equals("make")`, `getName().equals("deeper")`)
	if enabled {
		fixture = strings.ReplaceAll(fixture, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`)
	}
	return fixture
}
func TestNativeAnonymousNestedAssertionProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousNestedAssertionFixture(false), "MemberAssertionOwner", "MemberAssertionDriver", "assertions:false:owner:condition:message:order\n")
}
func TestNativeAnonymousNestedEnabledAssertionProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousNestedAssertionFixture(true), "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}
